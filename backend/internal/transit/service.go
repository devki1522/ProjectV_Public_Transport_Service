package transit

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
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

// SearchStops returns active stops matching an optional case-insensitive name filter.
func (s *Service) SearchStops(ctx context.Context, q StopSearchQuery) ([]Stop, error) {
	q.Query = strings.TrimSpace(q.Query)
	if len(q.Query) > 100 {
		return nil, &InvalidInput{"q must be at most 100 characters"}
	}
	if q.Limit < 1 || q.Limit > 100 {
		return nil, &InvalidInput{"limit must be between 1 and 100"}
	}
	stops, err := s.repo.SearchStops(ctx, q)
	if err != nil {
		return nil, err
	}
	if stops == nil {
		stops = []Stop{}
	}
	return stops, nil
}

// Journeys finds direct, multi-transfer, and walking-connected transit itineraries between two locations.
func (s *Service) Journeys(ctx context.Context, q JourneyQuery) (JourneySearchResult, error) {
	q.Origin = strings.TrimSpace(q.Origin)
	q.Destination = strings.TrimSpace(q.Destination)

	if q.OriginStopID == 0 && q.Origin == "" {
		return JourneySearchResult{}, &InvalidInput{"provide origin or origin_stop_id"}
	}
	if q.DestinationStopID == 0 && q.Destination == "" {
		return JourneySearchResult{}, &InvalidInput{"provide destination or destination_stop_id"}
	}
	if (q.OriginStopID != 0 && !validID(q.OriginStopID)) || (q.DestinationStopID != 0 && !validID(q.DestinationStopID)) {
		return JourneySearchResult{}, &InvalidInput{"identifiers must be positive 32-bit integers"}
	}
	if len(q.Origin) > 100 || len(q.Destination) > 100 {
		return JourneySearchResult{}, &InvalidInput{"location queries must be at most 100 characters"}
	}
	if q.OriginStopID != 0 && q.OriginStopID == q.DestinationStopID {
		return JourneySearchResult{}, &InvalidInput{"origin and destination must be different locations"}
	}
	if q.OriginStopID == 0 && q.DestinationStopID == 0 && strings.EqualFold(q.Origin, q.Destination) {
		return JourneySearchResult{}, &InvalidInput{"origin and destination must be different locations"}
	}
	if q.From.IsZero() || q.From.Year() < 1 || q.Until.Year() > 9999 || !q.Until.After(q.From) || q.Until.Sub(q.From) > 24*time.Hour {
		return JourneySearchResult{}, &InvalidInput{"journey window must be greater than zero and at most 24 hours"}
	}
	if q.Limit < 1 || q.Limit > 20 {
		return JourneySearchResult{}, &InvalidInput{"limit must be between 1 and 20"}
	}
	if q.MaxTransfers < 0 || q.MaxTransfers > MaxAllowedTransfers {
		return JourneySearchResult{}, &InvalidInput{"max_transfers must be between 0 and 3"}
	}
	if q.MaxWalkMeters < 0 || q.MaxWalkMeters > MaxAllowedWalkMeters {
		return JourneySearchResult{}, &InvalidInput{"max_walk_meters must be between 0 and 1500"}
	}

	originLabel, originStops, err := s.resolveLocationStops(ctx, q.OriginStopID, q.Origin)
	if err != nil {
		return JourneySearchResult{}, err
	}
	destLabel, destStops, err := s.resolveLocationStops(ctx, q.DestinationStopID, q.Destination)
	if err != nil {
		return JourneySearchResult{}, err
	}

	result := JourneySearchResult{
		Origin:           originLabel,
		Destination:      destLabel,
		OriginStops:      originStops,
		DestinationStops: destStops,
		From:             q.From,
		Until:            q.Until,
		Timezone:         AgencyTimezone,
		Itineraries:      []Itinerary{},
	}

	if len(originStops) == 0 || len(destStops) == 0 {
		result.Message = "No active stops matched the requested origin or destination. Try a different stop name."
		return result, nil
	}
	if len(originStops) == 1 && len(destStops) == 1 && originStops[0].ID == destStops[0].ID {
		return JourneySearchResult{}, &InvalidInput{"origin and destination must be different locations"}
	}

	originIDs := make([]int, len(originStops))
	for i, stop := range originStops {
		originIDs[i] = stop.ID
	}
	destIDs := make([]int, len(destStops))
	for i, stop := range destStops {
		destIDs[i] = stop.ID
	}

	plan, err := s.repo.Journeys(ctx, JourneyRepoQuery{
		OriginStopIDs:      originIDs,
		DestinationStopIDs: destIDs,
		From:               q.From,
		Until:              q.Until,
		Limit:              q.Limit,
		MaxTransfers:       q.MaxTransfers,
		MaxWalkMeters:      q.MaxWalkMeters,
	})
	if err != nil {
		return JourneySearchResult{}, err
	}
	result.ExcludedDisruptions = plan.ExcludedDisruptions
	itineraries := plan.Itineraries
	if len(itineraries) == 0 {
		if len(plan.ExcludedDisruptions) > 0 {
			result.Message = "No available transit itineraries found in this time window due to active service disruptions."
		} else {
			result.Message = "No transit itineraries found between the selected origin and destination in this time window."
		}
		return result, nil
	}

	stopLookup := make(map[int]Stop, len(originStops)+len(destStops))
	for _, stop := range originStops {
		stopLookup[stop.ID] = stop
	}
	for _, stop := range destStops {
		stopLookup[stop.ID] = stop
	}
	for i := range itineraries {
		if full, ok := stopLookup[itineraries[i].OriginStop.ID]; ok {
			itineraries[i].OriginStop = full
		}
		if full, ok := stopLookup[itineraries[i].DestinationStop.ID]; ok {
			itineraries[i].DestinationStop = full
		}
	}

	sortItineraries(itineraries)
	if len(itineraries) > q.Limit {
		itineraries = itineraries[:q.Limit]
	}
	if err := s.applyJourneyEstimates(ctx, itineraries); err != nil {
		return JourneySearchResult{}, err
	}
	result.Itineraries = itineraries
	if len(result.Itineraries) == 1 {
		result.Message = "Found 1 itinerary option."
	} else {
		result.Message = fmt.Sprintf("Found %d itinerary options.", len(result.Itineraries))
	}
	return result, nil
}

func (s *Service) applyJourneyEstimates(ctx context.Context, itineraries []Itinerary) error {
	if s.estimates == nil || len(itineraries) == 0 {
		return nil
	}
	seen := make(map[DepartureKey]struct{})
	var lookups []Departure
	addLookup := func(leg JourneyLeg, st StopTime, scheduled time.Time) {
		key := DepartureKey{TripID: leg.TripID, RouteStopID: st.RouteStopID, ServiceDate: leg.ServiceDate}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		var route Route
		if leg.Route != nil {
			route = *leg.Route
		}
		lookups = append(lookups, Departure{
			TripID:             leg.TripID,
			Route:              route,
			RouteStopID:        st.RouteStopID,
			StopID:             st.StopID,
			StopName:           st.StopName,
			StopSequence:       st.StopSequence,
			ServiceDate:        leg.ServiceDate,
			ScheduledDeparture: scheduled,
		})
	}
	for _, it := range itineraries {
		for _, leg := range it.Legs {
			if leg.Mode != "transit" || len(leg.Stops) == 0 {
				continue
			}
			addLookup(leg, leg.Stops[0], leg.ScheduledDeparture)
			addLookup(leg, leg.Stops[len(leg.Stops)-1], leg.ScheduledArrival)
		}
	}
	if len(lookups) == 0 {
		return nil
	}
	estimates, err := s.estimates.Estimates(ctx, lookups)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Printf("journey estimates unavailable: %v", err)
		return nil
	}
	for i := range itineraries {
		it := &itineraries[i]
		for j := range it.Legs {
			leg := &it.Legs[j]
			if leg.Mode != "transit" || len(leg.Stops) == 0 {
				continue
			}
			depKey := DepartureKey{TripID: leg.TripID, RouteStopID: leg.Stops[0].RouteStopID, ServiceDate: leg.ServiceDate}
			if est, ok := estimates[depKey]; ok && !est.IsZero() {
				v := est
				leg.EstimatedDeparture = &v
			}
			arrKey := DepartureKey{TripID: leg.TripID, RouteStopID: leg.Stops[len(leg.Stops)-1].RouteStopID, ServiceDate: leg.ServiceDate}
			if est, ok := estimates[arrKey]; ok && !est.IsZero() {
				v := est
				leg.EstimatedArrival = &v
			}
		}
		if len(it.Legs) > 0 {
			if it.Legs[0].Mode == "transit" && it.Legs[0].EstimatedDeparture != nil {
				v := *it.Legs[0].EstimatedDeparture
				it.EstimatedDeparture = &v
			} else if it.Legs[0].Mode == "walk" && len(it.Legs) > 1 && it.Legs[1].EstimatedDeparture != nil {
				v := it.Legs[1].EstimatedDeparture.Add(-time.Duration(it.Legs[0].DurationMinutes) * time.Minute)
				it.EstimatedDeparture = &v
			}
			lastIdx := len(it.Legs) - 1
			if it.Legs[lastIdx].Mode == "transit" && it.Legs[lastIdx].EstimatedArrival != nil {
				v := *it.Legs[lastIdx].EstimatedArrival
				it.EstimatedArrival = &v
			} else if it.Legs[lastIdx].Mode == "walk" && lastIdx > 0 && it.Legs[lastIdx-1].EstimatedArrival != nil {
				v := it.Legs[lastIdx-1].EstimatedArrival.Add(time.Duration(it.Legs[lastIdx].DurationMinutes) * time.Minute)
				it.EstimatedArrival = &v
			}
		}
	}
	return nil
}

func (s *Service) resolveLocationStops(ctx context.Context, stopID int, queryText string) (string, []Stop, error) {
	if stopID != 0 {
		stop, err := s.repo.Stop(ctx, stopID)
		if err != nil {
			return "", nil, err
		}
		if queryText != "" && !strings.Contains(strings.ToLower(stop.Name), strings.ToLower(queryText)) {
			return "", nil, &InvalidInput{"location query conflicts with stop id"}
		}
		if !stop.IsActive {
			return stop.Name, []Stop{}, nil
		}
		return stop.Name, []Stop{stop}, nil
	}
	stops, err := s.repo.SearchStops(ctx, StopSearchQuery{Query: queryText, Limit: 25})
	if err != nil {
		return "", nil, err
	}
	if stops == nil {
		stops = []Stop{}
	}
	return queryText, stops, nil
}

func firstTransitTripID(it Itinerary) int {
	for _, leg := range it.Legs {
		if leg.TripID != 0 {
			return leg.TripID
		}
	}
	return 0
}

func sortItineraries(itineraries []Itinerary) {
	sort.SliceStable(itineraries, func(i, j int) bool {
		a, b := itineraries[i], itineraries[j]
		if !a.ScheduledArrival.Equal(b.ScheduledArrival) {
			return a.ScheduledArrival.Before(b.ScheduledArrival)
		}
		if a.Transfers != b.Transfers {
			return a.Transfers < b.Transfers
		}
		if !a.ScheduledDeparture.Equal(b.ScheduledDeparture) {
			return a.ScheduledDeparture.After(b.ScheduledDeparture)
		}
		if a.WalkingMinutes != b.WalkingMinutes {
			return a.WalkingMinutes < b.WalkingMinutes
		}
		return firstTransitTripID(a) < firstTransitTripID(b)
	})
}
