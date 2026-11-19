package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/auth"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/handlers"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/router"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/vehicles"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

type fakeVehicleService struct {
	submitInput     vehicles.LocationUpdateInput
	submitSource    string
	submitResult    vehicles.LocationUpdateResult
	submitErr       error
	listQuery       vehicles.ActiveVehiclesQuery
	listResult      []vehicles.ActiveVehicle
	listErr         error
	geometryID      int
	geometry        vehicles.RouteGeometry
	geometryErr     error
	arrivalsTripID  int
	arrivalsStopID  int
	arrivalsRouteID int
	arrivalsResult  []vehicles.EstimatedArrival
	arrivalsErr     error
	called          string
}

func (f *fakeVehicleService) SubmitLocation(_ context.Context, input vehicles.LocationUpdateInput, defaultSource string) (vehicles.LocationUpdateResult, error) {
	f.called = "submit"
	f.submitInput = input
	f.submitSource = defaultSource
	return f.submitResult, f.submitErr
}

func (f *fakeVehicleService) ListActiveVehicles(_ context.Context, q vehicles.ActiveVehiclesQuery) ([]vehicles.ActiveVehicle, error) {
	f.called = "list"
	f.listQuery = q
	return f.listResult, f.listErr
}

func (f *fakeVehicleService) RouteGeometry(_ context.Context, routeID int) (vehicles.RouteGeometry, error) {
	f.called = "geometry"
	f.geometryID = routeID
	return f.geometry, f.geometryErr
}

func (f *fakeVehicleService) UpcomingArrivals(_ context.Context, tripID, stopID int) ([]vehicles.EstimatedArrival, error) {
	f.called = "upcoming_arrivals"
	f.arrivalsTripID = tripID
	f.arrivalsStopID = stopID
	f.arrivalsRouteID = 0
	return f.arrivalsResult, f.arrivalsErr
}

func (f *fakeVehicleService) StopArrivals(_ context.Context, stopID, routeID int) ([]vehicles.EstimatedArrival, error) {
	f.called = "stop_arrivals"
	f.arrivalsTripID = 0
	f.arrivalsStopID = stopID
	f.arrivalsRouteID = routeID
	return f.arrivalsResult, f.arrivalsErr
}

type fakeSimulator struct {
	status vehicles.SimulatorStatus
	err    error
	called string
}

func (f *fakeSimulator) Status() vehicles.SimulatorStatus {
	f.called = "status"
	return f.status
}

func (f *fakeSimulator) Start(speed *float64) (vehicles.SimulatorStatus, error) {
	f.called = "start"
	if speed != nil {
		f.status.SpeedMultiplier = *speed
	}
	f.status.Running = f.err == nil
	return f.status, f.err
}

func (f *fakeSimulator) Stop() vehicles.SimulatorStatus {
	f.called = "stop"
	f.status.Running = false
	return f.status
}

func (f *fakeSimulator) Step(context.Context) (vehicles.SimulatorStatus, error) {
	f.called = "step"
	f.status.TotalUpdatesSent++
	return f.status, f.err
}

func (f *fakeSimulator) Reset(context.Context) (vehicles.SimulatorStatus, error) {
	f.called = "reset"
	f.status.TotalUpdatesSent = 0
	return f.status, f.err
}

func newTestVehicleEnv(t *testing.T, svc handlers.VehicleService, sim handlers.VehicleSimulator) (*testAuthEnv, *fakeVehicleService) {
	t.Helper()
	now := func() time.Time { return time.Now().UTC() }
	trust := config.JWTTrustConfig{
		Issuer:   routerTestIssuer,
		Audience: routerTestAudience,
		Secret:   []byte(routerTestSecret),
	}
	provider, err := auth.NewMockProvider(trust, config.MockAuthConfig{AdminPassword: routerAdminPass}, now)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := auth.NewJWTValidator(trust, now)
	if err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewInMemorySessionStore(now, time.Hour, 100)
	t.Cleanup(sessions.Close)

	csrf := auth.NewCSRFProtector([]string{routerTestOrigin})
	throttler := auth.NewLoginThrottler(20, time.Minute, false, now)
	authHandler := auth.NewHandler(provider, validator, sessions, csrf, throttler, "transit_session", false, "TransitAdministrator", now)
	authMW := auth.NewMiddleware(sessions, validator, "transit_session", false)

	var auditBuf bytes.Buffer
	auditLogger := slog.New(slog.NewJSONHandler(&auditBuf, nil))
	handler := handlers.NewVehicleHandlerWithAudit(svc, sim, auditLogger)

	mux := chi.NewRouter()
	router.RegisterAuthRoutes(mux, authHandler, authMW)
	router.RegisterVehicleRoutes(mux, handler, authMW, csrf, "TransitAdministrator")

	fakeSvc, _ := svc.(*fakeVehicleService)
	return &testAuthEnv{
		mux:       mux,
		sessions:  sessions,
		validator: validator,
		csrf:      csrf,
		auditBuf:  &auditBuf,
	}, fakeSvc
}

func TestVehiclePublicReadEndpoints(t *testing.T) {
	now := time.Date(2026, 11, 18, 18, 0, 0, 0, time.UTC)
	heading := 120.0
	speed := 40.5
	tripID := 300
	svc := &fakeVehicleService{
		listResult: []vehicles.ActiveVehicle{
			{
				VehicleID:     11,
				VehicleNumber: "SIM-001",
				IsAccessible:  true,
				RouteID:       7,
				RouteNumber:   "7",
				RouteName:     "King",
				Direction:     "direction_0",
				TripID:        &tripID,
				Latitude:      43.4516,
				Longitude:     -80.4925,
				Heading:       &heading,
				SpeedKmh:      &speed,
				Status:        vehicles.StatusInService,
				SourceService: "simulator",
				IsSimulated:   true,
				RecordedAt:    now,
				ReceivedAt:    now,
			},
		},
		geometry: vehicles.RouteGeometry{
			RouteID:     7,
			RouteNumber: "7",
			RouteName:   "King",
			Direction:   "direction_0",
			Headsign:    "Conestoga Mall",
			Coordinates: []vehicles.Coordinate{{Lat: 43.45, Lng: -80.49}, {Lat: 43.47, Lng: -80.51}},
		},
	}
	sim := &fakeSimulator{status: vehicles.SimulatorStatus{Running: true, VehicleCount: 24, RouteCount: 24, TickIntervalMs: 3000, SpeedMultiplier: 1}}
	env, _ := newTestVehicleEnv(t, svc, sim)

	rec := authedRequest(env.mux, http.MethodGet, "/api/vehicles?route_number=7&max_age_seconds=300", "", nil, "")
	if rec.Code != http.StatusOK || svc.listQuery.RouteNumber != "7" || svc.listQuery.MaxAge != 300*time.Second {
		t.Fatalf("GET /api/vehicles failed: code=%d query=%+v body=%s", rec.Code, svc.listQuery, rec.Body.String())
	}
	var gotVehicles []vehicles.ActiveVehicle
	if err := json.Unmarshal(rec.Body.Bytes(), &gotVehicles); err != nil {
		t.Fatal(err)
	}
	if len(gotVehicles) != 1 || gotVehicles[0].VehicleNumber != "SIM-001" || !gotVehicles[0].IsSimulated {
		t.Fatalf("unexpected decoded vehicles: %+v", gotVehicles)
	}

	rec = authedRequest(env.mux, http.MethodGet, "/api/routes/7/vehicles?trip_id=300", "", nil, "")
	if rec.Code != http.StatusOK || svc.listQuery.RouteID != 7 || svc.listQuery.TripID != 300 {
		t.Fatalf("GET /api/routes/7/vehicles failed: code=%d query=%+v body=%s", rec.Code, svc.listQuery, rec.Body.String())
	}

	rec = authedRequest(env.mux, http.MethodGet, "/api/trips/300/vehicles", "", nil, "")
	if rec.Code != http.StatusOK || svc.listQuery.TripID != 300 {
		t.Fatalf("GET /api/trips/300/vehicles failed: code=%d query=%+v", rec.Code, svc.listQuery)
	}

	rec = authedRequest(env.mux, http.MethodGet, "/api/routes/7/shape", "", nil, "")
	if rec.Code != http.StatusOK || svc.geometryID != 7 {
		t.Fatalf("GET /api/routes/7/shape failed: code=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = authedRequest(env.mux, http.MethodGet, "/api/vehicles/simulator", "", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/vehicles/simulator failed: code=%d", rec.Code)
	}

	// Conflicting path vs query filters and unknown parameters return 400.
	for _, badPath := range []string{
		"/api/routes/7/vehicles?route_id=8",
		"/api/trips/300/vehicles?trip_id=301",
		"/api/vehicles?bogus=1",
		"/api/routes/0/vehicles",
		"/api/routes/abc/shape",
	} {
		badRec := authedRequest(env.mux, http.MethodGet, badPath, "", nil, "")
		if badRec.Code != http.StatusBadRequest {
			t.Fatalf("%s returned %d, want 400", badPath, badRec.Code)
		}
	}
}

func TestVehicleLocationMutationsEnforceAuthRoleCSRFAndErrors(t *testing.T) {
	svc := &fakeVehicleService{
		submitResult: vehicles.LocationUpdateResult{
			Created: true,
			Vehicle: vehicles.ActiveVehicle{VehicleID: 44, VehicleNumber: "GRT-404", RouteID: 7},
		},
	}
	sim := &fakeSimulator{status: vehicles.SimulatorStatus{SpeedMultiplier: 1}}
	env, _ := newTestVehicleEnv(t, svc, sim)

	expires := time.Now().Add(15 * time.Minute)
	signed, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "test-user", "role": "User", "iss": routerTestIssuer,
		"aud": routerTestAudience, "exp": expires.Unix(),
	}).SignedString([]byte(routerTestSecret))
	principal, _ := env.validator.Validate(signed)
	sessionID, session, _ := env.sessions.Create(principal, signed, expires)
	userCookie := &http.Cookie{Name: "transit_session", Value: sessionID}
	userCSRF := session.CSRFToken
	adminCookie, adminCSRF := loginAs(t, env.mux, "transit.admin", routerAdminPass)

	payload := `{"vehicle_number":"GRT-404","route_id":7,"latitude":43.4516,"longitude":-80.4925,"status":"in_service"}`

	// Unauthenticated 401
	for _, path := range []string{
		"/api/vehicles/locations",
		"/api/admin/vehicles/simulator/start",
		"/api/admin/vehicles/simulator/stop",
		"/api/admin/vehicles/simulator/step",
		"/api/admin/vehicles/simulator/reset",
	} {
		rec := authedRequest(env.mux, http.MethodPost, path, payload, nil, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("POST %s unauthenticated = %d, want 401", path, rec.Code)
		}
	}

	// Non-admin 403
	rec := authedRequest(env.mux, http.MethodPost, "/api/vehicles/locations", payload, userCookie, userCSRF)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin POST /api/vehicles/locations = %d, want 403", rec.Code)
	}

	// Missing CSRF 403
	rec = authedRequest(env.mux, http.MethodPost, "/api/vehicles/locations", payload, adminCookie, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF POST /api/vehicles/locations = %d, want 403", rec.Code)
	}

	// Admin valid creation returns 201 and captures principal username as fallback source.
	rec = authedRequest(env.mux, http.MethodPost, "/api/vehicles/locations", payload, adminCookie, adminCSRF)
	if rec.Code != http.StatusCreated || svc.submitSource != "transit.admin" {
		t.Fatalf("POST create status=%d source=%q body=%s", rec.Code, svc.submitSource, rec.Body.String())
	}
	if !strings.Contains(env.auditBuf.String(), `"resource_type":"vehicle_location"`) {
		t.Fatalf("expected vehicle audit event, got %s", env.auditBuf.String())
	}

	// Subsequent update returns 200 OK.
	svc.submitResult.Created = false
	rec = authedRequest(env.mux, http.MethodPost, "/api/vehicles/locations", payload, adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST update status=%d, want 200", rec.Code)
	}

	// Stale conflict 409, not found 404, invalid input 400, internal error 500 without leaking details.
	for _, tc := range []struct {
		err    error
		status int
	}{
		{vehicles.ErrStaleLocationUpdate, 409},
		{vehicles.ErrNotFound, 404},
		{&vehicles.InvalidInput{Message: "latitude invalid"}, 400},
		{errors.New("secret postgres crash"), 500},
	} {
		svc.submitErr = tc.err
		errRec := authedRequest(env.mux, http.MethodPost, "/api/vehicles/locations", payload, adminCookie, adminCSRF)
		if errRec.Code != tc.status {
			t.Fatalf("err=%v status=%d, want %d", tc.err, errRec.Code, tc.status)
		}
		if strings.Contains(errRec.Body.String(), "secret") {
			t.Fatalf("internal error leaked: %s", errRec.Body.String())
		}
	}

	// Unknown JSON fields rejected with 400.
	badJSON := authedRequest(env.mux, http.MethodPost, "/api/vehicles/locations", `{"unknown_field":1}`, adminCookie, adminCSRF)
	if badJSON.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status=%d, want 400", badJSON.Code)
	}

	// Admin simulator endpoints work with auth & CSRF.
	startRec := authedRequest(env.mux, http.MethodPost, "/api/admin/vehicles/simulator/start", `{"speed_multiplier":2}`, adminCookie, adminCSRF)
	if startRec.Code != http.StatusOK || !sim.status.Running || sim.status.SpeedMultiplier != 2 {
		t.Fatalf("simulator start failed: code=%d status=%+v body=%s", startRec.Code, sim.status, startRec.Body.String())
	}
	stepRec := authedRequest(env.mux, http.MethodPost, "/api/admin/vehicles/simulator/step", "", adminCookie, adminCSRF)
	if stepRec.Code != http.StatusOK || sim.called != "step" {
		t.Fatalf("simulator step failed: code=%d called=%s", stepRec.Code, sim.called)
	}
	stopRec := authedRequest(env.mux, http.MethodPost, "/api/admin/vehicles/simulator/stop", "", adminCookie, adminCSRF)
	if stopRec.Code != http.StatusOK || sim.status.Running {
		t.Fatalf("simulator stop failed: code=%d status=%+v", stopRec.Code, sim.status)
	}
	resetRec := authedRequest(env.mux, http.MethodPost, "/api/admin/vehicles/simulator/reset", "", adminCookie, adminCSRF)
	if resetRec.Code != http.StatusOK || sim.called != "reset" {
		t.Fatalf("simulator reset failed: code=%d called=%s", resetRec.Code, sim.called)
	}
}

func TestVehicleArrivalEndpoints(t *testing.T) {
	now := time.Date(2026, 11, 18, 18, 0, 0, 0, time.UTC)
	sched := now.Add(5 * time.Minute)
	est := now.Add(7 * time.Minute)
	svc := &fakeVehicleService{
		arrivalsResult: []vehicles.EstimatedArrival{
			{
				TripID:               300,
				RouteID:              7,
				RouteNumber:          "7",
				RouteName:            "King",
				Direction:            "direction_0",
				StopID:               42,
				StopName:             "Central Station",
				StopSequence:         3,
				VehicleID:            11,
				VehicleNumber:        "SIM-001",
				VehicleStatus:        vehicles.StatusDelayed,
				ScheduledArrivalTime: &sched,
				EstimatedArrivalTime: est,
				DelayMinutes:         2,
				DistanceMeters:       1400,
				IsSimulated:          true,
				RecordedAt:           now,
			},
		},
	}

	// Register on full router alongside schedule routes to ensure zero chi parameter conflicts.
	fullMux := router.NewWithDependencies(nil, router.Dependencies{
		VehicleHandler: handlers.NewVehicleHandler(svc, nil),
	})

	rec := authedRequest(fullMux, http.MethodGet, "/api/trips/300/arrivals", "", nil, "")
	if rec.Code != http.StatusOK || svc.called != "upcoming_arrivals" || svc.arrivalsTripID != 300 || svc.arrivalsStopID != 0 {
		t.Fatalf("GET /api/trips/300/arrivals failed: code=%d called=%s trip=%d stop=%d body=%s",
			rec.Code, svc.called, svc.arrivalsTripID, svc.arrivalsStopID, rec.Body.String())
	}
	var gotTripArrivals []vehicles.EstimatedArrival
	if err := json.Unmarshal(rec.Body.Bytes(), &gotTripArrivals); err != nil {
		t.Fatal(err)
	}
	if len(gotTripArrivals) != 1 || gotTripArrivals[0].StopName != "Central Station" || gotTripArrivals[0].DelayMinutes != 2 || gotTripArrivals[0].DistanceMeters != 1400 {
		t.Fatalf("unexpected trip arrivals payload: %+v", gotTripArrivals)
	}

	rec = authedRequest(fullMux, http.MethodGet, "/api/stops/42/arrivals", "", nil, "")
	if rec.Code != http.StatusOK || svc.called != "stop_arrivals" || svc.arrivalsStopID != 42 || svc.arrivalsRouteID != 0 {
		t.Fatalf("GET /api/stops/42/arrivals failed: code=%d called=%s stop=%d route=%d body=%s",
			rec.Code, svc.called, svc.arrivalsStopID, svc.arrivalsRouteID, rec.Body.String())
	}

	rec = authedRequest(fullMux, http.MethodGet, "/api/stops/42/arrivals?route_id=7", "", nil, "")
	if rec.Code != http.StatusOK || svc.called != "stop_arrivals" || svc.arrivalsStopID != 42 || svc.arrivalsRouteID != 7 {
		t.Fatalf("GET /api/stops/42/arrivals?route_id=7 failed: code=%d called=%s stop=%d route=%d body=%s",
			rec.Code, svc.called, svc.arrivalsStopID, svc.arrivalsRouteID, rec.Body.String())
	}

	// Nil slice from service still serializes as JSON [] rather than null.
	svc.arrivalsResult = nil
	rec = authedRequest(fullMux, http.MethodGet, "/api/trips/300/arrivals", "", nil, "")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("empty arrivals = code %d body %q, want 200 []", rec.Code, rec.Body.String())
	}

	// 404 when trip or stop does not exist, 500 without leaking internal error text.
	svc.arrivalsErr = vehicles.ErrNotFound
	for _, path := range []string{"/api/trips/999/arrivals", "/api/stops/999/arrivals"} {
		notFound := authedRequest(fullMux, http.MethodGet, path, "", nil, "")
		if notFound.Code != http.StatusNotFound {
			t.Fatalf("%s returned %d, want 404", path, notFound.Code)
		}
	}

	svc.arrivalsErr = errors.New("secret db password")
	errRec := authedRequest(fullMux, http.MethodGet, "/api/stops/42/arrivals", "", nil, "")
	if errRec.Code != http.StatusInternalServerError || strings.Contains(errRec.Body.String(), "secret") {
		t.Fatalf("expected sanitized 500, got code=%d body=%s", errRec.Code, errRec.Body.String())
	}

	// Invalid IDs and unexpected/bad query parameters return 400.
	svc.arrivalsErr = nil
	for _, badPath := range []string{
		"/api/trips/0/arrivals",
		"/api/trips/abc/arrivals",
		"/api/trips/300/arrivals?bogus=1",
		"/api/stops/0/arrivals",
		"/api/stops/abc/arrivals",
		"/api/stops/42/arrivals?route_id=0",
		"/api/stops/42/arrivals?route_id=abc",
		"/api/stops/42/arrivals?route_id=7&route_id=8",
		"/api/stops/42/arrivals?bogus=1",
	} {
		badRec := authedRequest(fullMux, http.MethodGet, badPath, "", nil, "")
		if badRec.Code != http.StatusBadRequest {
			t.Fatalf("%s returned %d, want 400 (body=%s)", badPath, badRec.Code, badRec.Body.String())
		}
	}
}
