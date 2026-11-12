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
	err    error
	query  transit.DepartureQuery
	id     int
	date   string
	called bool
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

func scheduleRequest(f *fakeSchedules, path string) *httptest.ResponseRecorder {
	mux := chi.NewRouter()
	router.RegisterScheduleRoutes(mux, handlers.NewScheduleHandler(f))
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
		{"/api/stops/1/departures", 1, 0, 0},
		{"/api/routes/7/departures", 0, 7, 0},
		{"/api/trips/9/departures", 0, 0, 9},
		{"/api/departures?stop_id=1&route_id=7&trip_id=9", 1, 7, 9},
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
	response := scheduleRequest(f, "/api/routes/7/schedule?date=2026-11-10")
	if response.Code != 200 || f.id != 7 || f.date != "2026-11-10" {
		t.Fatalf("%d %+v", response.Code, f)
	}
	f = &fakeSchedules{}
	response = scheduleRequest(f, "/api/stops/1")
	if response.Code != 200 || f.id != 1 {
		t.Fatalf("%d %+v", response.Code, f)
	}
}

// TestExplicitDepartureWindow verifies explicit departure window.
func TestExplicitDepartureWindow(t *testing.T) {
	f := &fakeSchedules{}
	response := scheduleRequest(f, "/api/departures?stop_id=1&from=2026-11-10T12:00:00Z&until=2026-11-10T13:00:00Z&limit=5")
	if response.Code != 200 || f.query.From.Hour() != 12 || f.query.Until.Hour() != 13 || f.query.Limit != 5 {
		t.Fatalf("%d %+v", response.Code, f.query)
	}
}

// TestInvalidScheduleRequests verifies invalid schedule requests.
func TestInvalidScheduleRequests(t *testing.T) {
	for _, path := range []string{
		"/api/routes/0/schedule?date=2026-11-10", "/api/routes/abc/schedule", "/api/routes/2147483648/schedule",
		"/api/routes/1/schedule?date=2026-11-10&date=2026-11-11", "/api/routes/1/schedule?unexpected=1",
		"/api/stops/0/departures", "/api/routes/abc/departures", "/api/trips/2147483648/departures",
		"/api/departures?stop_id=0", "/api/departures?trip_id=-1", "/api/departures?route_id=abc",
		"/api/departures?stop_id=1&from=today", "/api/departures?stop_id=1&until=2026-11-10",
		"/api/departures?stop_id=1&limit=many", "/api/departures?stop_id=1&limit=",
		"/api/departures?stop_id=1&stop_id=2", "/api/departures?stop_id=1&unknown=true",
		"/api/stops/1/departures?stop_id=2", "/api/stops/1?stop_id=2",
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
		for _, path := range []string{"/api/routes/1/schedule?date=2026-11-10", "/api/stops/1/departures", "/api/stops/1"} {
			response := scheduleRequest(&fakeSchedules{err: tc.err}, path)
			if response.Code != tc.status || response.Header().Get("Content-Type") != "application/json" || strings.Contains(response.Body.String(), "secret") {
				t.Fatalf("%s: %d %s", path, response.Code, response.Body)
			}
		}
	}
}
