// Package disruptions implements transit disruption creation, status lifecycle,
// affected-service evaluation, operational status queries, and external notice ingestion.
package disruptions

import (
	"context"
	"errors"
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
	MaxSeverityRunes      = 30
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

// UtilitiesNoticeInput is the request payload for POST /api/integrations/utilities/notices (US 186 / PT-FR-018).
type UtilitiesNoticeInput struct {
	ExternalID        *string    `json:"external_id,omitempty"`
	ExternalReference *string    `json:"external_reference,omitempty"`
	NoticeType        string     `json:"notice_type,omitempty" enums:"road_closure,construction" default:"road_closure"`
	Type              string     `json:"type,omitempty" enums:"road_closure,construction"`
	Title             string     `json:"title"`
	Description       *string    `json:"description,omitempty"`
	AffectedArea      *string    `json:"affected_area,omitempty"`
	StartTime         *time.Time `json:"start_time,omitempty"`
	EndTime           *time.Time `json:"end_time,omitempty"`
	Status            string     `json:"status,omitempty" enums:"scheduled,active,resolved"`
	RouteIDs          []int      `json:"route_ids,omitempty"`
	StopIDs           []int      `json:"stop_ids,omitempty"`
	TripIDs           []int      `json:"trip_ids,omitempty"`
	VehicleIDs        []int      `json:"vehicle_ids,omitempty"`
	AffectedSegments  []string   `json:"affected_segments,omitempty"`
	Latitude          *float64   `json:"latitude,omitempty"`
	Longitude         *float64   `json:"longitude,omitempty"`
	RadiusMeters      *float64   `json:"radius_meters,omitempty"`
}

// EmergencyIncidentInput is the request payload for POST /api/integrations/emergency/incidents (US 187 / PT-FR-019).
type EmergencyIncidentInput struct {
	ExternalID        *string    `json:"external_id,omitempty"`
	ExternalReference *string    `json:"external_reference,omitempty"`
	IncidentType      string     `json:"incident_type,omitempty" enums:"emergency,road_closure" default:"emergency"`
	Type              string     `json:"type,omitempty" enums:"emergency,road_closure"`
	Severity          string     `json:"severity,omitempty" enums:"low,medium,moderate,high,critical"`
	Title             string     `json:"title"`
	Description       *string    `json:"description,omitempty"`
	AffectedArea      *string    `json:"affected_area,omitempty"`
	StartTime         *time.Time `json:"start_time,omitempty"`
	EndTime           *time.Time `json:"end_time,omitempty"`
	Status            string     `json:"status,omitempty" enums:"scheduled,active,resolved"`
	RouteIDs          []int      `json:"route_ids,omitempty"`
	StopIDs           []int      `json:"stop_ids,omitempty"`
	TripIDs           []int      `json:"trip_ids,omitempty"`
	VehicleIDs        []int      `json:"vehicle_ids,omitempty"`
	AffectedSegments  []string   `json:"affected_segments,omitempty"`
	Latitude          *float64   `json:"latitude,omitempty"`
	Longitude         *float64   `json:"longitude,omitempty"`
	RadiusMeters      *float64   `json:"radius_meters,omitempty"`
}

// SecurityTrafficAlertInput is the request payload for POST /api/integrations/security/traffic-alerts (US 188 / PT-FR-020).
type SecurityTrafficAlertInput struct {
	ExternalID        *string    `json:"external_id,omitempty"`
	ExternalReference *string    `json:"external_reference,omitempty"`
	AlertType         string     `json:"alert_type,omitempty" enums:"traffic_restriction,road_closure" default:"traffic_restriction"`
	Type              string     `json:"type,omitempty" enums:"traffic_restriction,road_closure"`
	Title             string     `json:"title"`
	Description       *string    `json:"description,omitempty"`
	AffectedArea      *string    `json:"affected_area,omitempty"`
	StartTime         *time.Time `json:"start_time,omitempty"`
	EndTime           *time.Time `json:"end_time,omitempty"`
	Status            string     `json:"status,omitempty" enums:"scheduled,active,resolved"`
	RouteIDs          []int      `json:"route_ids,omitempty"`
	StopIDs           []int      `json:"stop_ids,omitempty"`
	TripIDs           []int      `json:"trip_ids,omitempty"`
	VehicleIDs        []int      `json:"vehicle_ids,omitempty"`
	AffectedSegments  []string   `json:"affected_segments,omitempty"`
	Latitude          *float64   `json:"latitude,omitempty"`
	Longitude         *float64   `json:"longitude,omitempty"`
	RadiusMeters      *float64   `json:"radius_meters,omitempty"`
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

// ExternalNoticeResult describes the outcome of ingesting a notice or alert from an external enterprise service (US 186, 187, 188).
type ExternalNoticeResult struct {
	Created         bool               `json:"created"`
	Disruption      DisruptionResponse `json:"disruption"`
	MatchedRouteIDs []int              `json:"matched_route_ids"`
	MatchedStopIDs  []int              `json:"matched_stop_ids"`
	TransitImpact   string             `json:"transit_impact"`
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
	UpsertByExternalRef(ctx context.Context, params SaveDisruptionParams) (DisruptionResponse, bool, error)
	FindStopsInBounds(ctx context.Context, minLat, maxLat, minLng, maxLng float64) ([]NearbyStop, error)
	ListActiveImpactedTrips(ctx context.Context, disruptionIDs []int) ([]AffectedTripDetail, error)
	ListActiveImpactedVehicles(ctx context.Context, disruptionIDs []int) ([]AffectedVehicleDetail, error)
	RouteExists(ctx context.Context, routeID int) error
	StopExists(ctx context.Context, stopID int) error
	TripRouteID(ctx context.Context, tripID int) (int, error)
	ListActiveForTrip(ctx context.Context, tripID int, routeID int) ([]DisruptionResponse, error)
}
