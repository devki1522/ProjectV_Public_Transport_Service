// Package vehicles implements live vehicle location ingestion, querying, route geometry lookup, and movement simulation.
package vehicles

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNotFound indicates the requested route, trip, or vehicle record does not exist.
	ErrNotFound = errors.New("transit resource not found")
	// ErrStaleLocationUpdate indicates a newer location timestamp has already been stored for the vehicle.
	ErrStaleLocationUpdate = errors.New("stale vehicle location update")
)

// InvalidInput describes a request parameter or payload validation failure.
type InvalidInput struct{ Message string }

func (e *InvalidInput) Error() string { return e.Message }

const (
	// StatusInService indicates a vehicle operating normally on a route.
	StatusInService = "in_service"
	// StatusDelayed indicates a vehicle experiencing delays.
	StatusDelayed = "delayed"
	// StatusLayover indicates a vehicle waiting at a terminal between trips.
	StatusLayover = "layover"
	// StatusOutOfService indicates a vehicle not currently in passenger service.
	StatusOutOfService = "out_of_service"

	// DefaultActiveMaxAge is the freshness window used when filtering active vehicle locations.
	DefaultActiveMaxAge = 15 * time.Minute
	// MinActiveMaxAge is the smallest allowed freshness window for vehicle queries.
	MinActiveMaxAge = 10 * time.Second
	// MaxActiveMaxAge is the largest allowed freshness window for vehicle queries.
	MaxActiveMaxAge = 24 * time.Hour
	// MaxFutureSkew limits how far ahead of server time a submitted GPS timestamp may be.
	MaxFutureSkew = 2 * time.Minute

	// MaxVehicleNumberRunes matches the vehicles.number column length limit.
	MaxVehicleNumberRunes = 20
	// MaxSourceServiceRunes matches the vehicle_locations.source_service column length limit.
	MaxSourceServiceRunes = 100
)

// LocationUpdateInput is the validated API payload for creating or updating a vehicle location.
type LocationUpdateInput struct {
	VehicleID     *int       `json:"vehicle_id,omitempty"`
	VehicleNumber string     `json:"vehicle_number,omitempty"`
	IsAccessible  *bool      `json:"is_accessible,omitempty"`
	RouteID       int        `json:"route_id"`
	TripID        *int       `json:"trip_id,omitempty"`
	Latitude      *float64   `json:"latitude"`
	Longitude     *float64   `json:"longitude"`
	Heading       *float64   `json:"heading,omitempty"`
	SpeedKmh      *float64   `json:"speed_kmh,omitempty"`
	Status        string     `json:"status,omitempty"`
	SourceService string     `json:"source_service,omitempty"`
	IsSimulated   bool       `json:"is_simulated,omitempty"`
	RecordedAt    *time.Time `json:"recorded_at,omitempty"`
}

// LocationUpdateParams carries normalized fields into the repository layer.
type LocationUpdateParams struct {
	VehicleID     *int
	VehicleNumber string
	IsAccessible  bool
	RouteID       int
	TripID        *int
	Latitude      float64
	Longitude     float64
	Heading       *float64
	SpeedKmh      *float64
	Status        string
	SourceService string
	IsSimulated   bool
	RecordedAt    time.Time
	ReceivedAt    time.Time
}

// ActiveVehicle represents an active transit vehicle and its latest stored location.
type ActiveVehicle struct {
	VehicleID     int       `json:"vehicle_id"`
	VehicleNumber string    `json:"vehicle_number"`
	IsAccessible  bool      `json:"is_accessible"`
	RouteID       int       `json:"route_id"`
	RouteNumber   string    `json:"route_number"`
	RouteName     string    `json:"route_name"`
	Direction     string    `json:"direction"`
	TripID        *int      `json:"trip_id"`
	Latitude      float64   `json:"latitude"`
	Longitude     float64   `json:"longitude"`
	Heading       *float64  `json:"heading"`
	SpeedKmh      *float64  `json:"speed_kmh"`
	Status        string    `json:"status"`
	SourceService string    `json:"source_service"`
	IsSimulated   bool      `json:"is_simulated"`
	RecordedAt    time.Time `json:"recorded_at"`
	ReceivedAt    time.Time `json:"received_at"`
}

// LocationUpdateResult describes the outcome of a location submission.
type LocationUpdateResult struct {
	Vehicle ActiveVehicle `json:"vehicle"`
	Created bool          `json:"created"`
}

// ActiveVehiclesQuery filters active vehicle location lookups.
type ActiveVehiclesQuery struct {
	RouteID     int
	RouteNumber string
	TripID      int
	MaxAge      time.Duration
}

// Coordinate is a geographic latitude/longitude pair for route shapes and vehicle positions.
type Coordinate struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// RouteGeometry describes the ordered coordinates used to render a route path on a map.
type RouteGeometry struct {
	RouteID     int          `json:"route_id"`
	RouteNumber string       `json:"route_number"`
	RouteName   string       `json:"route_name"`
	Direction   string       `json:"direction"`
	Headsign    string       `json:"headsign,omitempty"`
	Coordinates []Coordinate `json:"coordinates"`
}

// SimulatedRoute pairs a route polyline with an optional representative trip ID for simulation.
type SimulatedRoute struct {
	RouteGeometry
	SampleTripID *int
}

// SimulatorStatus reports the current state of the built-in live bus simulator.
type SimulatorStatus struct {
	Running          bool       `json:"running"`
	VehicleCount     int        `json:"vehicle_count"`
	RouteCount       int        `json:"route_count"`
	TickIntervalMs   int        `json:"tick_interval_ms"`
	SpeedMultiplier  float64    `json:"speed_multiplier"`
	LastTickAt       *time.Time `json:"last_tick_at"`
	TotalUpdatesSent int64      `json:"total_updates_sent"`
}

// Repository defines database operations required by the live vehicle service and simulator.
type Repository interface {
	UpsertLocation(context.Context, LocationUpdateParams) (LocationUpdateResult, error)
	ListActiveVehicles(ctx context.Context, cutoff time.Time, routeID int, routeNumber string, tripID int) ([]ActiveVehicle, error)
	ListActiveVehiclesForStop(ctx context.Context, cutoff time.Time, stopID, routeID int) ([]ActiveVehicle, error)
	RouteExists(context.Context, int) error
	TripExists(context.Context, int) error
	StopExists(context.Context, int) error
	RouteGeometry(context.Context, int) (RouteGeometry, error)
	ActiveRoutesWithGeometries(ctx context.Context, limit int) ([]SimulatedRoute, error)
	ClearSimulatedLocations(context.Context) error
	LoadTripStops(ctx context.Context, tripIDs []int) ([]TripStopPoint, error)
	LoadRouteStops(ctx context.Context, routeIDs []int) ([]RouteStopPoint, error)
}
