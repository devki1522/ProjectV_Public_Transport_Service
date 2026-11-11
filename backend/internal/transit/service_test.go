package transit

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepository struct {
	err        error
	called     bool
	departures []Departure
}

func (f *fakeRepository) Route(context.Context, int) (Route, error) {
	f.called = true
	return Route{ID: 7, Direction: "direction_0"}, f.err
}
func (f *fakeRepository) Stop(context.Context, int) (Stop, error) {
	f.called = true
	return Stop{ID: 1, Name: "Station", WheelchairBoarding: 1}, f.err
}
func (f *fakeRepository) TripExists(context.Context, int) error { f.called = true; return f.err }
func (f *fakeRepository) Timetable(context.Context, int, string) ([]ScheduledTrip, error) {
	f.called = true
	return nil, f.err
}
func (f *fakeRepository) Departures(context.Context, DepartureQuery) ([]Departure, error) {
	f.called = true
	return f.departures, f.err
}

type fakeEstimates struct {
	values map[DepartureKey]time.Time
	err    error
}

func (f fakeEstimates) Estimates(context.Context, []Departure) (map[DepartureKey]time.Time, error) {
	return f.values, f.err
}

func testQuery() DepartureQuery {
	from := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	return DepartureQuery{StopID: 1, From: from, Until: from.Add(time.Hour), Limit: 20}
}

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
