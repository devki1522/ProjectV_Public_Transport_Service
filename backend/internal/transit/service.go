package transit

import (
	"context"
	"log"
	"time"
)

// Service validates schedule requests and combines repository data with estimates.
type Service struct {
	repo      Repository
	estimates EstimateProvider
}

// NewService creates a schedule service with its repository and optional estimate provider.
func NewService(repo Repository, estimates EstimateProvider) *Service {
	return &Service{repo: repo, estimates: estimates}
}

func validID(id int) bool { return id > 0 && int64(id) <= 2147483647 }

// Timetable returns scheduled trips for a route and service date.
func (s *Service) Timetable(ctx context.Context, routeID int, date string) (Timetable, error) {
	if !validID(routeID) {
		return Timetable{}, &InvalidInput{"route id must be a positive 32-bit integer"}
	}
	if parsed, err := time.Parse(time.DateOnly, date); err != nil || parsed.Year() < 1 {
		return Timetable{}, &InvalidInput{"date must be a valid YYYY-MM-DD date"}
	}
	route, err := s.repo.Route(ctx, routeID)
	if err != nil {
		return Timetable{}, err
	}
	trips, err := s.repo.Timetable(ctx, routeID, date)
	if err != nil {
		return Timetable{}, err
	}
	if trips == nil {
		trips = []ScheduledTrip{}
	}
	return Timetable{Route: route, ServiceDate: date, Timezone: AgencyTimezone, Trips: trips}, nil
}

// Departures returns departures matching the requested filters and time window.
func (s *Service) Departures(ctx context.Context, q DepartureQuery) ([]Departure, error) {
	if q.StopID == 0 && q.RouteID == 0 && q.TripID == 0 {
		return nil, &InvalidInput{"provide stop_id, route_id, or trip_id"}
	}
	for _, id := range []int{q.StopID, q.RouteID, q.TripID} {
		if id != 0 && !validID(id) {
			return nil, &InvalidInput{"identifiers must be positive 32-bit integers"}
		}
	}
	if q.From.IsZero() || q.From.Year() < 1 || q.Until.Year() > 9999 || !q.Until.After(q.From) || q.Until.Sub(q.From) > 24*time.Hour {
		return nil, &InvalidInput{"departure window must be greater than zero and at most 24 hours"}
	}
	if q.Limit < 1 || q.Limit > 100 {
		return nil, &InvalidInput{"limit must be between 1 and 100"}
	}
	if q.StopID != 0 {
		if _, err := s.repo.Stop(ctx, q.StopID); err != nil {
			return nil, err
		}
	}
	if q.RouteID != 0 {
		if _, err := s.repo.Route(ctx, q.RouteID); err != nil {
			return nil, err
		}
	}
	if q.TripID != 0 {
		if err := s.repo.TripExists(ctx, q.TripID); err != nil {
			return nil, err
		}
	}
	departures, err := s.repo.Departures(ctx, q)
	if err != nil {
		return nil, err
	}
	if departures == nil {
		departures = []Departure{}
	}
	if s.estimates != nil && len(departures) > 0 {
		estimates, err := s.estimates.Estimates(ctx, departures)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			log.Printf("departure estimates unavailable: %v", err)
		} else {
			for i := range departures {
				d := &departures[i]
				if estimate, ok := estimates[DepartureKey{d.TripID, d.RouteStopID, d.ServiceDate}]; ok && !estimate.IsZero() {
					d.EstimatedDeparture = &estimate
				}
			}
		}
	}
	return departures, nil
}

// Stop loads stop details and associated departures.
func (s *Service) Stop(ctx context.Context, id int, q DepartureQuery) (StopDetails, error) {
	if !validID(id) {
		return StopDetails{}, &InvalidInput{"stop id must be a positive 32-bit integer"}
	}
	stop, err := s.repo.Stop(ctx, id)
	if err != nil {
		return StopDetails{}, err
	}
	q.StopID = id
	departures, err := s.Departures(ctx, q)
	if err != nil {
		return StopDetails{}, err
	}
	return StopDetails{Stop: stop, Departures: departures}, nil
}
