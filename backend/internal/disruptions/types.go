// Package disruptions implements transit disruption creation, status lifecycle,
// affected-service evaluation, operational status queries, and external notice ingestion.
package disruptions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrNotFound indicates the requested disruption, route, stop, trip, or vehicle record does not exist.
	ErrNotFound = errors.New("transit resource not found")
)

// InvalidInput describes a request parameter or payload validation failure.
type InvalidInput struct{ Message string }

func (e *InvalidInput) Error() string { return e.Message }

const (
	// Disruption types allowed by the schema and API contract.
	TypeDelay              = "delay"
	TypeDetour             = "detour"
	TypeCancellation       = "cancellation"
	TypeStopClosure        = "stop_closure"
	TypeRoadClosure        = "road_closure"
	TypeConstruction       = "construction"
	TypeEmergency          = "emergency"
	TypeTrafficRestriction = "traffic_restriction"
	TypeAccessibility      = "accessibility"

	// Disruption lifecycle statuses.
	StatusScheduled = "scheduled"
	StatusActive    = "active"
	StatusResolved  = "resolved"

	// Entity operational statuses derived from active disruptions (PT-FR-016).
	OpStatusNormal     = "normal"
	OpStatusCancelled  = "cancelled"
	OpStatusClosed     = "closed"
	OpStatusDetoured   = "detoured"
	OpStatusDelayed    = "delayed"
	OpStatusRestricted = "restricted"
	OpStatusDisrupted  = "disrupted"

	// Overall system operational statuses (PT-FR-021).
	OverallStatusNormal    = "normal"
	OverallStatusDisrupted = "disrupted"

	// Canonical source services for administrative and external feeds (PT-NFR-007).
	SourceTransitAdmin = "transit_admin"
	SourceUtilities    = "utilities"
	SourceEmergency    = "emergency_services"
	SourceSecurity     = "security_police"

	// Validation bounds matching PostgreSQL schema constraints (PT-NFR-006).
	MaxTitleRunes         = 100
	MaxDescriptionRunes   = 2000
	MaxAffectedAreaRunes  = 200
	MaxSourceServiceRunes = 50
	MaxExternalRefRunes   = 120
	MaxSegmentRunes       = 255
	MaxListItems          = 500
	MinRadiusMeters       = 50.0
	MaxRadiusMeters       = 5000.0
	DefaultRadiusMeters   = 400.0
)

var allowedTypes = map[string]bool{
	TypeDelay:              true,
	TypeDetour:             true,
	TypeCancellation:       true,
	TypeStopClosure:        true,
	TypeRoadClosure:        true,
	TypeConstruction:       true,
	TypeEmergency:          true,
	TypeTrafficRestriction: true,
	TypeAccessibility:      true,
}

var allowedStatuses = map[string]bool{
	StatusScheduled: true,
	StatusActive:    true,
	StatusResolved:  true,
}

// IsValidType reports whether t is a supported disruption type.
func IsValidType(t string) bool {
	return allowedTypes[t]
}

// IsValidStatus reports whether s is a supported disruption status.
func IsValidStatus(s string) bool {
	return allowedStatuses[s]
}

// ListFilter specifies optional query filters for disruption lookups.
type ListFilter struct {
	Status        string
	Type          string
	RouteID       int
	StopID        int
	TripID        int
	VehicleID     int
	SourceService string
}

// CreateDisruptionInput is the request payload for POST /api/disruptions.
type CreateDisruptionInput struct {
	Type              string     `json:"type" enums:"delay,detour,cancellation,stop_closure,road_closure,construction,emergency,traffic_restriction,accessibility"`
	Title             string     `json:"title"`
	Description       *string    `json:"description,omitempty"`
	AffectedArea      *string    `json:"affected_area,omitempty"`
	Status            string     `json:"status,omitempty" enums:"scheduled,active,resolved"`
	StartTime         *time.Time `json:"start_time,omitempty"`
	EndTime           *time.Time `json:"end_time,omitempty"`
	SourceService     string     `json:"source_service,omitempty"`
	ExternalReference *string    `json:"external_reference,omitempty"`
	RouteIDs          []int      `json:"route_ids,omitempty"`
	StopIDs           []int      `json:"stop_ids,omitempty"`
	TripIDs           []int      `json:"trip_ids,omitempty"`
	VehicleIDs        []int      `json:"vehicle_ids,omitempty"`
	AffectedSegments  []string   `json:"affected_segments,omitempty"`
}

// UpdateDisruptionInput is the request payload for PATCH /api/disruptions/{id}.
type UpdateDisruptionInput struct {
	Type             *string    `json:"type,omitempty" enums:"delay,detour,cancellation,stop_closure,road_closure,construction,emergency,traffic_restriction,accessibility"`
	Title            *string    `json:"title,omitempty"`
	Description      *string    `json:"description,omitempty"`
	AffectedArea     *string    `json:"affected_area,omitempty"`
	Status           *string    `json:"status,omitempty" enums:"scheduled,active,resolved"`
	StartTime        *time.Time `json:"start_time,omitempty"`
	EndTime          *time.Time `json:"end_time,omitempty"`
	RouteIDs         *[]int     `json:"route_ids,omitempty"`
	StopIDs          *[]int     `json:"stop_ids,omitempty"`
	TripIDs          *[]int     `json:"trip_ids,omitempty"`
	VehicleIDs       *[]int     `json:"vehicle_ids,omitempty"`
	AffectedSegments *[]string  `json:"affected_segments,omitempty"`

	// EndTimeSet indicates the JSON payload explicitly included "end_time" (including null to clear it).
	EndTimeSet bool `json:"-"`
	// DescriptionSet indicates the JSON payload explicitly included "description".
	DescriptionSet bool `json:"-"`
	// AffectedAreaSet indicates the JSON payload explicitly included "affected_area".
	AffectedAreaSet bool `json:"-"`
}

// RoadClosureAddress represents the shared SH-I06 municipal address object.
type RoadClosureAddress struct {
	StreetNumber string `json:"streetNumber"`
	StreetName   string `json:"streetName"`
	City         string `json:"city"`
	PostalCode   string `json:"postalCode"`
}

// UnmarshalJSON decodes an SH-I06 municipal address object, accepting string or non-negative
// integer street numbers and rejecting unknown fields or conflicting camelCase/snake_case aliases.
func (a *RoadClosureAddress) UnmarshalJSON(data []byte) error {
	trimmedData := bytes.TrimSpace(data)
	if bytes.Equal(trimmedData, []byte("null")) {
		return errors.New("address cannot be null")
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(trimmedData, &raw); err != nil || raw == nil {
		return errors.New("address must be a JSON object")
	}

	var (
		hasStreetNumCamel, hasStreetNumSnake   bool
		streetNumCamel, streetNumSnake         string
		hasStreetNameCamel, hasStreetNameSnake bool
		streetNameCamel, streetNameSnake       string
		hasPostalCamel, hasPostalSnake         bool
		postalCamel, postalSnake               string
		cityVal                                string
	)

	for key, val := range raw {
		switch key {
		case "streetNumber":
			s, err := decodeStreetNumberJSON(val)
			if err != nil {
				return err
			}
			hasStreetNumCamel = true
			streetNumCamel = s
		case "street_number":
			s, err := decodeStreetNumberJSON(val)
			if err != nil {
				return err
			}
			hasStreetNumSnake = true
			streetNumSnake = s
		case "streetName":
			s, err := decodeAddressStringJSON("streetName", val)
			if err != nil {
				return err
			}
			hasStreetNameCamel = true
			streetNameCamel = s
		case "street_name":
			s, err := decodeAddressStringJSON("street_name", val)
			if err != nil {
				return err
			}
			hasStreetNameSnake = true
			streetNameSnake = s
		case "city":
			s, err := decodeAddressStringJSON("city", val)
			if err != nil {
				return err
			}
			cityVal = s
		case "postalCode":
			s, err := decodeAddressStringJSON("postalCode", val)
			if err != nil {
				return err
			}
			hasPostalCamel = true
			postalCamel = s
		case "postal_code":
			s, err := decodeAddressStringJSON("postal_code", val)
			if err != nil {
				return err
			}
			hasPostalSnake = true
			postalSnake = s
		default:
			return fmt.Errorf("unknown address field %q", key)
		}
	}

	if hasStreetNumCamel && hasStreetNumSnake && strings.TrimSpace(streetNumCamel) != strings.TrimSpace(streetNumSnake) {
		return errors.New("address streetNumber conflicts with street_number")
	}
	if hasStreetNameCamel && hasStreetNameSnake && strings.TrimSpace(streetNameCamel) != strings.TrimSpace(streetNameSnake) {
		return errors.New("address streetName conflicts with street_name")
	}
	if hasPostalCamel && hasPostalSnake && strings.TrimSpace(postalCamel) != strings.TrimSpace(postalSnake) {
		return errors.New("address postalCode conflicts with postal_code")
	}

	streetNumber := streetNumCamel
	if !hasStreetNumCamel {
		streetNumber = streetNumSnake
	}
	streetName := streetNameCamel
	if !hasStreetNameCamel {
		streetName = streetNameSnake
	}
	postalCode := postalCamel
	if !hasPostalCamel {
		postalCode = postalSnake
	}

	*a = RoadClosureAddress{
		StreetNumber: streetNumber,
		StreetName:   streetName,
		City:         cityVal,
		PostalCode:   postalCode,
	}
	return nil
}

// Format renders a compact display string for affected_area and derived titles.
func (a RoadClosureAddress) Format() string {
	streetNum := strings.TrimSpace(a.StreetNumber)
	streetName := strings.TrimSpace(a.StreetName)
	city := strings.TrimSpace(a.City)
	postalCode := strings.ToUpper(strings.TrimSpace(a.PostalCode))

	streetLine := streetName
	if streetNum != "" && streetName != "" {
		streetLine = streetNum + " " + streetName
	} else if streetNum != "" {
		streetLine = streetNum
	}

	parts := make([]string, 0, 3)
	if streetLine != "" {
		parts = append(parts, streetLine)
	}
	if city != "" {
		parts = append(parts, city)
	}
	if postalCode != "" {
		parts = append(parts, postalCode)
	}
	return strings.Join(parts, ", ")
}

func decodeStreetNumberJSON(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("null")) {
		return "", nil
	}
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return "", errors.New("address.streetNumber must be a string or non-negative integer")
		}
		return s, nil
	}
	num, err := strconv.ParseUint(string(trimmed), 10, 64)
	if err != nil {
		return "", errors.New("address.streetNumber must be a string or non-negative integer")
	}
	return strconv.FormatUint(num, 10), nil
}

func decodeAddressStringJSON(field string, raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("null")) {
		return "", nil
	}
	if len(trimmed) == 0 || trimmed[0] != '"' {
		return "", fmt.Errorf("address.%s must be a string", field)
	}
	var s string
	if err := json.Unmarshal(trimmed, &s); err != nil {
		return "", fmt.Errorf("address.%s must be a string", field)
	}
	return s, nil
}

// RoadClosureInput is the request payload for POST /api/transport/v1/road-closures.
type RoadClosureInput struct {
	Address           *RoadClosureAddress `json:"address,omitempty"`
	ValidUntil        *uint64             `json:"valid-until,omitempty"`
	ExternalID        *string             `json:"external_id,omitempty"`
	ExternalReference *string             `json:"external_reference,omitempty"`
	Type              string              `json:"type,omitempty" enums:"road_closure" default:"road_closure"`
	Title             string              `json:"title,omitempty"`
	Description       *string             `json:"description,omitempty"`
	AffectedArea      *string             `json:"affected_area,omitempty"`
	Status            string              `json:"status,omitempty" enums:"scheduled,active,resolved"`
	StartTime         *time.Time          `json:"start_time,omitempty"`
	EndTime           *time.Time          `json:"end_time,omitempty"`
	SourceService     string              `json:"source_service,omitempty"`
	RouteIDs          []int               `json:"route_ids,omitempty"`
	StopIDs           []int               `json:"stop_ids,omitempty"`
	TripIDs           []int               `json:"trip_ids,omitempty"`
	VehicleIDs        []int               `json:"vehicle_ids,omitempty"`
	AffectedSegments  []string            `json:"affected_segments,omitempty"`
	Latitude          *float64            `json:"latitude,omitempty"`
	Longitude         *float64            `json:"longitude,omitempty"`
	RadiusMeters      *float64            `json:"radius_meters,omitempty"`
}

// UpdateRoadClosureInput is the request payload for PATCH /api/transport/v1/road-closures/{id}.
type UpdateRoadClosureInput struct {
	Address          *RoadClosureAddress `json:"address,omitempty"`
	ValidUntil       *uint64             `json:"valid-until,omitempty"`
	Title            *string             `json:"title,omitempty"`
	Description      *string             `json:"description,omitempty"`
	AffectedArea     *string             `json:"affected_area,omitempty"`
	Status           *string             `json:"status,omitempty" enums:"scheduled,active,resolved"`
	StartTime        *time.Time          `json:"start_time,omitempty"`
	EndTime          *time.Time          `json:"end_time,omitempty"`
	RouteIDs         *[]int              `json:"route_ids,omitempty"`
	StopIDs          *[]int              `json:"stop_ids,omitempty"`
	TripIDs          *[]int              `json:"trip_ids,omitempty"`
	VehicleIDs       *[]int              `json:"vehicle_ids,omitempty"`
	AffectedSegments *[]string           `json:"affected_segments,omitempty"`

	// EndTimeSet indicates the JSON payload explicitly included "end_time" (including null to clear it).
	EndTimeSet bool `json:"-"`
	// DescriptionSet indicates the JSON payload explicitly included "description".
	DescriptionSet bool `json:"-"`
	// AffectedAreaSet indicates the JSON payload explicitly included "affected_area".
	AffectedAreaSet bool `json:"-"`
}

// AffectedRoute describes a transit route linked to a disruption.
type AffectedRoute struct {
	RouteID     int    `json:"route_id"`
	RouteNumber string `json:"route_number"`
	RouteName   string `json:"route_name"`
	Direction   string `json:"direction"`
}

// AffectedStop describes a transit stop linked to a disruption on a route.
type AffectedStop struct {
	StopID   int    `json:"stop_id"`
	StopName string `json:"stop_name"`
	RouteID  int    `json:"route_id"`
}

// DisruptionResponse is the serialized API representation of a disruption and its affected transit entities.
type DisruptionResponse struct {
	ID                int             `json:"id"`
	Type              string          `json:"type"`
	Title             string          `json:"title"`
	Description       *string         `json:"description"`
	AffectedArea      *string         `json:"affected_area"`
	StartTime         time.Time       `json:"start_time"`
	EndTime           *time.Time      `json:"end_time"`
	Status            string          `json:"status"`
	SourceService     string          `json:"source_service"`
	ExternalReference *string         `json:"external_reference"`
	ReceivedAt        time.Time       `json:"received_at"`
	AffectedRoutes    []AffectedRoute `json:"affected_routes"`
	AffectedStops     []AffectedStop  `json:"affected_stops"`
	AffectedTrips     []int           `json:"affected_trips"`
	AffectedVehicles  []int           `json:"affected_vehicles"`
	AffectedSegments  []string        `json:"affected_segments"`
}

// AffectedRouteSummary summarizes the operational status and active disruption IDs for an affected route.
type AffectedRouteSummary struct {
	RouteID           int    `json:"route_id"`
	RouteNumber       string `json:"route_number"`
	RouteName         string `json:"route_name"`
	Direction         string `json:"direction"`
	OperationalStatus string `json:"operational_status"`
	DisruptionIDs     []int  `json:"disruption_ids"`
}

// AffectedStopSummary summarizes the operational status and active disruption IDs for an affected stop.
type AffectedStopSummary struct {
	StopID            int    `json:"stop_id"`
	StopName          string `json:"stop_name"`
	OperationalStatus string `json:"operational_status"`
	DisruptionIDs     []int  `json:"disruption_ids"`
}

// AffectedTripSummary summarizes the operational status and active disruption IDs for an affected trip.
type AffectedTripSummary struct {
	TripID            int    `json:"trip_id"`
	RouteID           int    `json:"route_id"`
	OperationalStatus string `json:"operational_status"`
	DisruptionIDs     []int  `json:"disruption_ids"`
}

// AffectedVehicleSummary summarizes the operational status and active disruption IDs for an affected vehicle.
type AffectedVehicleSummary struct {
	VehicleID         int    `json:"vehicle_id"`
	VehicleNumber     string `json:"vehicle_number"`
	OperationalStatus string `json:"operational_status"`
	DisruptionIDs     []int  `json:"disruption_ids"`
}

// AffectedSegmentSummary summarizes the operational status of a disrupted route segment.
type AffectedSegmentSummary struct {
	DisruptionID      int    `json:"disruption_id"`
	RouteID           *int   `json:"route_id"`
	Segment           string `json:"segment"`
	OperationalStatus string `json:"operational_status"`
}

// AffectedServicesSummary aggregates all currently affected routes, stops, trips, vehicles, and segments (US 191, 195).
type AffectedServicesSummary struct {
	GeneratedAt           time.Time                `json:"generated_at"`
	ActiveDisruptionCount int                      `json:"active_disruption_count"`
	AffectedRoutes        []AffectedRouteSummary   `json:"affected_routes"`
	AffectedStops         []AffectedStopSummary    `json:"affected_stops"`
	AffectedTrips         []AffectedTripSummary    `json:"affected_trips"`
	AffectedVehicles      []AffectedVehicleSummary `json:"affected_vehicles"`
	AffectedSegments      []AffectedSegmentSummary `json:"affected_segments"`
}

// ServiceStatusResponse reports the overall transit network status, active disruptions, detours, and affected services (US 194, 195, 196).
type ServiceStatusResponse struct {
	GeneratedAt           time.Time                `json:"generated_at"`
	OverallStatus         string                   `json:"overall_status"`
	ActiveDisruptionCount int                      `json:"active_disruption_count"`
	ActiveDisruptions     []DisruptionResponse     `json:"active_disruptions"`
	Detours               []DisruptionResponse     `json:"detours"`
	AffectedRoutes        []AffectedRouteSummary   `json:"affected_routes"`
	AffectedStops         []AffectedStopSummary    `json:"affected_stops"`
	AffectedTrips         []AffectedTripSummary    `json:"affected_trips"`
	AffectedVehicles      []AffectedVehicleSummary `json:"affected_vehicles"`
	AffectedSegments      []AffectedSegmentSummary `json:"affected_segments"`
}

// EntityOperationalStatus reports the current operational status and active disruptions for a single route, stop, or trip (US 192 / PT-FR-016).
type EntityOperationalStatus struct {
	EntityType        string               `json:"entity_type"`
	EntityID          int                  `json:"entity_id"`
	OperationalStatus string               `json:"operational_status"`
	ActiveDisruptions []DisruptionResponse `json:"active_disruptions"`
}

// SaveDisruptionParams carries validated disruption fields and entity links into the repository layer.
type SaveDisruptionParams struct {
	Type              string
	Title             string
	Description       *string
	AffectedArea      *string
	StartTime         time.Time
	EndTime           *time.Time
	Status            string
	SourceService     string
	ExternalReference *string
	ReceivedAt        time.Time
	RouteIDs          []int
	StopIDs           []int
	TripIDs           []int
	VehicleIDs        []int
	AffectedSegments  []string
}

// PatchDisruptionParams carries validated partial update fields into the repository layer.
type PatchDisruptionParams struct {
	Type             *string
	Title            *string
	Description      *string
	DescriptionSet   bool
	AffectedArea     *string
	AffectedAreaSet  bool
	Status           *string
	StartTime        *time.Time
	EndTime          *time.Time
	EndTimeSet       bool
	RouteIDs         *[]int
	StopIDs          *[]int
	TripIDs          *[]int
	VehicleIDs       *[]int
	AffectedSegments *[]string
}

// NearbyStop holds stop coordinates for spatial radius impact evaluation.
type NearbyStop struct {
	ID        int
	Name      string
	Latitude  float64
	Longitude float64
}

// AffectedTripDetail associates an impacted trip with its route and disruption.
type AffectedTripDetail struct {
	DisruptionID int
	TripID       int
	RouteID      int
}

// AffectedVehicleDetail associates an impacted vehicle with its fleet number and disruption.
type AffectedVehicleDetail struct {
	DisruptionID  int
	VehicleID     int
	VehicleNumber string
}

// Repository defines database operations required by the disruptions service.
type Repository interface {
	List(ctx context.Context, filter ListFilter) ([]DisruptionResponse, error)
	Get(ctx context.Context, id int) (DisruptionResponse, error)
	Create(ctx context.Context, params SaveDisruptionParams) (DisruptionResponse, error)
	Update(ctx context.Context, id int, params PatchDisruptionParams) (DisruptionResponse, error)
	FindStopsInBounds(ctx context.Context, minLat, maxLat, minLng, maxLng float64) ([]NearbyStop, error)
	ListActiveImpactedTrips(ctx context.Context, disruptionIDs []int) ([]AffectedTripDetail, error)
	ListActiveImpactedVehicles(ctx context.Context, disruptionIDs []int) ([]AffectedVehicleDetail, error)
	RouteExists(ctx context.Context, routeID int) error
	StopExists(ctx context.Context, stopID int) error
	TripRouteID(ctx context.Context, tripID int) (int, error)
	ListActiveForTrip(ctx context.Context, tripID int, routeID int) ([]DisruptionResponse, error)
}
