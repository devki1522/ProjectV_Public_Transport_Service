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
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/handlers"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/models"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/router"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/services"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

const (
	routerTestOrigin   = "http://localhost:3000"
	routerTestIssuer   = "transit-dev"
	routerTestAudience = "transit-api"
	routerTestSecret   = "0123456789abcdef0123456789abcdef"
	routerAdminPass    = "admin-pass-123"
	routerUserPass     = "user-pass-123"
)

// An in-memory service keeps HTTP tests independent of a running database.
type fakeTransitRoutes struct {
	route  models.Route
	err    error
	called string
	id     int
	create services.CreateTransitRouteInput
	update services.UpdateTransitRouteInput
}

func (f *fakeTransitRoutes) List(context.Context) ([]models.Route, error) {
	f.called = "list"
	return nil, f.err
}
func (f *fakeTransitRoutes) Get(_ context.Context, id int) (models.Route, error) {
	f.called, f.id = "get", id
	return f.route, f.err
}
func (f *fakeTransitRoutes) Create(_ context.Context, input services.CreateTransitRouteInput) (models.Route, error) {
	f.called, f.create = "create", input
	return f.route, f.err
}
func (f *fakeTransitRoutes) Update(_ context.Context, id int, input services.UpdateTransitRouteInput) (models.Route, error) {
	f.called, f.id, f.update = "update", id, input
	return f.route, f.err
}
func (f *fakeTransitRoutes) Deactivate(_ context.Context, id int) error {
	f.called, f.id = "deactivate", id
	return f.err
}

type fakeDashboard struct {
	summary services.DashboardSummary
	err     error
}

func (f *fakeDashboard) Summary(context.Context) (services.DashboardSummary, error) {
	return f.summary, f.err
}

type testAuthEnv struct {
	mux       *chi.Mux
	sessions  *auth.InMemorySessionStore
	validator *auth.JWTValidator
	csrf      *auth.CSRFProtector
	auditBuf  *bytes.Buffer
}

func newTestAuthEnv(t *testing.T, service *fakeTransitRoutes, dashboard *fakeDashboard) *testAuthEnv {
	t.Helper()
	now := func() time.Time { return time.Now().UTC() }
	trust := config.JWTTrustConfig{
		Issuer:   routerTestIssuer,
		Audience: routerTestAudience,
		Secret:   []byte(routerTestSecret),
	}
	provider, err := auth.NewMockProvider(trust, config.MockAuthConfig{
		AdminPassword: routerAdminPass,
		UserPassword:  routerUserPass,
	}, now)
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
	authHandler := auth.NewHandler(provider, validator, sessions, csrf, throttler, "transit_session", false, now)
	authMW := auth.NewMiddleware(sessions, validator, "transit_session", false)

	var auditBuf bytes.Buffer
	auditLogger := slog.New(slog.NewJSONHandler(&auditBuf, nil))

	mux := chi.NewRouter()
	router.RegisterAuthRoutes(mux, authHandler, authMW)
	router.RegisterTransitRoutes(
		mux,
		handlers.NewTransitRouteHandlerWithAudit(service, auditLogger),
		authMW,
		csrf,
		"TransitAdministrator",
	)
	if dashboard != nil {
		router.RegisterAdminRoutes(mux, handlers.NewAdminHandler(dashboard), authMW, "TransitAdministrator")
	}

	return &testAuthEnv{
		mux:       mux,
		sessions:  sessions,
		validator: validator,
		csrf:      csrf,
		auditBuf:  &auditBuf,
	}
}

func loginAs(t *testing.T, handler http.Handler, username, password string) (*http.Cookie, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"username":   username,
		"password":   password,
		"rememberMe": false,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Origin", routerTestOrigin)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login(%s) status=%d body=%s", username, rec.Code, rec.Body.String())
	}
	var payload auth.SessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return rec.Result().Cookies()[0], payload.CSRFToken
}

func authedRequest(handler http.Handler, method, path, body string, cookie *http.Cookie, csrfToken string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if method != http.MethodGet {
		req.Header.Set("Origin", routerTestOrigin)
		if csrfToken != "" {
			req.Header.Set(auth.CSRFHeaderName, csrfToken)
		}
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestRouteEndpointsWithAuthorization(t *testing.T) {
	for _, tc := range []struct {
		method, path, body, call string
		status                   int
		needsAuth                bool
	}{
		{"GET", "/api/routes", "", "list", 200, false},
		{"GET", "/api/routes/42", "", "get", 200, false},
		{"POST", "/api/routes", `{"number":"7","name":"Main","direction":"direction_0"}`, "create", 201, true},
		{"PATCH", "/api/routes/42", `{"is_active":false}`, "update", 200, true},
		{"DELETE", "/api/routes/42", "", "deactivate", 204, true},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			service := &fakeTransitRoutes{route: models.Route{ID: 42, Number: "7", Name: "Main", Direction: "direction_0"}}
			env := newTestAuthEnv(t, service, nil)

			var cookie *http.Cookie
			var csrfToken string
			if tc.needsAuth {
				cookie, csrfToken = loginAs(t, env.mux, "transit.admin", routerAdminPass)
			}

			response := authedRequest(env.mux, tc.method, tc.path, tc.body, cookie, csrfToken)
			if response.Code != tc.status || service.called != tc.call {
				t.Fatalf("status=%d call=%s body=%s", response.Code, service.called, response.Body)
			}
			if tc.call == "list" && strings.TrimSpace(response.Body.String()) != "[]" {
				t.Fatal("empty list must be []")
			}
			if tc.call == "create" {
				if response.Header().Get("Location") != "/api/routes/42" || service.create.Number != "7" {
					t.Fatal("create input or location missing")
				}
			}
			if tc.call == "update" && (service.update.IsActive == nil || *service.update.IsActive) {
				t.Fatal("explicit false lost")
			}
			if tc.call == "deactivate" && response.Body.Len() != 0 {
				t.Fatal("204 must have no body")
			}
			if tc.call == "get" {
				var body map[string]any
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if len(body) != 7 || body["id"] != float64(42) {
					t.Fatalf("unexpected response: %v", body)
				}
			}
			if tc.needsAuth {
				auditOutput := env.auditBuf.String()
				if !strings.Contains(auditOutput, `"subject":"dev-transit-admin-001"`) || !strings.Contains(auditOutput, `"outcome":"success"`) {
					t.Fatalf("expected structured audit event, got: %s", auditOutput)
				}
			}
		})
	}
}

func TestRouteWritesEnforceAuthRoleAndCSRF(t *testing.T) {
	service := &fakeTransitRoutes{route: models.Route{ID: 42, Number: "7", Name: "Main", Direction: "direction_0"}}
	dashboard := &fakeDashboard{summary: services.DashboardSummary{TotalRoutes: 5, ActiveRoutes: 4, InactiveRoutes: 1}}
	env := newTestAuthEnv(t, service, dashboard)

	userCookie, userCSRF := loginAs(t, env.mux, "transit.user", routerUserPass)
	adminCookie, _ := loginAs(t, env.mux, "transit.admin", routerAdminPass)

	// Unauthenticated writes return 401
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/api/routes", `{"number":"7","name":"Main","direction":"direction_0"}`},
		{"PATCH", "/api/routes/42", `{"is_active":false}`},
		{"DELETE", "/api/routes/42", ""},
		{"GET", "/api/admin/health", ""},
		{"GET", "/api/admin/dashboard", ""},
	} {
		rec := authedRequest(env.mux, tc.method, tc.path, tc.body, nil, "")
		if rec.Code != http.StatusUnauthorized || service.called != "" {
			t.Fatalf("%s %s unauthenticated = %d called=%s", tc.method, tc.path, rec.Code, service.called)
		}
	}

	// Normal user receives 403 from admin endpoints and route writes
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/api/routes", `{"number":"7","name":"Main","direction":"direction_0"}`},
		{"PATCH", "/api/routes/42", `{"is_active":false}`},
		{"DELETE", "/api/routes/42", ""},
		{"GET", "/api/admin/health", ""},
		{"GET", "/api/admin/dashboard", ""},
	} {
		rec := authedRequest(env.mux, tc.method, tc.path, tc.body, userCookie, userCSRF)
		if rec.Code != http.StatusForbidden || service.called != "" {
			t.Fatalf("%s %s normal user = %d called=%s", tc.method, tc.path, rec.Code, service.called)
		}
	}

	// Admin without CSRF token receives 403
	recNoCSRF := authedRequest(env.mux, http.MethodPost, "/api/routes", `{"number":"7","name":"Main","direction":"direction_0"}`, adminCookie, "")
	if recNoCSRF.Code != http.StatusForbidden || service.called != "" {
		t.Fatalf("missing CSRF = %d called=%s", recNoCSRF.Code, service.called)
	}

	// Admin dashboard returns repository data
	dashRec := authedRequest(env.mux, http.MethodGet, "/api/admin/dashboard", "", adminCookie, "")
	if dashRec.Code != http.StatusOK {
		t.Fatalf("dashboard status=%d body=%s", dashRec.Code, dashRec.Body.String())
	}
	var gotSummary services.DashboardSummary
	if err := json.Unmarshal(dashRec.Body.Bytes(), &gotSummary); err != nil {
		t.Fatal(err)
	}
	if gotSummary != dashboard.summary {
		t.Fatalf("dashboard = %+v, want %+v", gotSummary, dashboard.summary)
	}
}

func TestFakeRegistrationServiceEndToEndFlow(t *testing.T) {
	now := time.Now().UTC()
	signed := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":         "cm-admin-77",
		"unique_name": "city.transit.admin",
		"role":        []string{"User", "TransitAdministrator"},
		"iss":         "CmRegistrationService",
		"aud":         "CmRegistrationClients",
		"exp":         now.Add(15 * time.Minute).Unix(),
	})
	jwtString, err := signed.SignedString([]byte(routerTestSecret))
	if err != nil {
		t.Fatal(err)
	}

	fakeRegServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/Login/authenticate" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accessToken":  jwtString,
			"tokenType":    "Bearer",
			"role":         "User", // First role only; must not block TransitAdministrator in JWT
			"expiresAtUtc": now.Add(15 * time.Minute).Format(time.RFC3339),
		})
	}))
	defer fakeRegServer.Close()

	trust := config.JWTTrustConfig{
		Issuer:   "CmRegistrationService",
		Audience: "CmRegistrationClients",
		Secret:   []byte(routerTestSecret),
	}
	provider, err := auth.NewRegistrationProvider(config.RegistrationAuthConfig{ServiceURL: fakeRegServer.URL}, fakeRegServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	validator, err := auth.NewJWTValidator(trust, nil)
	if err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewInMemorySessionStore(nil, time.Hour, 100)
	defer sessions.Close()

	csrf := auth.NewCSRFProtector([]string{routerTestOrigin})
	throttler := auth.NewLoginThrottler(10, time.Minute, false, nil)
	authHandler := auth.NewHandler(provider, validator, sessions, csrf, throttler, "transit_session", false, nil)
	authMW := auth.NewMiddleware(sessions, validator, "transit_session", false)
	service := &fakeTransitRoutes{route: models.Route{ID: 10, Number: "10", Name: "Conestoga", Direction: "direction_0"}}
	dashboard := &fakeDashboard{summary: services.DashboardSummary{TotalRoutes: 2, ActiveRoutes: 2, InactiveRoutes: 0}}

	h := router.NewWithDependencies(nil, router.Dependencies{
		AuthHandler:    authHandler,
		AuthMiddleware: authMW,
		CSRF:           csrf,
		RequiredRole:   "TransitAdministrator",
		AdminHandler:   handlers.NewAdminHandler(dashboard),
	})
	// Also test RegisterTransitRoutes with the same authMW/csrf/role
	mux := chi.NewRouter()
	mux.Mount("/", h)
	router.RegisterTransitRoutes(mux, handlers.NewTransitRouteHandler(service), authMW, csrf, "TransitAdministrator")

	cookie, csrfToken := loginAs(t, mux, "city.transit.admin", "any-password")
	dashRec := authedRequest(mux, http.MethodGet, "/api/admin/dashboard", "", cookie, "")
	if dashRec.Code != http.StatusOK {
		t.Fatalf("admin dashboard via registration provider failed: %d %s", dashRec.Code, dashRec.Body.String())
	}
	createRec := authedRequest(mux, http.MethodPost, "/api/routes", `{"number":"10","name":"Conestoga","direction":"direction_0"}`, cookie, csrfToken)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("route create via registration provider failed: %d %s", createRec.Code, createRec.Body.String())
	}
}

func TestRouterWithoutAuthDependenciesFailsClosedOnWrites(t *testing.T) {
	h := router.New(nil)
	req := httptest.NewRequest(http.MethodPost, "/api/routes", strings.NewReader(`{"number":"7","name":"Main","direction":"direction_0"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected fail-closed 401 when auth deps omitted, got %d", rec.Code)
	}
}

func TestRejectInvalidRouteRequests(t *testing.T) {
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/api/routes/0", ""}, {"GET", "/api/routes/nope", ""}, {"GET", "/api/routes/2147483648", ""},
		{"POST", "/api/routes", "{"}, {"POST", "/api/routes", "null"}, {"POST", "/api/routes", "[]"},
		{"POST", "/api/routes", `{"id":42}`}, {"POST", "/api/routes", `{"number":7}`},
		{"PATCH", "/api/routes/42", `{"is_active":null}`},
		{"PATCH", "/api/routes/42", `{} {}`},
	} {
		service := &fakeTransitRoutes{}
		env := newTestAuthEnv(t, service, nil)
		cookie, csrfToken := loginAs(t, env.mux, "transit.admin", routerAdminPass)
		response := authedRequest(env.mux, tc.method, tc.path, tc.body, cookie, csrfToken)
		if response.Code != 400 || service.called != "" {
			t.Errorf("%s %s %s: status=%d call=%s", tc.method, tc.path, tc.body, response.Code, service.called)
		}
	}
}

func TestRouteErrorResponses(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{services.ErrTransitRouteNotFound, 404}, {services.ErrTransitRouteConflict, 409},
		{&services.ValidationError{Message: "invalid route"}, 400}, {errors.New("secret database details"), 500},
	} {
		env := newTestAuthEnv(t, &fakeTransitRoutes{err: tc.err}, nil)
		response := authedRequest(env.mux, http.MethodGet, "/api/routes/42", "", nil, "")
		if response.Code != tc.status {
			t.Fatalf("status=%d want=%d", response.Code, tc.status)
		}
		if response.Header().Get("Content-Type") != "application/json" {
			t.Fatal("expected JSON error")
		}
		if strings.Contains(response.Body.String(), "secret") {
			t.Fatal("internal error leaked")
		}
	}
}
