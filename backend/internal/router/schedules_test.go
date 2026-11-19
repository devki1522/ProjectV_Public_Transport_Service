package router_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/handlers"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/router"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/transit"
	"github.com/go-chi/chi/v5"
)

type fakeSchedules struct {
	err          error
	query        transit.DepartureQuery
	stopQuery    transit.StopSearchQuery
	journeyQuery transit.JourneyQuery
	id           int
	date         string
	called       bool
}

// Timetable returns scheduled trips for a route and service date.
func (f *fakeSchedules) Timetable(_ context.Context, id int, date string) (transit.Timetable, error) {
	f.id, f.date, f.called = id, date, true
	return transit.Timetable{ServiceDate: date, Trips: []transit.ScheduledTrip{}}, f.err
}

// Departures returns departures matching the requested filters and time window.
func (f *fakeSchedules) Departures(_ context.Context, q transit.DepartureQuery) ([]transit.Departure, error) {
	f.query, f.called = q, true
	return []transit.Departure{{TripID: 9, Route: transit.Route{ID: 7, Direction: "direction_0"}, ScheduledDeparture: q.From}}, f.err
}

// Stop loads stop details and associated departures.
func (f *fakeSchedules) Stop(_ context.Context, id int, q transit.DepartureQuery) (transit.StopDetails, error) {
	f.id, f.query, f.called = id, q, true
	return transit.StopDetails{Stop: transit.Stop{ID: id}, Departures: []transit.Departure{}}, f.err
}

// SearchStops searches active stops in the fake schedule service.
func (f *fakeSchedules) SearchStops(_ context.Context, q transit.StopSearchQuery) ([]transit.Stop, error) {
	f.stopQuery, f.called = q, true
	return []transit.Stop{{ID: 21, Name: "Central Station", IsActive: true}}, f.err
}

// Journeys returns fake journey search results for HTTP handler tests.
func (f *fakeSchedules) Journeys(_ context.Context, q transit.JourneyQuery) (transit.JourneySearchResult, error) {
	f.journeyQuery, f.called = q, true
	return transit.JourneySearchResult{
		Origin:           q.Origin,
		Destination:      q.Destination,
		OriginStops:      []transit.Stop{},
		DestinationStops: []transit.Stop{},
		From:             q.From,
		Until:            q.Until,
		Timezone:         transit.AgencyTimezone,
		Itineraries: []transit.Itinerary{{
			OriginStop:         transit.Stop{ID: 21, Name: "Central Station", IsActive: true},
			DestinationStop:    transit.Stop{ID: 22, Name: "Fairway Station", IsActive: true},
			ScheduledDeparture: q.From.Add(10 * time.Minute),
			ScheduledArrival:   q.From.Add(30 * time.Minute),
			DurationMinutes:    20,
			Transfers:          0,
			Legs: []transit.JourneyLeg{{
				Mode:               "transit",
				TripID:             100,
				Route:              &transit.Route{ID: 7, Number: "7", Name: "King", Direction: "direction_0", IsActive: true},
				ServiceDate:        "2026-11-10",
				FromStopID:         21,
				FromStopName:       "Central Station",
				FromStopSequence:   1,
				ToStopID:           22,
				ToStopName:         "Fairway Station",
				ToStopSequence:     2,
				StopCount:          1,
				ScheduledDeparture: q.From.Add(10 * time.Minute),
				ScheduledArrival:   q.From.Add(30 * time.Minute),
				DurationMinutes:    20,
			}},
		}},
		Message: "Found 1 itinerary option.",
	}, f.err
}

func scheduleRequest(f *fakeSchedules, path string) *httptest.ResponseRecorder {
	mux := chi.NewRouter()
	mux.Route(router.APIBasePath(""), func(api chi.Router) {
		router.RegisterScheduleRoutes(api, handlers.NewScheduleHandler(f))
	})
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
	return recorder
}

// TestScheduleRoutes verifies schedule routes.
func TestScheduleRoutes(t *testing.T) {
	for _, tc := range []struct {
		path              string
		stop, route, trip int
	}{
		{"/api/transport/v1/stops/1/departures", 1, 0, 0},
		{"/api/transport/v1/routes/7/departures", 0, 7, 0},
		{"/api/transport/v1/trips/9/departures", 0, 0, 9},
		{"/api/transport/v1/departures?stop_id=1&route_id=7&trip_id=9", 1, 7, 9},
	} {
		f := &fakeSchedules{}
		before := time.Now().Add(-time.Second)
		response := scheduleRequest(f, tc.path)
		if response.Code != 200 || !f.called || f.query.StopID != tc.stop || f.query.RouteID != tc.route || f.query.TripID != tc.trip {
			t.Fatalf("%s: %d %+v %s", tc.path, response.Code, f.query, response.Body)
		}
		if f.query.Limit != 20 || f.query.Until.Sub(f.query.From) != 2*time.Hour || f.query.From.Before(before) {
			t.Fatal("wrong defaults")
		}
		var result []map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if _, ok := result[0]["scheduled_departure_time"]; !ok {
			t.Fatal("missing schedule")
		}
		if estimate, ok := result[0]["estimated_departure_time"]; !ok || estimate != nil {
			t.Fatal("missing estimates must be explicit null")
		}
	}
	f := &fakeSchedules{}
	response := scheduleRequest(f, "/api/transport/v1/routes/7/schedule?date=2026-11-10")
	if response.Code != 200 || f.id != 7 || f.date != "2026-11-10" {
		t.Fatalf("%d %+v", response.Code, f)
	}
	f = &fakeSchedules{}
	response = scheduleRequest(f, "/api/transport/v1/stops/1")
	if response.Code != 200 || f.id != 1 {
		t.Fatalf("%d %+v", response.Code, f)
	}
}

// TestExplicitDepartureWindow verifies explicit departure window.
func TestExplicitDepartureWindow(t *testing.T) {
	f := &fakeSchedules{}
	response := scheduleRequest(f, "/api/transport/v1/departures?stop_id=1&from=2026-11-10T12:00:00Z&until=2026-11-10T13:00:00Z&limit=5")
	if response.Code != 200 || f.query.From.Hour() != 12 || f.query.Until.Hour() != 13 || f.query.Limit != 5 {
		t.Fatalf("%d %+v", response.Code, f.query)
	}
}

// TestInvalidScheduleRequests verifies invalid schedule requests.
func TestInvalidScheduleRequests(t *testing.T) {
	for _, path := range []string{
		"/api/transport/v1/routes/0/schedule?date=2026-11-10", "/api/transport/v1/routes/abc/schedule", "/api/transport/v1/routes/2147483648/schedule",
		"/api/transport/v1/routes/1/schedule?date=2026-11-10&date=2026-11-11", "/api/transport/v1/routes/1/schedule?unexpected=1",
		"/api/transport/v1/stops/0/departures", "/api/transport/v1/routes/abc/departures", "/api/transport/v1/trips/2147483648/departures",
		"/api/transport/v1/departures?stop_id=0", "/api/transport/v1/departures?trip_id=-1", "/api/transport/v1/departures?route_id=abc",
		"/api/transport/v1/departures?stop_id=1&from=today", "/api/transport/v1/departures?stop_id=1&until=2026-11-10",
		"/api/transport/v1/departures?stop_id=1&limit=many", "/api/transport/v1/departures?stop_id=1&limit=",
		"/api/transport/v1/departures?stop_id=1&stop_id=2", "/api/transport/v1/departures?stop_id=1&unknown=true",
		"/api/transport/v1/stops/1/departures?stop_id=2", "/api/transport/v1/stops/1?stop_id=2",
	} {
		f := &fakeSchedules{}
		response := scheduleRequest(f, path)
		if response.Code != 400 || f.called {
			t.Errorf("%s: %d called=%v", path, response.Code, f.called)
		}
	}
}

// TestScheduleHTTPErrorMapping verifies schedule HTTP error mapping.
func TestScheduleHTTPErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{transit.ErrNotFound, 404}, {&transit.InvalidInput{Message: "invalid date"}, 400}, {errors.New("secret database details"), 500},
	} {
		for _, path := range []string{"/api/transport/v1/routes/1/schedule?date=2026-11-10", "/api/transport/v1/stops/1/departures", "/api/transport/v1/stops/1", "/api/transport/v1/stops", "/api/transport/v1/journeys?origin=Central&destination=Fairway"} {
			response := scheduleRequest(&fakeSchedules{err: tc.err}, path)
			if response.Code != tc.status || response.Header().Get("Content-Type") != "application/json" || strings.Contains(response.Body.String(), "secret") {
				t.Fatalf("%s: %d %s", path, response.Code, response.Body)
			}
		}
	}
}

// TestStopSearchAndJourneyRoutes verifies /api/stops and /api/journeys parsing and defaults.
func TestStopSearchAndJourneyRoutes(t *testing.T) {
	f := &fakeSchedules{}
	response := scheduleRequest(f, "/api/transport/v1/stops?q=Central&limit=10")
	if response.Code != 200 || !f.called || f.stopQuery.Query != "Central" || f.stopQuery.Limit != 10 {
		t.Fatalf("%d %+v %s", response.Code, f.stopQuery, response.Body)
	}

	f = &fakeSchedules{}
	response = scheduleRequest(f, "/api/transport/v1/stops")
	if response.Code != 200 || !f.called || f.stopQuery.Limit != 20 || f.stopQuery.Query != "" {
		t.Fatalf("%d %+v", response.Code, f.stopQuery)
	}

	f = &fakeSchedules{}
	before := time.Now().Add(-time.Second)
	response = scheduleRequest(f, "/api/transport/v1/journeys?origin=Central&destination=Fairway")
	if response.Code != 200 || !f.called || f.journeyQuery.Origin != "Central" || f.journeyQuery.Destination != "Fairway" {
		t.Fatalf("%d %+v %s", response.Code, f.journeyQuery, response.Body)
	}
	if f.journeyQuery.Limit != 5 || f.journeyQuery.MaxTransfers != transit.DefaultMaxTransfers || f.journeyQuery.MaxWalkMeters != transit.DefaultMaxWalkMeters || f.journeyQuery.Until.Sub(f.journeyQuery.From) != 6*time.Hour || f.journeyQuery.From.Before(before) {
		t.Fatalf("wrong journey defaults: %+v", f.journeyQuery)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	itineraries, ok := payload["itineraries"].([]any)
	if !ok || len(itineraries) != 1 {
		t.Fatalf("missing itineraries in response: %+v", payload)
	}
	firstIt := itineraries[0].(map[string]any)
	if est, ok := firstIt["estimated_departure_time"]; !ok || est != nil {
		t.Fatalf("missing itinerary estimated_departure_time must be explicit null: %+v", firstIt)
	}
	if est, ok := firstIt["estimated_arrival_time"]; !ok || est != nil {
		t.Fatalf("missing itinerary estimated_arrival_time must be explicit null: %+v", firstIt)
	}
	firstLeg := firstIt["legs"].([]any)[0].(map[string]any)
	if est, ok := firstLeg["estimated_departure_time"]; !ok || est != nil {
		t.Fatalf("missing leg estimated_departure_time must be explicit null: %+v", firstLeg)
	}
	if est, ok := firstLeg["estimated_arrival_time"]; !ok || est != nil {
		t.Fatalf("missing leg estimated_arrival_time must be explicit null: %+v", firstLeg)
	}

	f = &fakeSchedules{}
	response = scheduleRequest(f, "/api/transport/v1/journeys?origin_stop_id=21&destination_stop_id=22&from=2026-11-10T12:00:00Z&until=2026-11-10T16:00:00Z&limit=3&max_transfers=2&max_walk_meters=400")
	if response.Code != 200 || f.journeyQuery.OriginStopID != 21 || f.journeyQuery.DestinationStopID != 22 || f.journeyQuery.Limit != 3 || f.journeyQuery.MaxTransfers != 2 || f.journeyQuery.MaxWalkMeters != 400 || f.journeyQuery.Until.Hour() != 16 {
		t.Fatalf("%d %+v %s", response.Code, f.journeyQuery, response.Body)
	}
}

// TestInvalidStopSearchAndJourneyRequests verifies invalid stop search and journey query rejection.
func TestInvalidStopSearchAndJourneyRequests(t *testing.T) {
	for _, path := range []string{
		"/api/transport/v1/stops?q=",
		"/api/transport/v1/stops?limit=many",
		"/api/transport/v1/stops?unknown=1",
		"/api/transport/v1/stops?q=a&q=b",
		"/api/transport/v1/journeys?origin_stop_id=0&destination_stop_id=2",
		"/api/transport/v1/journeys?origin_stop_id=abc&destination=Fairway",
		"/api/transport/v1/journeys?origin=Central&destination_stop_id=2147483648",
		"/api/transport/v1/journeys?origin=Central&destination=Fairway&from=today",
		"/api/transport/v1/journeys?origin=Central&destination=Fairway&until=2026-11-10",
		"/api/transport/v1/journeys?origin=Central&destination=Fairway&limit=many",
		"/api/transport/v1/journeys?origin=Central&destination=Fairway&max_transfers=abc",
		"/api/transport/v1/journeys?origin=Central&destination=Fairway&max_walk_meters=far",
		"/api/transport/v1/journeys?origin=Central&destination=Fairway&unknown=true",
		"/api/transport/v1/journeys?origin=Central&origin=King&destination=Fairway",
		"/api/transport/v1/journeys?origin=&destination=Fairway",
	} {
		f := &fakeSchedules{}
		response := scheduleRequest(f, path)
		if response.Code != 400 || f.called {
			t.Errorf("%s: %d called=%v body=%s", path, response.Code, f.called, response.Body)
		}
	}
}
