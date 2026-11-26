package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/auth"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/disruptions"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/handlers"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/router"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

type fakeDisruptionService struct {
	listFilter              disruptions.ListFilter
	listResult              []disruptions.DisruptionResponse
	listErr                 error
	getID                   int
	getResult               disruptions.DisruptionResponse
	getErr                  error
	createInput             disruptions.CreateDisruptionInput
	createSource            string
	createResult            disruptions.DisruptionResponse
	createErr               error
	updateID                int
	updateInput             disruptions.UpdateDisruptionInput
	updateResult            disruptions.DisruptionResponse
	updateErr               error
	roadClosureInput        disruptions.RoadClosureInput
	roadClosureSource       string
	roadClosureResult       disruptions.DisruptionResponse
	roadClosureErr          error
	getRoadClosureID        int
	getRoadClosureResult    disruptions.DisruptionResponse
	getRoadClosureErr       error
	updateRoadClosureID     int
	updateRoadClosureInput  disruptions.UpdateRoadClosureInput
	updateRoadClosureResult disruptions.DisruptionResponse
	updateRoadClosureErr    error
	affectedResult          disruptions.AffectedServicesSummary
	affectedErr             error
	serviceStatusResult     disruptions.ServiceStatusResponse
	serviceStatusErr        error
	entityID                int
	entityStatusResult      disruptions.EntityOperationalStatus
	entityStatusErr         error
	called                  string
}

func (f *fakeDisruptionService) List(_ context.Context, filter disruptions.ListFilter) ([]disruptions.DisruptionResponse, error) {
	f.called = "list"
	f.listFilter = filter
	return f.listResult, f.listErr
}

func (f *fakeDisruptionService) ListActive(_ context.Context, filter disruptions.ListFilter) ([]disruptions.DisruptionResponse, error) {
	f.called = "list_active"
	f.listFilter = filter
	return f.listResult, f.listErr
}

func (f *fakeDisruptionService) Get(_ context.Context, id int) (disruptions.DisruptionResponse, error) {
	f.called = "get"
	f.getID = id
	return f.getResult, f.getErr
}

func (f *fakeDisruptionService) Create(_ context.Context, input disruptions.CreateDisruptionInput, defaultSource string) (disruptions.DisruptionResponse, error) {
	f.called = "create"
	f.createInput = input
	f.createSource = defaultSource
	return f.createResult, f.createErr
}

func (f *fakeDisruptionService) Update(_ context.Context, id int, input disruptions.UpdateDisruptionInput) (disruptions.DisruptionResponse, error) {
	f.called = "update"
	f.updateID = id
	f.updateInput = input
	return f.updateResult, f.updateErr
}

func (f *fakeDisruptionService) CreateRoadClosure(_ context.Context, input disruptions.RoadClosureInput, defaultSource string) (disruptions.DisruptionResponse, error) {
	f.called = "create_road_closure"
	f.roadClosureInput = input
	f.roadClosureSource = defaultSource
	return f.roadClosureResult, f.roadClosureErr
}

func (f *fakeDisruptionService) GetRoadClosure(_ context.Context, id int) (disruptions.DisruptionResponse, error) {
	f.called = "get_road_closure"
	f.getRoadClosureID = id
	return f.getRoadClosureResult, f.getRoadClosureErr
}

func (f *fakeDisruptionService) UpdateRoadClosure(_ context.Context, id int, input disruptions.UpdateRoadClosureInput) (disruptions.DisruptionResponse, error) {
	f.called = "update_road_closure"
	f.updateRoadClosureID = id
	f.updateRoadClosureInput = input
	return f.updateRoadClosureResult, f.updateRoadClosureErr
}

func (f *fakeDisruptionService) AffectedServices(_ context.Context) (disruptions.AffectedServicesSummary, error) {
	f.called = "affected"
	return f.affectedResult, f.affectedErr
}

func (f *fakeDisruptionService) ServiceStatus(_ context.Context) (disruptions.ServiceStatusResponse, error) {
	f.called = "service_status"
	return f.serviceStatusResult, f.serviceStatusErr
}

func (f *fakeDisruptionService) RouteStatus(_ context.Context, routeID int) (disruptions.EntityOperationalStatus, error) {
	f.called = "route_status"
	f.entityID = routeID
	return f.entityStatusResult, f.entityStatusErr
}

func (f *fakeDisruptionService) StopStatus(_ context.Context, stopID int) (disruptions.EntityOperationalStatus, error) {
	f.called = "stop_status"
	f.entityID = stopID
	return f.entityStatusResult, f.entityStatusErr
}

func (f *fakeDisruptionService) TripStatus(_ context.Context, tripID int) (disruptions.EntityOperationalStatus, error) {
	f.called = "trip_status"
	f.entityID = tripID
	return f.entityStatusResult, f.entityStatusErr
}

func newTestDisruptionEnv(t *testing.T, svc handlers.DisruptionService) (*testAuthEnv, *fakeDisruptionService) {
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
	handler := handlers.NewDisruptionHandlerWithAudit(svc, auditLogger)

	mux := chi.NewRouter()
	mux.Route(router.APIBasePath(""), func(api chi.Router) {
		router.RegisterAuthRoutes(api, authHandler, authMW)
		router.RegisterDisruptionRoutes(api, handler, authMW, csrf, "TransitAdministrator", config.DefaultIntegrationRoles())
	})

	fakeSvc, _ := svc.(*fakeDisruptionService)
	return &testAuthEnv{
		mux:       mux,
		sessions:  sessions,
		validator: validator,
		csrf:      csrf,
		auditBuf:  &auditBuf,
	}, fakeSvc
}

func TestDisruptionPublicReadEndpoints(t *testing.T) {
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	svc := &fakeDisruptionService{
		listResult: []disruptions.DisruptionResponse{
			{
				ID:            12,
				Type:          disruptions.TypeDetour,
				Title:         "King Street Construction Detour",
				Status:        disruptions.StatusActive,
				SourceService: disruptions.SourceUtilities,
				StartTime:     now,
				ReceivedAt:    now,
				AffectedRoutes: []disruptions.AffectedRoute{
					{RouteID: 7, RouteNumber: "7", RouteName: "King", Direction: "direction_0"},
				},
				AffectedStops: []disruptions.AffectedStop{
					{StopID: 3, StopName: "Midtown Hub", RouteID: 7},
				},
				AffectedTrips:    []int{100},
				AffectedVehicles: []int{55},
				AffectedSegments: []string{"King St N"},
			},
		},
		getResult: disruptions.DisruptionResponse{
			ID:               12,
			Type:             disruptions.TypeDetour,
			Title:            "King Street Construction Detour",
			Status:           disruptions.StatusActive,
			SourceService:    disruptions.SourceUtilities,
			StartTime:        now,
			ReceivedAt:       now,
			AffectedRoutes:   []disruptions.AffectedRoute{},
			AffectedStops:    []disruptions.AffectedStop{},
			AffectedTrips:    []int{},
			AffectedVehicles: []int{},
			AffectedSegments: []string{},
		},
		affectedResult: disruptions.AffectedServicesSummary{
			GeneratedAt:           now,
			ActiveDisruptionCount: 1,
			AffectedRoutes: []disruptions.AffectedRouteSummary{
				{RouteID: 7, RouteNumber: "7", RouteName: "King", Direction: "direction_0", OperationalStatus: disruptions.OpStatusDetoured, DisruptionIDs: []int{12}},
			},
			AffectedStops:    []disruptions.AffectedStopSummary{},
			AffectedTrips:    []disruptions.AffectedTripSummary{},
			AffectedVehicles: []disruptions.AffectedVehicleSummary{},
			AffectedSegments: []disruptions.AffectedSegmentSummary{},
		},
		serviceStatusResult: disruptions.ServiceStatusResponse{
			GeneratedAt:           now,
			OverallStatus:         disruptions.OverallStatusDisrupted,
			ActiveDisruptionCount: 1,
			ActiveDisruptions:     []disruptions.DisruptionResponse{},
			Detours:               []disruptions.DisruptionResponse{},
			AffectedRoutes:        []disruptions.AffectedRouteSummary{},
			AffectedStops:         []disruptions.AffectedStopSummary{},
			AffectedTrips:         []disruptions.AffectedTripSummary{},
			AffectedVehicles:      []disruptions.AffectedVehicleSummary{},
			AffectedSegments:      []disruptions.AffectedSegmentSummary{},
		},
		entityStatusResult: disruptions.EntityOperationalStatus{
			EntityType:        "route",
			EntityID:          7,
			OperationalStatus: disruptions.OpStatusDetoured,
			ActiveDisruptions: []disruptions.DisruptionResponse{},
		},
	}

	fullMux := router.NewWithDependencies(nil, router.Dependencies{
		DisruptionHandler: handlers.NewDisruptionHandler(svc),
	})

	rec := authedRequest(fullMux, http.MethodGet, "/api/transport/v1/disruptions?status=active&type=detour&route_id=7&stop_id=3&trip_id=100&vehicle_id=55&source_service=utilities", "", nil, "")
	if rec.Code != http.StatusOK || svc.called != "list" || svc.listFilter.RouteID != 7 || svc.listFilter.StopID != 3 || svc.listFilter.SourceService != "utilities" {
		t.Fatalf("GET /api/disruptions failed: code=%d filter=%+v body=%s", rec.Code, svc.listFilter, rec.Body.String())
	}
	var gotList []disruptions.DisruptionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &gotList); err != nil {
		t.Fatal(err)
	}
	if len(gotList) != 1 || gotList[0].ID != 12 || len(gotList[0].AffectedRoutes) != 1 {
		t.Fatalf("unexpected decoded disruptions list: %+v", gotList)
	}

	rec = authedRequest(fullMux, http.MethodGet, "/api/transport/v1/disruptions/active?type=detour&route_id=7&stop_id=3&trip_id=100", "", nil, "")
	if rec.Code != http.StatusOK || svc.called != "list_active" || svc.listFilter.RouteID != 7 {
		t.Fatalf("GET /api/disruptions/active failed: code=%d filter=%+v body=%s", rec.Code, svc.listFilter, rec.Body.String())
	}

	rec = authedRequest(fullMux, http.MethodGet, "/api/transport/v1/disruptions/affected", "", nil, "")
	if rec.Code != http.StatusOK || svc.called != "affected" {
		t.Fatalf("GET /api/disruptions/affected failed: code=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = authedRequest(fullMux, http.MethodGet, "/api/transport/v1/disruptions/12", "", nil, "")
	if rec.Code != http.StatusOK || svc.called != "get" || svc.getID != 12 {
		t.Fatalf("GET /api/disruptions/12 failed: code=%d id=%d body=%s", rec.Code, svc.getID, rec.Body.String())
	}

	rec = authedRequest(fullMux, http.MethodGet, "/api/transport/v1/service-status", "", nil, "")
	if rec.Code != http.StatusOK || svc.called != "service_status" {
		t.Fatalf("GET /api/service-status failed: code=%d body=%s", rec.Code, rec.Body.String())
	}

	for _, ep := range []struct {
		path   string
		called string
		id     int
	}{
		{"/api/transport/v1/routes/7/status", "route_status", 7},
		{"/api/transport/v1/stops/3/status", "stop_status", 3},
		{"/api/transport/v1/trips/100/status", "trip_status", 100},
	} {
		rec = authedRequest(fullMux, http.MethodGet, ep.path, "", nil, "")
		if rec.Code != http.StatusOK || svc.called != ep.called || svc.entityID != ep.id {
			t.Fatalf("GET %s failed: code=%d called=%s id=%d body=%s", ep.path, rec.Code, svc.called, svc.entityID, rec.Body.String())
		}
	}

	// Missing resources return 404.
	svc.getErr = disruptions.ErrNotFound
	svc.entityStatusErr = disruptions.ErrNotFound
	for _, notFoundPath := range []string{
		"/api/transport/v1/disruptions/999",
		"/api/transport/v1/routes/999/status",
		"/api/transport/v1/stops/999/status",
		"/api/transport/v1/trips/999/status",
	} {
		nfRec := authedRequest(fullMux, http.MethodGet, notFoundPath, "", nil, "")
		if nfRec.Code != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404", notFoundPath, nfRec.Code)
		}
	}
	svc.getErr = nil
	svc.entityStatusErr = nil

	// Invalid IDs and unexpected/disallowed query parameters return 400.
	for _, badPath := range []string{
		"/api/transport/v1/disruptions?bogus=1",
		"/api/transport/v1/disruptions?route_id=0",
		"/api/transport/v1/disruptions?route_id=abc",
		"/api/transport/v1/disruptions?status=",
		"/api/transport/v1/disruptions/active?status=resolved",
		"/api/transport/v1/disruptions/active?bogus=1",
		"/api/transport/v1/disruptions/affected?bogus=1",
		"/api/transport/v1/disruptions/0",
		"/api/transport/v1/disruptions/abc",
		"/api/transport/v1/disruptions/12?bogus=1",
		"/api/transport/v1/service-status?bogus=1",
		"/api/transport/v1/routes/0/status",
		"/api/transport/v1/routes/7/status?bogus=1",
		"/api/transport/v1/stops/0/status",
		"/api/transport/v1/trips/0/status",
	} {
		badRec := authedRequest(fullMux, http.MethodGet, badPath, "", nil, "")
		if badRec.Code != http.StatusBadRequest {
			t.Fatalf("GET %s = %d, want 400 (body=%s)", badPath, badRec.Code, badRec.Body.String())
		}
	}
}

func TestDisruptionProtectedEndpointsEnforceFailClosedAuthRoleCSRFAndAudit(t *testing.T) {
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	extID := "UTIL-101"

	// 1. Fail-closed verification when auth dependencies are nil.
	unconfiguredMux := router.New(nil)
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/transport/v1/disruptions"},
		{http.MethodPatch, "/api/transport/v1/disruptions/1"},
		{http.MethodPost, "/api/transport/v1/road-closures"},
		{http.MethodPatch, "/api/transport/v1/road-closures/1"},
		{http.MethodGet, "/api/transport/v1/integrations/service-status"},
	} {
		rec := authedRequest(unconfiguredMux, tc.method, tc.path, `{}`, nil, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("fail-closed %s %s = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}

	// 2. Configured auth environment tests.
	svc := &fakeDisruptionService{
		createResult: disruptions.DisruptionResponse{
			ID:            42,
			Type:          disruptions.TypeDetour,
			Title:         "King Detour",
			Status:        disruptions.StatusActive,
			SourceService: disruptions.SourceTransitAdmin,
			StartTime:     now,
			ReceivedAt:    now,
		},
		updateResult: disruptions.DisruptionResponse{
			ID:            42,
			Type:          disruptions.TypeDetour,
			Title:         "King Detour Resolved",
			Status:        disruptions.StatusResolved,
			SourceService: disruptions.SourceTransitAdmin,
			StartTime:     now,
			ReceivedAt:    now,
		},
		roadClosureResult: disruptions.DisruptionResponse{
			ID:                88,
			Type:              disruptions.TypeRoadClosure,
			Title:             "Watermain Break",
			Status:            disruptions.StatusActive,
			SourceService:     disruptions.SourceTransitAdmin,
			ExternalReference: &extID,
			StartTime:         now,
			ReceivedAt:        now,
		},
		updateRoadClosureResult: disruptions.DisruptionResponse{
			ID:            88,
			Type:          disruptions.TypeRoadClosure,
			Title:         "Watermain Break",
			Status:        disruptions.StatusResolved,
			SourceService: disruptions.SourceTransitAdmin,
			StartTime:     now,
			ReceivedAt:    now,
		},
		serviceStatusResult: disruptions.ServiceStatusResponse{
			GeneratedAt:           now,
			OverallStatus:         disruptions.OverallStatusDisrupted,
			ActiveDisruptionCount: 1,
			ActiveDisruptions:     []disruptions.DisruptionResponse{},
			Detours:               []disruptions.DisruptionResponse{},
		},
	}
	env, _ := newTestDisruptionEnv(t, svc)

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

	// Unauthenticated 401 and non-admin 403 across all protected endpoints.
	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/transport/v1/disruptions", `{"type":"detour","title":"Detour"}`},
		{http.MethodPatch, "/api/transport/v1/disruptions/42", `{"status":"resolved"}`},
		{http.MethodPost, "/api/transport/v1/road-closures", `{"title":"Watermain Break"}`},
		{http.MethodPatch, "/api/transport/v1/road-closures/88", `{"status":"resolved"}`},
		{http.MethodGet, "/api/transport/v1/integrations/service-status", ""},
	} {
		unauthRec := authedRequest(env.mux, tc.method, tc.path, tc.body, nil, "")
		if unauthRec.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s %s = %d, want 401", tc.method, tc.path, unauthRec.Code)
		}
		nonAdminRec := authedRequest(env.mux, tc.method, tc.path, tc.body, userCookie, userCSRF)
		if nonAdminRec.Code != http.StatusForbidden {
			t.Fatalf("non-admin %s %s = %d, want 403", tc.method, tc.path, nonAdminRec.Code)
		}
		if tc.method != http.MethodGet {
			noCSRFRec := authedRequest(env.mux, tc.method, tc.path, tc.body, adminCookie, "")
			if noCSRFRec.Code != http.StatusForbidden {
				t.Fatalf("missing CSRF %s %s = %d, want 403", tc.method, tc.path, noCSRFRec.Code)
			}
		}
	}

	// Enterprise service status GET works with admin cookie and no CSRF token.
	entRec := authedRequest(env.mux, http.MethodGet, "/api/transport/v1/integrations/service-status", "", adminCookie, "")
	if entRec.Code != http.StatusOK || svc.called != "service_status" {
		t.Fatalf("GET /api/integrations/service-status failed: code=%d body=%s", entRec.Code, entRec.Body.String())
	}

	// Admin POST /api/disruptions -> 201 Created + Location header + audit event.
	createRec := authedRequest(env.mux, http.MethodPost, "/api/transport/v1/disruptions", `{"type":"detour","title":"King Detour","route_ids":[7]}`, adminCookie, adminCSRF)
	if createRec.Code != http.StatusCreated || createRec.Header().Get("Location") != "/api/transport/v1/disruptions/42" {
		t.Fatalf("POST /api/disruptions failed: code=%d loc=%q body=%s", createRec.Code, createRec.Header().Get("Location"), createRec.Body.String())
	}

	// Admin PATCH /api/disruptions/42 with explicit null end_time -> 200 OK.
	patchRec := authedRequest(env.mux, http.MethodPatch, "/api/transport/v1/disruptions/42", `{"status":"resolved","end_time":null}`, adminCookie, adminCSRF)
	if patchRec.Code != http.StatusOK || svc.updateID != 42 || !svc.updateInput.EndTimeSet {
		t.Fatalf("PATCH /api/disruptions/42 failed: code=%d input=%+v body=%s", patchRec.Code, svc.updateInput, patchRec.Body.String())
	}

	// Road closure POST via admin session always returns 201 Created, and PATCH resolves by ID (200 OK).
	rcBody := `{"external_id":"UTIL-101","title":"Watermain Break","latitude":43.45,"longitude":-80.49,"radius_meters":400}`
	rec201 := authedRequest(env.mux, http.MethodPost, "/api/transport/v1/road-closures", rcBody, adminCookie, adminCSRF)
	if rec201.Code != http.StatusCreated {
		t.Fatalf("POST /road-closures = %d, want 201: %s", rec201.Code, rec201.Body.String())
	}
	patchRCRec := authedRequest(env.mux, http.MethodPatch, "/api/transport/v1/road-closures/88", `{"status":"resolved"}`, adminCookie, adminCSRF)
	if patchRCRec.Code != http.StatusOK || svc.updateRoadClosureID != 88 {
		t.Fatalf("PATCH /road-closures/88 = %d (id=%d), want 200: %s", patchRCRec.Code, svc.updateRoadClosureID, patchRCRec.Body.String())
	}

	if !strings.Contains(env.auditBuf.String(), `"admin_disruption_operation"`) {
		t.Fatalf("expected structured disruption audit events, got %s", env.auditBuf.String())
	}

	// Unknown JSON fields return 400; service errors map to 400/404/500 without leaking secrets.
	badJSON := authedRequest(env.mux, http.MethodPost, "/api/transport/v1/disruptions", `{"unknown":1}`, adminCookie, adminCSRF)
	if badJSON.Code != http.StatusBadRequest {
		t.Fatalf("unknown JSON field = %d, want 400", badJSON.Code)
	}

	for _, errCase := range []struct {
		err    error
		status int
	}{
		{&disruptions.InvalidInput{Message: "title is required"}, http.StatusBadRequest},
		{disruptions.ErrNotFound, http.StatusNotFound},
		{errors.New("secret postgres failure"), http.StatusInternalServerError},
	} {
		svc.createErr = errCase.err
		errRec := authedRequest(env.mux, http.MethodPost, "/api/transport/v1/disruptions", `{"type":"delay","title":"Test"}`, adminCookie, adminCSRF)
		if errRec.Code != errCase.status || strings.Contains(errRec.Body.String(), "secret") {
			t.Fatalf("err=%v status=%d body=%s, want %d", errCase.err, errRec.Code, errRec.Body.String(), errCase.status)
		}
	}
}

func TestRoadClosuresGetPostAndPatch(t *testing.T) {
	svc := &fakeDisruptionService{
		listResult: []disruptions.DisruptionResponse{{ID: 10, Type: disruptions.TypeRoadClosure, Title: "King St Closure", Status: "active"}},
		getRoadClosureResult: disruptions.DisruptionResponse{
			ID:     11,
			Type:   disruptions.TypeRoadClosure,
			Title:  "Road closure: 45 Weber St E, Kitchener, N2H 1C4",
			Status: "active",
		},
		roadClosureResult: disruptions.DisruptionResponse{
			ID:            11,
			Type:          disruptions.TypeRoadClosure,
			Title:         "Road closure: 45 Weber St E, Kitchener, N2H 1C4",
			Status:        "active",
			SourceService: disruptions.SourceTransitAdmin,
		},
		updateRoadClosureResult: disruptions.DisruptionResponse{
			ID:            11,
			Type:          disruptions.TypeRoadClosure,
			Title:         "Road closure: 45 Weber St E, Kitchener, N2H 1C4",
			Status:        "resolved",
			SourceService: disruptions.SourceTransitAdmin,
		},
	}
	env, _ := newTestDisruptionEnv(t, svc)

	getRec := authedRequest(env.mux, http.MethodGet, "/api/transport/v1/road-closures?status=active", "", nil, "")
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /api/transport/v1/road-closures = %d, want 200: %s", getRec.Code, getRec.Body.String())
	}
	if svc.listFilter.Type != disruptions.TypeRoadClosure || svc.listFilter.Status != "active" {
		t.Fatalf("unexpected filter: %+v", svc.listFilter)
	}

	getOneRec := authedRequest(env.mux, http.MethodGet, "/api/transport/v1/road-closures/11", "", nil, "")
	if getOneRec.Code != http.StatusOK || svc.called != "get_road_closure" || svc.getRoadClosureID != 11 {
		t.Fatalf("GET /api/transport/v1/road-closures/11 = %d (id=%d), want 200: %s", getOneRec.Code, svc.getRoadClosureID, getOneRec.Body.String())
	}

	svc.getRoadClosureErr = disruptions.ErrNotFound
	notFoundRec := authedRequest(env.mux, http.MethodGet, "/api/transport/v1/road-closures/999", "", nil, "")
	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("GET /api/transport/v1/road-closures/999 = %d, want 404", notFoundRec.Code)
	}
	svc.getRoadClosureErr = nil

	badTypeGet := authedRequest(env.mux, http.MethodGet, "/api/transport/v1/road-closures?type=delay", "", nil, "")
	if badTypeGet.Code != http.StatusBadRequest {
		t.Fatalf("GET /api/transport/v1/road-closures?type=delay = %d, want 400", badTypeGet.Code)
	}

	unauthPost := authedRequest(env.mux, http.MethodPost, "/api/transport/v1/road-closures", `{"title":"Weber St Closure"}`, nil, "")
	if unauthPost.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST /road-closures = %d, want 401", unauthPost.Code)
	}

	adminCookie, adminCSRF := loginAs(t, env.mux, "transit.admin", routerAdminPass)
	postRec := authedRequest(
		env.mux,
		http.MethodPost,
		"/api/transport/v1/road-closures",
		`{"address":{"streetNumber":45,"streetName":"Weber St E","city":"Kitchener","postalCode":"N2H 1C4"},"valid-until":3600}`,
		adminCookie,
		adminCSRF,
	)
	if postRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/transport/v1/road-closures = %d, want 201: %s", postRec.Code, postRec.Body.String())
	}
	if svc.called != "create_road_closure" || svc.roadClosureInput.Address == nil || svc.roadClosureInput.Address.StreetNumber != "45" || svc.roadClosureInput.ValidUntil == nil || *svc.roadClosureInput.ValidUntil != 3600 {
		t.Fatalf("unexpected roadClosureInput: %+v", svc.roadClosureInput)
	}
	if svc.roadClosureSource != disruptions.SourceTransitAdmin {
		t.Fatalf("roadClosureSource = %q, want %q", svc.roadClosureSource, disruptions.SourceTransitAdmin)
	}
	if loc := postRec.Header().Get("Location"); loc != "/api/transport/v1/road-closures/11" {
		t.Fatalf("Location = %q, want /api/transport/v1/road-closures/11", loc)
	}

	// PATCH /road-closures/{id} updates/resolves by ID and returns 200 OK.
	patchRec := authedRequest(
		env.mux,
		http.MethodPatch,
		"/api/transport/v1/road-closures/11",
		`{"status":"resolved","end_time":null}`,
		adminCookie,
		adminCSRF,
	)
	if patchRec.Code != http.StatusOK || svc.called != "update_road_closure" || svc.updateRoadClosureID != 11 || !svc.updateRoadClosureInput.EndTimeSet {
		t.Fatalf("PATCH /api/transport/v1/road-closures/11 = %d, want 200: %s", patchRec.Code, patchRec.Body.String())
	}

	// Non-road-closure ID on PATCH returns 404.
	svc.updateRoadClosureErr = disruptions.ErrNotFound
	patchNotFound := authedRequest(env.mux, http.MethodPatch, "/api/transport/v1/road-closures/999", `{"status":"resolved"}`, adminCookie, adminCSRF)
	if patchNotFound.Code != http.StatusNotFound {
		t.Fatalf("PATCH /api/transport/v1/road-closures/999 = %d, want 404", patchNotFound.Code)
	}
	svc.updateRoadClosureErr = nil

	// Invalid address field type returns 400.
	badAddr := authedRequest(env.mux, http.MethodPost, "/api/transport/v1/road-closures", `{"address":{"streetNumber":-1,"streetName":"Weber St","city":"Kitchener"}}`, adminCookie, adminCSRF)
	if badAddr.Code != http.StatusBadRequest {
		t.Fatalf("POST /road-closures with negative streetNumber = %d, want 400", badAddr.Code)
	}
}

func bearerRequest(mux http.Handler, method, path, body, authHeader string, cookie *http.Cookie, csrfToken string) *httptest.ResponseRecorder {
	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	} else {
		bodyReader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if cookie != nil {
		req.AddCookie(cookie)
		req.Header.Set("Origin", routerTestOrigin)
	}
	if csrfToken != "" {
		req.Header.Set(auth.CSRFHeaderName, csrfToken)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestIntegrationEndpointsBearerJWTAuth(t *testing.T) {
	now := time.Date(2026, 11, 25, 20, 0, 0, 0, time.UTC)
	svc := &fakeDisruptionService{
		roadClosureResult: disruptions.DisruptionResponse{
			ID:            99,
			Type:          disruptions.TypeRoadClosure,
			Title:         "Road closure: 123 King St N, Waterloo, N2J 2X7",
			Status:        disruptions.StatusActive,
			SourceService: "emergency.service",
			StartTime:     now,
			ReceivedAt:    now,
		},
		updateRoadClosureResult: disruptions.DisruptionResponse{
			ID:            99,
			Type:          disruptions.TypeRoadClosure,
			Title:         "Road closure: 123 King St N, Waterloo, N2J 2X7",
			Status:        disruptions.StatusResolved,
			SourceService: "emergency.service",
			StartTime:     now,
			ReceivedAt:    now,
		},
		serviceStatusResult: disruptions.ServiceStatusResponse{
			GeneratedAt:       now,
			OverallStatus:     disruptions.OverallStatusNormal,
			ActiveDisruptions: []disruptions.DisruptionResponse{},
			Detours:           []disruptions.DisruptionResponse{},
			AffectedRoutes:    []disruptions.AffectedRouteSummary{},
			AffectedStops:     []disruptions.AffectedStopSummary{},
			AffectedTrips:     []disruptions.AffectedTripSummary{},
			AffectedVehicles:  []disruptions.AffectedVehicleSummary{},
			AffectedSegments:  []disruptions.AffectedSegmentSummary{},
		},
	}
	env, _ := newTestDisruptionEnv(t, svc)

	signJWT := func(secret, sub, username string, role any, exp time.Time) string {
		claims := jwt.MapClaims{
			"sub": sub,
			"iss": routerTestIssuer,
			"aud": routerTestAudience,
			"exp": exp.Unix(),
		}
		if username != "" {
			claims["unique_name"] = username
		}
		if role != nil {
			claims["role"] = role
		}
		tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}

	roadClosurePayload := `{"address":{"streetNumber":123,"streetName":"King St N","city":"Waterloo","postalCode":"N2J 2X7"},"valid-until":7200}`
	endpoints := []struct {
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{http.MethodGet, "/api/transport/v1/integrations/service-status", "", http.StatusOK},
		{http.MethodPost, "/api/transport/v1/road-closures", roadClosurePayload, http.StatusCreated},
		{http.MethodPatch, "/api/transport/v1/road-closures/99", `{"status":"resolved"}`, http.StatusOK},
	}

	// 1. Every allowed integration role succeeds on POST /road-closures and GET /integrations/service-status without cookie, Origin, or CSRF.
	for _, role := range config.DefaultIntegrationRoles() {
		sub := "svc-" + strings.ReplaceAll(strings.ToLower(role), " ", "-")
		username := "caller-" + strings.ReplaceAll(strings.ToLower(role), " ", "-")
		token := signJWT(routerTestSecret, sub, username, role, time.Now().Add(15*time.Minute))
		for _, ep := range endpoints {
			rec := bearerRequest(env.mux, ep.method, ep.path, ep.body, "Bearer "+token, nil, "")
			if rec.Code != ep.wantStatus {
				t.Fatalf("role=%q %s %s = %d, want %d (body=%s)", role, ep.method, ep.path, rec.Code, ep.wantStatus, rec.Body.String())
			}
			if ep.method == http.MethodPost && svc.roadClosureSource != username {
				t.Fatalf("roadClosureSource = %q, want %q", svc.roadClosureSource, username)
			}
		}
		if !strings.Contains(env.auditBuf.String(), `"subject":"`+sub+`"`) {
			t.Fatalf("expected audit log subject %q, got %s", sub, env.auditBuf.String())
		}
	}

	// 2. Disallowed roles (User, Police Officer) receive 403.
	for _, disallowed := range []string{"User", "Police Officer"} {
		token := signJWT(routerTestSecret, "blocked-sub", "blocked", disallowed, time.Now().Add(15*time.Minute))
		for _, ep := range endpoints {
			rec := bearerRequest(env.mux, ep.method, ep.path, ep.body, "Bearer "+token, nil, "")
			if rec.Code != http.StatusForbidden {
				t.Fatalf("disallowed role=%q %s %s = %d, want 403", disallowed, ep.method, ep.path, rec.Code)
			}
		}
	}

	// 3. Expired, wrong-secret, or malformed Bearer headers return 401 even if paired with valid admin cookie + CSRF.
	adminCookie, adminCSRF := loginAs(t, env.mux, "transit.admin", routerAdminPass)
	expired := signJWT(routerTestSecret, "svc", "svc", "Administrator", time.Now().Add(-time.Minute))
	wrongSecret := signJWT("wrong-secret-0123456789abcdef0123", "svc", "svc", "Administrator", time.Now().Add(15*time.Minute))
	for _, badHeader := range []string{
		"Bearer " + expired,
		"Bearer " + wrongSecret,
		"Basic dXNlcjpwYXNz",
		"Bearer",
	} {
		for _, ep := range endpoints {
			rec := bearerRequest(env.mux, ep.method, ep.path, ep.body, badHeader, adminCookie, adminCSRF)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("badHeader=%q %s %s = %d, want 401", badHeader, ep.method, ep.path, rec.Code)
			}
		}
	}

	// 4. Non-integration admin disruption mutations remain strictly cookie-authenticated (reject Bearer JWT with 401).
	adminBearer := signJWT(routerTestSecret, "svc-admin", "svc-admin", "TransitAdministrator", time.Now().Add(15*time.Minute))
	for _, adminOnly := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/transport/v1/disruptions", `{"type":"detour","title":"Test"}`},
		{http.MethodPatch, "/api/transport/v1/disruptions/42", `{"status":"resolved"}`},
	} {
		rec := bearerRequest(env.mux, adminOnly.method, adminOnly.path, adminOnly.body, "Bearer "+adminBearer, nil, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("non-integration %s %s with Bearer JWT = %d, want 401", adminOnly.method, adminOnly.path, rec.Code)
		}
	}

	// 5. Removed per-group endpoints return 404.
	for _, removedPath := range []string{
		"/api/transport/v1/integrations/utilities/notices",
		"/api/transport/v1/integrations/emergency/incidents",
		"/api/transport/v1/integrations/security/traffic-alerts",
	} {
		rec := bearerRequest(env.mux, http.MethodPost, removedPath, `{"title":"Removed"}`, "Bearer "+adminBearer, nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("removed endpoint %s returned %d, want 404", removedPath, rec.Code)
		}
	}

	// 6. End-to-end RegistrationProvider (/api/Login/validate) via NewMiddlewareWithBearerValidator on POST /road-closures.
	const claimTypeRole = "http://schemas.microsoft.com/ws/2008/06/identity/claims/role"
	const claimTypeName = "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name"
	const claimTypeSub = "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/nameidentifier"

	regServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/Login/validate" {
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Header.Get("Authorization") {
		case "Bearer emg-dispatcher-token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authenticated": true,
				"claims": []map[string]string{
					{"type": claimTypeSub, "value": "emg-svc-id"},
					{"type": claimTypeName, "value": "emergency.service"},
					{"type": claimTypeRole, "value": "Dispatcher"},
				},
			})
		case "Bearer regular-user-token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authenticated": true,
				"claims": []map[string]string{
					{"type": claimTypeSub, "value": "resident-1"},
					{"type": claimTypeName, "value": "resident"},
					{"type": claimTypeRole, "value": "User"},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authenticated": false,
				"claims":        []map[string]string{},
			})
		}
	}))
	defer regServer.Close()

	regTrust := config.JWTTrustConfig{
		Issuer:   "public-transport-local",
		Audience: "public-transport-api",
		Secret:   []byte(routerTestSecret),
	}
	regProvider, err := auth.NewRegistrationProvider(config.RegistrationAuthConfig{
		ServiceURL:       regServer.URL,
		UpstreamIssuer:   "CmRegistrationService",
		UpstreamAudience: "CmRegistrationClients",
		LocalTrust:       regTrust,
		AdminUsers:       []string{"transit.admin"},
		RequiredRole:     "TransitAdministrator",
	}, regServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	localValidator, err := auth.NewJWTValidator(regTrust, nil)
	if err != nil {
		t.Fatal(err)
	}
	regMW := auth.NewMiddlewareWithBearerValidator(env.sessions, localValidator, regProvider, "transit_session", false)
	regMux := chi.NewRouter()
	regMux.Route(router.APIBasePath(""), func(api chi.Router) {
		router.RegisterDisruptionRoutes(api, handlers.NewDisruptionHandler(svc), regMW, env.csrf, "TransitAdministrator", config.DefaultIntegrationRoles())
	})

	rcPath := "/api/transport/v1/road-closures"
	if rec := bearerRequest(regMux, http.MethodPost, rcPath, roadClosurePayload, "Bearer emg-dispatcher-token", nil, ""); rec.Code != http.StatusCreated {
		t.Fatalf("RegistrationProvider Dispatcher token = %d, want 201 (%s)", rec.Code, rec.Body.String())
	} else if svc.roadClosureSource != "emergency.service" {
		t.Fatalf("roadClosureSource = %q, want emergency.service", svc.roadClosureSource)
	}
	if rec := bearerRequest(regMux, http.MethodPost, rcPath, roadClosurePayload, "Bearer regular-user-token", nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("RegistrationProvider User token = %d, want 403", rec.Code)
	}
	if rec := bearerRequest(regMux, http.MethodPost, rcPath, roadClosurePayload, "Bearer invalid-token", nil, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("RegistrationProvider unauthenticated token = %d, want 401", rec.Code)
	}
}
