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
	TripExists(context.Context, int) error
	Timetable(context.Context, int, string) ([]ScheduledTrip, error)
	Departures(context.Context, DepartureQuery) ([]Departure, error)
}
