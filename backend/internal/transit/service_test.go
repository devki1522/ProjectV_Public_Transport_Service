package transit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeRepository struct {
	err                 error
	stopsErr            error
	journeysErr         error
	called              bool
	departures          []Departure
	stops               []Stop
	stopByID            map[int]Stop
	searchCalls         []StopSearchQuery
	journeys            []Itinerary
	excludedDisruptions []DisruptionNotice
	journeyQueries      []JourneyRepoQuery
}

// Route loads one route by identifier.
func (f *fakeRepository) Route(context.Context, int) (Route, error) {
	f.called = true
	return Route{ID: 7, Direction: "direction_0"}, f.err
}

// Stop loads stop details and associated departures.
func (f *fakeRepository) Stop(_ context.Context, id int) (Stop, error) {
	f.called = true
	if f.err != nil {
		return Stop{}, f.err
	}
	if f.stopByID != nil {
		stop, ok := f.stopByID[id]
		if !ok {
			return Stop{}, ErrNotFound
		}
		return stop, nil
	}
	return Stop{ID: id, Name: "Station", WheelchairBoarding: 1, IsActive: true}, nil
}

// SearchStops searches active stops in the fake repository.
func (f *fakeRepository) SearchStops(_ context.Context, q StopSearchQuery) ([]Stop, error) {
	f.called = true
	f.searchCalls = append(f.searchCalls, q)
	if f.stopsErr != nil {
		return nil, f.stopsErr
	}
	if f.err != nil {
		return nil, f.err
	}
	if f.stops == nil {
		return nil, nil
	}
	var matches []Stop
	needle := strings.ToLower(q.Query)
	for _, stop := range f.stops {
		if !stop.IsActive {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(stop.Name), needle) {
			continue
		}
		matches = append(matches, stop)
		if q.Limit > 0 && len(matches) >= q.Limit {
			break
		}
	}
	return matches, nil
}

// TripExists checks whether a trip identifier exists.
func (f *fakeRepository) TripExists(context.Context, int) error { f.called = true; return f.err }

// Timetable returns scheduled trips for a route and service date.
func (f *fakeRepository) Timetable(context.Context, int, string) ([]ScheduledTrip, error) {
	f.called = true
	return nil, f.err
}

// Departures returns departures matching the requested filters and time window.
func (f *fakeRepository) Departures(context.Context, DepartureQuery) ([]Departure, error) {
	f.called = true
	return f.departures, f.err
}

// Journeys returns fake itineraries and excluded disruptions for service tests.
func (f *fakeRepository) Journeys(_ context.Context, q JourneyRepoQuery) (JourneyPlan, error) {
	f.called = true
	f.journeyQueries = append(f.journeyQueries, q)
	if f.journeysErr != nil {
		return JourneyPlan{}, f.journeysErr
	}
	if f.err != nil {
		return JourneyPlan{}, f.err
	}
	return JourneyPlan{
		Itineraries:         f.journeys,
		ExcludedDisruptions: f.excludedDisruptions,
	}, nil
}

type fakeEstimates struct {
	values map[DepartureKey]time.Time
	err    error
}

// Estimates returns fake arrival estimates for service tests.
func (f fakeEstimates) Estimates(context.Context, []Departure) (map[DepartureKey]time.Time, error) {
	return f.values, f.err
}

func testQuery() DepartureQuery {
	from := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	return DepartureQuery{StopID: 1, From: from, Until: from.Add(time.Hour), Limit: 20}
}

// TestTimetableValidationAndEmptyResult verifies timetable validation and empty result.
func TestTimetableValidationAndEmptyResult(t *testing.T) {
	for _, tc := range []struct {
		id   int
		date string
	}{
		{0, "2026-11-10"}, {-1, "2026-11-10"}, {2147483648, "2026-11-10"},
		{1, ""}, {1, "0000-01-01"}, {1, "2026-02-29"}, {1, "2026-1-10"}, {1, "2026-11-10T00:00:00Z"},
	} {
		repo := &fakeRepository{}
		_, err := NewService(repo, nil).Timetable(context.Background(), tc.id, tc.date)
		var invalid *InvalidInput
		if !errors.As(err, &invalid) || repo.called {
			t.Fatalf("input %+v: err=%v called=%v", tc, err, repo.called)
		}
	}
	result, err := NewService(&fakeRepository{}, nil).Timetable(context.Background(), 7, "2026-11-10")
	if err != nil || result.Trips == nil || result.ServiceDate != "2026-11-10" || result.Timezone != AgencyTimezone || result.Route.ID != 7 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

// TestDepartureValidationBeforeDatabase verifies departure validation before database.
func TestDepartureValidationBeforeDatabase(t *testing.T) {
	for name, mutate := range map[string]func(*DepartureQuery){
		"no filter":    func(q *DepartureQuery) { q.StopID = 0 },
		"negative":     func(q *DepartureQuery) { q.RouteID = -1 },
		"overflow":     func(q *DepartureQuery) { q.TripID = 2147483648 },
		"missing from": func(q *DepartureQuery) { q.From = time.Time{} },
		"empty window": func(q *DepartureQuery) { q.Until = q.From },
		"reversed":     func(q *DepartureQuery) { q.Until = q.From.Add(-time.Second) },
		"large window": func(q *DepartureQuery) { q.Until = q.From.Add(24*time.Hour + time.Second) },
		"zero limit":   func(q *DepartureQuery) { q.Limit = 0 },
		"large limit":  func(q *DepartureQuery) { q.Limit = 101 },
	} {
		t.Run(name, func(t *testing.T) {
			q := testQuery()
			mutate(&q)
			repo := &fakeRepository{}
			_, err := NewService(repo, nil).Departures(context.Background(), q)
			var invalid *InvalidInput
			if !errors.As(err, &invalid) || repo.called {
				t.Fatalf("err=%v called=%v", err, repo.called)
			}
		})
	}
}

// TestMissingResourcesAndDatabaseFailures verifies missing resources and database failures.
func TestMissingResourcesAndDatabaseFailures(t *testing.T) {
	for _, failure := range []error{ErrNotFound, errors.New("database unavailable")} {
		service := NewService(&fakeRepository{err: failure}, nil)
		if _, err := service.Timetable(context.Background(), 1, "2026-11-10"); !errors.Is(err, failure) {
			t.Fatal(err)
		}
		if _, err := service.Stop(context.Background(), 1, testQuery()); !errors.Is(err, failure) {
			t.Fatal(err)
		}
		for _, q := range []DepartureQuery{
			{StopID: 1}, {RouteID: 1}, {TripID: 1},
		} {
			base := testQuery()
			q.From, q.Until, q.Limit = base.From, base.Until, base.Limit
			if _, err := service.Departures(context.Background(), q); !errors.Is(err, failure) {
				t.Fatal(err)
			}
		}
	}
}

// TestEstimatesKeepScheduleAndMatchExactOccurrence verifies estimates keep schedule and match exact occurrence.
func TestEstimatesKeepScheduleAndMatchExactOccurrence(t *testing.T) {
	q := testQuery()
	estimated := q.From.Add(10 * time.Minute)
	key := DepartureKey{TripID: 10, RouteStopID: 20, ServiceDate: "2026-11-10"}
	for _, tc := range []struct {
		name        string
		provider    EstimateProvider
		hasEstimate bool
	}{
		{"absent provider", nil, false},
		{"absent estimate", fakeEstimates{}, false},
		{"zero estimate", fakeEstimates{values: map[DepartureKey]time.Time{key: {}}}, false},
		{"provider failure", fakeEstimates{err: errors.New("feed offline")}, false},
		{"available", fakeEstimates{values: map[DepartureKey]time.Time{key: estimated}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepository{departures: []Departure{
				{TripID: 10, RouteStopID: 20, ServiceDate: key.ServiceDate, ScheduledDeparture: q.From},
				{TripID: 10, RouteStopID: 21, ServiceDate: key.ServiceDate, ScheduledDeparture: q.From},
				{TripID: 10, RouteStopID: 20, ServiceDate: "2026-11-09", ScheduledDeparture: q.From},
			}}
			result, err := NewService(repo, tc.provider).Departures(context.Background(), q)
			if err != nil {
				t.Fatal(err)
			}
			if (result[0].EstimatedDeparture != nil) != tc.hasEstimate {
				t.Fatalf("wrong estimate: %+v", result[0])
			}
			if tc.hasEstimate && !result[0].EstimatedDeparture.Equal(estimated) {
				t.Fatal("estimate changed")
			}
			for i, d := range result {
				if !d.ScheduledDeparture.Equal(q.From) || (i > 0 && d.EstimatedDeparture != nil) {
					t.Fatalf("incorrect occurrence %+v", d)
				}
			}
		})
	}
}

// TestEmptyDeparturesAndStopDetails verifies empty departures and stop details.
func TestEmptyDeparturesAndStopDetails(t *testing.T) {
	s := NewService(&fakeRepository{}, nil)
	departures, err := s.Departures(context.Background(), testQuery())
	if err != nil || departures == nil || len(departures) != 0 {
		t.Fatalf("%+v %v", departures, err)
	}
	stop, err := s.Stop(context.Background(), 1, testQuery())
	if err != nil || stop.Stop.Name != "Station" || stop.Stop.WheelchairBoarding != 1 || stop.Departures == nil {
		t.Fatalf("%+v %v", stop, err)
	}
}

func testJourneyQuery() JourneyQuery {
	from := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	return JourneyQuery{
		Origin:        "Central",
		Destination:   "Fairway",
		From:          from,
		Until:         from.Add(6 * time.Hour),
		Limit:         5,
		MaxTransfers:  DefaultMaxTransfers,
		MaxWalkMeters: DefaultMaxWalkMeters,
	}
}

// TestSearchStopsValidationAndEmptyResult verifies stop search validation and non-nil empty slice.
func TestSearchStopsValidationAndEmptyResult(t *testing.T) {
	tooLong := strings.Repeat("a", 101)
	for name, q := range map[string]StopSearchQuery{
		"long query":   {Query: tooLong, Limit: 20},
		"zero limit":   {Query: "station", Limit: 0},
		"large limit":  {Query: "station", Limit: 101},
		"negative lim": {Query: "station", Limit: -1},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepository{}
			_, err := NewService(repo, nil).SearchStops(context.Background(), q)
			var invalid *InvalidInput
			if !errors.As(err, &invalid) || repo.called {
				t.Fatalf("err=%v called=%v", err, repo.called)
			}
		})
	}

	repo := &fakeRepository{}
	stops, err := NewService(repo, nil).SearchStops(context.Background(), StopSearchQuery{Query: "  Station  ", Limit: 20})
	if err != nil || stops == nil || len(stops) != 0 {
		t.Fatalf("stops=%+v err=%v", stops, err)
	}
	if len(repo.searchCalls) != 1 || repo.searchCalls[0].Query != "Station" {
		t.Fatalf("searchCalls=%+v", repo.searchCalls)
	}
}

// TestJourneyValidationBeforeDatabase verifies journey input validation before database queries.
func TestJourneyValidationBeforeDatabase(t *testing.T) {
	tooLong := strings.Repeat("x", 101)
	for name, mutate := range map[string]func(*JourneyQuery){
		"missing origin":       func(q *JourneyQuery) { q.Origin = "   " },
		"missing destination":  func(q *JourneyQuery) { q.Destination = "" },
		"negative origin id":   func(q *JourneyQuery) { q.Origin = ""; q.OriginStopID = -1 },
		"overflow dest id":     func(q *JourneyQuery) { q.Destination = ""; q.DestinationStopID = 2147483648 },
		"long origin":          func(q *JourneyQuery) { q.Origin = tooLong },
		"long destination":     func(q *JourneyQuery) { q.Destination = tooLong },
		"same stop id":         func(q *JourneyQuery) { q.Origin = ""; q.Destination = ""; q.OriginStopID = 5; q.DestinationStopID = 5 },
		"same text":            func(q *JourneyQuery) { q.Destination = " central " },
		"missing from":         func(q *JourneyQuery) { q.From = time.Time{} },
		"empty window":         func(q *JourneyQuery) { q.Until = q.From },
		"reversed window":      func(q *JourneyQuery) { q.Until = q.From.Add(-time.Minute) },
		"large window":         func(q *JourneyQuery) { q.Until = q.From.Add(24*time.Hour + time.Second) },
		"zero limit":           func(q *JourneyQuery) { q.Limit = 0 },
		"large limit":          func(q *JourneyQuery) { q.Limit = 21 },
		"negative transfers":   func(q *JourneyQuery) { q.MaxTransfers = -1 },
		"too many transfers":   func(q *JourneyQuery) { q.MaxTransfers = 4 },
		"negative walk meters": func(q *JourneyQuery) { q.MaxWalkMeters = -1 },
		"large walk meters":    func(q *JourneyQuery) { q.MaxWalkMeters = 1501 },
	} {
		t.Run(name, func(t *testing.T) {
			q := testJourneyQuery()
			mutate(&q)
			repo := &fakeRepository{}
			_, err := NewService(repo, nil).Journeys(context.Background(), q)
			var invalid *InvalidInput
			if !errors.As(err, &invalid) || repo.called {
				t.Fatalf("err=%v called=%v", err, repo.called)
			}
		})
	}
}

// TestJourneyResolutionAndOutcomes verifies stop resolution, conflict checks, missing resources, and results.
func TestJourneyResolutionAndOutcomes(t *testing.T) {
	stopsFixture := []Stop{
		{ID: 21, Name: "Central Station", Latitude: 43.45, Longitude: -80.49, WheelchairBoarding: 1, IsActive: true},
		{ID: 22, Name: "Fairway Station", Latitude: 43.42, Longitude: -80.43, WheelchairBoarding: 1, IsActive: true},
		{ID: 23, Name: "Closed Stop", IsActive: false},
	}
	stopByID := map[int]Stop{21: stopsFixture[0], 22: stopsFixture[1], 23: stopsFixture[2]}

	t.Run("conflicting stop id and text", func(t *testing.T) {
		repo := &fakeRepository{stopByID: stopByID}
		q := testJourneyQuery()
		q.OriginStopID = 21
		q.Origin = "Fairway"
		_, err := NewService(repo, nil).Journeys(context.Background(), q)
		var invalid *InvalidInput
		if !errors.As(err, &invalid) {
			t.Fatalf("expected invalid input, got %v", err)
		}
	})

	t.Run("missing stop id returns not found", func(t *testing.T) {
		repo := &fakeRepository{stopByID: stopByID}
		q := testJourneyQuery()
		q.Origin = ""
		q.OriginStopID = 999
		_, err := NewService(repo, nil).Journeys(context.Background(), q)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("no matching stops yields meaningful message", func(t *testing.T) {
		repo := &fakeRepository{stops: stopsFixture}
		q := testJourneyQuery()
		q.Origin = "Nonexistent"
		res, err := NewService(repo, nil).Journeys(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		if res.Itineraries == nil || len(res.Itineraries) != 0 || len(repo.journeyQueries) != 0 || !strings.Contains(res.Message, "No active stops matched") {
			t.Fatalf("unexpected result: %+v", res)
		}
	})

	t.Run("inactive stop yields meaningful message", func(t *testing.T) {
		repo := &fakeRepository{stopByID: stopByID}
		q := testJourneyQuery()
		q.Origin = ""
		q.Destination = ""
		q.OriginStopID = 23
		q.DestinationStopID = 22
		res, err := NewService(repo, nil).Journeys(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Itineraries) != 0 || len(repo.journeyQueries) != 0 || !strings.Contains(res.Message, "No active stops matched") {
			t.Fatalf("unexpected result: %+v", res)
		}
	})

	t.Run("resolved identical single stop returns invalid input", func(t *testing.T) {
		repo := &fakeRepository{stops: stopsFixture}
		q := testJourneyQuery()
		q.Origin = "Central"
		q.Destination = "Central Station"
		_, err := NewService(repo, nil).Journeys(context.Background(), q)
		var invalid *InvalidInput
		if !errors.As(err, &invalid) {
			t.Fatalf("expected invalid input, got %v", err)
		}
	})

	t.Run("no itineraries in window yields meaningful message", func(t *testing.T) {
		repo := &fakeRepository{stops: stopsFixture}
		res, err := NewService(repo, nil).Journeys(context.Background(), testJourneyQuery())
		if err != nil {
			t.Fatal(err)
		}
		if res.Itineraries == nil || len(res.Itineraries) != 0 || len(repo.journeyQueries) != 1 || !strings.Contains(res.Message, "No transit itineraries found") {
			t.Fatalf("unexpected result: %+v", res)
		}
	})

	t.Run("direct and transfer itineraries sorted and summarized", func(t *testing.T) {
		from := testJourneyQuery().From
		transferIt := Itinerary{
			OriginStop:         Stop{ID: 21, Name: "Central Station"},
			DestinationStop:    Stop{ID: 22, Name: "Fairway Station"},
			ScheduledDeparture: from.Add(5 * time.Minute),
			ScheduledArrival:   from.Add(40 * time.Minute),
			DurationMinutes:    35,
			Transfers:          1,
			Legs:               []JourneyLeg{{Mode: "transit", TripID: 201}},
		}
		directIt := Itinerary{
			OriginStop:         Stop{ID: 21, Name: "Central Station"},
			DestinationStop:    Stop{ID: 22, Name: "Fairway Station"},
			ScheduledDeparture: from.Add(10 * time.Minute),
			ScheduledArrival:   from.Add(30 * time.Minute),
			DurationMinutes:    20,
			Transfers:          0,
			Legs:               []JourneyLeg{{Mode: "transit", TripID: 100}},
		}
		repo := &fakeRepository{
			stops:    stopsFixture,
			journeys: []Itinerary{transferIt, directIt},
		}
		res, err := NewService(repo, nil).Journeys(context.Background(), testJourneyQuery())
		if err != nil {
			t.Fatal(err)
		}
		if res.Timezone != AgencyTimezone || res.Message != "Found 2 itinerary options." {
			t.Fatalf("unexpected metadata/message: %+v", res)
		}
		if len(res.Itineraries) != 2 || res.Itineraries[0].Transfers != 0 || res.Itineraries[0].OriginStop.Latitude != 43.45 {
			t.Fatalf("unexpected itineraries: %+v", res.Itineraries)
		}
	})

	t.Run("single itinerary uses singular message", func(t *testing.T) {
		from := testJourneyQuery().From
		repo := &fakeRepository{
			stops: stopsFixture,
			journeys: []Itinerary{{
				OriginStop:         Stop{ID: 21},
				DestinationStop:    Stop{ID: 22},
				ScheduledDeparture: from.Add(10 * time.Minute),
				ScheduledArrival:   from.Add(30 * time.Minute),
				DurationMinutes:    20,
				Transfers:          0,
				Legs:               []JourneyLeg{{Mode: "transit", TripID: 100}},
			}},
		}
		res, err := NewService(repo, nil).Journeys(context.Background(), testJourneyQuery())
		if err != nil {
			t.Fatal(err)
		}
		if res.Message != "Found 1 itinerary option." {
			t.Fatalf("unexpected message: %q", res.Message)
		}
	})
}

// TestJourneyEstimatesKeepScheduleAndHandleFailures verifies live estimates enrich journeys safely without altering scheduled times.
func TestJourneyEstimatesKeepScheduleAndHandleFailures(t *testing.T) {
	q := testJourneyQuery()
	stopsFixture := []Stop{
		{ID: 21, Name: "Central Station", Latitude: 43.45, Longitude: -80.49, IsActive: true},
		{ID: 22, Name: "Fairway Station", Latitude: 43.42, Longitude: -80.43, IsActive: true},
	}
	depSched := q.From.Add(10 * time.Minute)
	arrSched := q.From.Add(30 * time.Minute)
	walkDepSched := depSched.Add(-2 * time.Minute)
	walkArrSched := arrSched.Add(3 * time.Minute)
	depKey := DepartureKey{TripID: 100, RouteStopID: 701, ServiceDate: "2026-11-10"}
	arrKey := DepartureKey{TripID: 100, RouteStopID: 703, ServiceDate: "2026-11-10"}
	estDep := depSched.Add(4 * time.Minute)
	estArr := arrSched.Add(5 * time.Minute)

	baseItinerary := func() Itinerary {
		return Itinerary{
			OriginStop:         stopsFixture[0],
			DestinationStop:    stopsFixture[1],
			ScheduledDeparture: walkDepSched,
			ScheduledArrival:   walkArrSched,
			DurationMinutes:    25,
			WalkingMinutes:     5,
			Transfers:          0,
			Legs: []JourneyLeg{
				{Mode: "walk", FromStopID: 21, FromStopName: "Central Station", ToStopID: 24, ToStopName: "Board Stop", ScheduledDeparture: walkDepSched, ScheduledArrival: depSched, DurationMinutes: 2},
				{
					Mode:               "transit",
					TripID:             100,
					Route:              &Route{ID: 7, Number: "7", Name: "King", Direction: "direction_0", IsActive: true},
					ServiceDate:        "2026-11-10",
					FromStopID:         24,
					FromStopName:       "Board Stop",
					ToStopID:           25,
					ToStopName:         "Alight Stop",
					ScheduledDeparture: depSched,
					ScheduledArrival:   arrSched,
					DurationMinutes:    20,
					Stops: []StopTime{
						{RouteStopID: 701, StopID: 24, StopName: "Board Stop", StopSequence: 1, ScheduledDeparture: depSched},
						{RouteStopID: 703, StopID: 25, StopName: "Alight Stop", StopSequence: 2, ScheduledArrival: arrSched},
					},
				},
				{Mode: "walk", FromStopID: 25, FromStopName: "Alight Stop", ToStopID: 22, ToStopName: "Fairway Station", ScheduledDeparture: arrSched, ScheduledArrival: walkArrSched, DurationMinutes: 3},
			},
		}
	}

	for _, tc := range []struct {
		name        string
		provider    EstimateProvider
		hasEstimate bool
	}{
		{"absent provider", nil, false},
		{"missing key", fakeEstimates{}, false},
		{"zero estimate", fakeEstimates{values: map[DepartureKey]time.Time{depKey: {}, arrKey: {}}}, false},
		{"provider failure", fakeEstimates{err: errors.New("feed offline")}, false},
		{"available", fakeEstimates{values: map[DepartureKey]time.Time{depKey: estDep, arrKey: estArr}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepository{stops: stopsFixture, journeys: []Itinerary{baseItinerary()}}
			res, err := NewService(repo, tc.provider).Journeys(context.Background(), q)
			if err != nil {
				t.Fatal(err)
			}
			it := res.Itineraries[0]
			if !it.ScheduledDeparture.Equal(walkDepSched) || !it.ScheduledArrival.Equal(walkArrSched) {
				t.Fatalf("scheduled times mutated: %+v", it)
			}
			if (it.EstimatedDeparture != nil) != tc.hasEstimate || (it.EstimatedArrival != nil) != tc.hasEstimate {
				t.Fatalf("unexpected estimate presence: %+v", it)
			}
			if tc.hasEstimate {
				if !it.EstimatedDeparture.Equal(estDep.Add(-2*time.Minute)) || !it.EstimatedArrival.Equal(estArr.Add(3*time.Minute)) {
					t.Fatalf("walk-shifted itinerary estimates wrong: %+v", it)
				}
				if !it.Legs[1].EstimatedDeparture.Equal(estDep) || !it.Legs[1].EstimatedArrival.Equal(estArr) {
					t.Fatalf("transit leg estimates wrong: %+v", it.Legs[1])
				}
			}
		})
	}
}

// TestJourneyDisruptionSummaryMessage verifies excluded disruption notices and empty-result messaging.
func TestJourneyDisruptionSummaryMessage(t *testing.T) {
	stopsFixture := []Stop{
		{ID: 21, Name: "Central Station", Latitude: 43.45, Longitude: -80.49, IsActive: true},
		{ID: 22, Name: "Fairway Station", Latitude: 43.42, Longitude: -80.43, IsActive: true},
	}
	notice := DisruptionNotice{ID: 5, Type: "cancellation", Title: "Route 7 cancelled", Status: "active"}
	repo := &fakeRepository{
		stops:               stopsFixture,
		excludedDisruptions: []DisruptionNotice{notice},
	}
	res, err := NewService(repo, nil).Journeys(context.Background(), testJourneyQuery())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Itineraries) != 0 || len(res.ExcludedDisruptions) != 1 || res.ExcludedDisruptions[0].ID != 5 {
		t.Fatalf("unexpected disruption result: %+v", res)
	}
	if !strings.Contains(res.Message, "active service disruptions") {
		t.Fatalf("expected disruption-aware empty message, got %q", res.Message)
	}
}
