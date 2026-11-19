// Package transit provides resident-facing schedules and departure information.
package transit

import (
	"context"
	"errors"
	"time"
)

// AgencyTimezone is the time zone used to interpret transit service dates.
const AgencyTimezone = "America/Toronto"

// ErrNotFound indicates that a requested transit resource does not exist.
var ErrNotFound = errors.New("transit resource not found")

// InvalidInput reports a validation failure in a transit request.
type InvalidInput struct{ Message string }

// Error returns the validation error message.
func (e *InvalidInput) Error() string { return e.Message }

// Route contains the public route details returned by schedule queries.
type Route struct {
	ID        int    `json:"id"`
	Number    string `json:"number"`
	Name      string `json:"name"`
	Direction string `json:"direction"`
	IsActive  bool   `json:"is_active"`
}

// Stop contains the public stop details returned by schedule queries.
type Stop struct {
	ID                 int     `json:"id"`
	Name               string  `json:"name"`
	Latitude           float64 `json:"latitude"`
	Longitude          float64 `json:"longitude"`
	WheelchairBoarding int     `json:"wheelchair_boarding"`
	IsActive           bool    `json:"is_active"`
}

// StopTime contains the scheduled times for one stop occurrence.
type StopTime struct {
	RouteStopID        int       `json:"route_stop_id"`
	StopID             int       `json:"stop_id"`
	StopName           string    `json:"stop_name"`
	StopSequence       int       `json:"stop_sequence"`
	ScheduledArrival   time.Time `json:"scheduled_arrival_time"`
	ScheduledDeparture time.Time `json:"scheduled_departure_time"`
}

// ScheduledTrip groups a trip with its ordered stop times.
type ScheduledTrip struct {
	ID           int        `json:"id"`
	ScheduleType string     `json:"schedule_type"`
	Status       string     `json:"status"`
	StopTimes    []StopTime `json:"stop_times"`
}

// Timetable contains the trips for a route on one service date.
type Timetable struct {
	Route       Route           `json:"route"`
	ServiceDate string          `json:"service_date"`
	Timezone    string          `json:"timezone"`
	Trips       []ScheduledTrip `json:"trips"`
}

// Departure combines a scheduled departure with an optional live estimate.
type Departure struct {
	TripID             int        `json:"trip_id"`
	Route              Route      `json:"route"`
	RouteStopID        int        `json:"route_stop_id"`
	StopID             int        `json:"stop_id"`
	StopName           string     `json:"stop_name"`
	StopSequence       int        `json:"stop_sequence"`
	ServiceDate        string     `json:"service_date"`
	ScheduledDeparture time.Time  `json:"scheduled_departure_time"`
	EstimatedDeparture *time.Time `json:"estimated_departure_time"`
}

// DepartureQuery selects departures by stop, route, trip, time window, and limit.
type DepartureQuery struct {
	StopID, RouteID, TripID int
	From, Until             time.Time
	Limit                   int
}

// StopSearchQuery filters active stops by an optional search string and limit.
type StopSearchQuery struct {
	Query string
	Limit int
}

// JourneyQuery specifies origin, destination, departure time window, transfer/walk bounds, and result limit.
type JourneyQuery struct {
	OriginStopID      int
	DestinationStopID int
	Origin            string
	Destination       string
	From, Until       time.Time
	Limit             int
	MaxTransfers      int
	MaxWalkMeters     int
}

// JourneyRepoQuery contains resolved stop IDs, time window, and routing bounds for itinerary queries.
type JourneyRepoQuery struct {
	OriginStopIDs      []int
	DestinationStopIDs []int
	From, Until        time.Time
	Limit              int
	MaxTransfers       int
	MaxWalkMeters      int
}

// JourneyLeg represents one continuous transit ride or walking segment between two stops.
type JourneyLeg struct {
	Mode               string     `json:"mode"`
	TripID             int        `json:"trip_id,omitempty"`
	Route              *Route     `json:"route,omitempty"`
	ServiceDate        string     `json:"service_date,omitempty"`
	FromStopID         int        `json:"from_stop_id"`
	FromStopName       string     `json:"from_stop_name"`
	FromStopSequence   int        `json:"from_stop_sequence,omitempty"`
	ToStopID           int        `json:"to_stop_id"`
	ToStopName         string     `json:"to_stop_name"`
	ToStopSequence     int        `json:"to_stop_sequence,omitempty"`
	StopCount          int        `json:"stop_count,omitempty"`
	DistanceMeters     int        `json:"distance_meters,omitempty"`
	ScheduledDeparture time.Time  `json:"scheduled_departure_time"`
	ScheduledArrival   time.Time  `json:"scheduled_arrival_time"`
	EstimatedDeparture *time.Time `json:"estimated_departure_time"`
	EstimatedArrival   *time.Time `json:"estimated_arrival_time"`
	DurationMinutes    int        `json:"duration_minutes"`
	Stops              []StopTime `json:"stops,omitempty"`
}

// Itinerary represents a complete journey option from origin to destination.
type Itinerary struct {
	OriginStop         Stop         `json:"origin_stop"`
	DestinationStop    Stop         `json:"destination_stop"`
	ScheduledDeparture time.Time    `json:"scheduled_departure_time"`
	ScheduledArrival   time.Time    `json:"scheduled_arrival_time"`
	EstimatedDeparture *time.Time   `json:"estimated_departure_time"`
	EstimatedArrival   *time.Time   `json:"estimated_arrival_time"`
	DurationMinutes    int          `json:"duration_minutes"`
	WalkingMinutes     int          `json:"walking_minutes"`
	Transfers          int          `json:"transfers"`
	Legs               []JourneyLeg `json:"legs"`
}

// JourneyStopTimeRow is one ordered stop-time record loaded for journey routing.
type JourneyStopTimeRow struct {
	TripID       int
	RouteID      int
	ScheduleType string
	RouteStopID  int
	StopID       int
	StopSequence int
	ArrivalSec   int64
	DepartureSec int64
}

// JourneyDisruption represents an active or scheduled disruption affecting a route or stop.
type JourneyDisruption struct {
	ID        int
	Type      string
	Title     string
	StartTime time.Time
	EndTime   *time.Time
	Status    string
	RouteID   int
	StopID    *int
}

// DisruptionNotice summarizes an active or scheduled disruption excluded from journey results.
type DisruptionNotice struct {
	ID        int        `json:"id"`
	Type      string     `json:"type"`
	Title     string     `json:"title"`
	Status    string     `json:"status"`
	StartTime time.Time  `json:"start_time"`
	EndTime   *time.Time `json:"end_time,omitempty"`
}

// JourneyPlan holds planned itineraries alongside any disruptions excluded during planning.
type JourneyPlan struct {
	Itineraries         []Itinerary
	ExcludedDisruptions []DisruptionNotice
}

// JourneyNetwork holds active stops, routes, ordered trip stop-times, and disruptions for planning.
type JourneyNetwork struct {
	Stops       []Stop
	Routes      []Route
	StopTimes   []JourneyStopTimeRow
	Disruptions []JourneyDisruption
}

// JourneySearchResult contains the resolved locations, itinerary options, excluded disruptions, and a summary message.
type JourneySearchResult struct {
	Origin              string             `json:"origin"`
	Destination         string             `json:"destination"`
	OriginStops         []Stop             `json:"origin_stops"`
	DestinationStops    []Stop             `json:"destination_stops"`
	From                time.Time          `json:"from"`
	Until               time.Time          `json:"until"`
	Timezone            string             `json:"timezone"`
	Itineraries         []Itinerary        `json:"itineraries"`
	ExcludedDisruptions []DisruptionNotice `json:"excluded_disruptions,omitempty"`
	Message             string             `json:"message"`
}

// StopDetails combines stop metadata with its upcoming departures.
type StopDetails struct {
	Stop       Stop        `json:"stop"`
	Departures []Departure `json:"departures"`
}

// DepartureKey identifies an occurrence, including repeated stops and overnight trips.
type DepartureKey struct {
	TripID, RouteStopID int
	ServiceDate         string
}

// EstimateProvider is supplied by a live-data integration. A missing key means
// no estimate is available; scheduled times are always retained.
type EstimateProvider interface {
	Estimates(context.Context, []Departure) (map[DepartureKey]time.Time, error)
}

// Repository defines the schedule data reads required by the transit service.
type Repository interface {
	Route(context.Context, int) (Route, error)
	Stop(context.Context, int) (Stop, error)
	SearchStops(context.Context, StopSearchQuery) ([]Stop, error)
	TripExists(context.Context, int) error
	Timetable(context.Context, int, string) ([]ScheduledTrip, error)
	Departures(context.Context, DepartureQuery) ([]Departure, error)
	Journeys(context.Context, JourneyRepoQuery) (JourneyPlan, error)
}
