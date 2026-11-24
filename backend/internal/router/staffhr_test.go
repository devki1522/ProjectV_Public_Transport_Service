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
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/router"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/staffhr"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

type fakeStaffHRService struct {
	overviewDept   string
	overviewResult staffhr.StaffOverview
	overviewErr    error
	addInput       staffhr.AddEmployeeInput
	addResult      staffhr.StaffMember
	addErr         error
}

func (f *fakeStaffHRService) Overview(_ context.Context, dept string) (staffhr.StaffOverview, error) {
	f.overviewDept = dept
	return f.overviewResult, f.overviewErr
}

func (f *fakeStaffHRService) AddEmployee(_ context.Context, input staffhr.AddEmployeeInput) (staffhr.StaffMember, error) {
	f.addInput = input
	return f.addResult, f.addErr
}

func newTestStaffHREnv(t *testing.T, svc handlers.StaffHRService) (*testAuthEnv, *fakeStaffHRService) {
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
	handler := handlers.NewStaffHRHandlerWithAudit(svc, auditLogger)

	mux := chi.NewRouter()
	mux.Route(router.APIBasePath(""), func(api chi.Router) {
		router.RegisterAuthRoutes(api, authHandler, authMW)
		router.RegisterStaffHRRoutes(api, handler, authMW, csrf, "TransitAdministrator")
	})

	fakeSvc, _ := svc.(*fakeStaffHRService)
	return &testAuthEnv{
		mux:       mux,
		sessions:  sessions,
		validator: validator,
		csrf:      csrf,
		auditBuf:  &auditBuf,
	}, fakeSvc
}

func TestStaffHREndpointsEnforceAuthCSRFAndContract(t *testing.T) {
	svc := &fakeStaffHRService{
		overviewResult: staffhr.StaffOverview{
			DepartmentCode: "PTR",
			DepartmentName: "Public Transport",
			ActiveCount:    5,
			OnDutyCount:    2,
			VacanciesCount: 1,
			TotalEmployees: 6,
			CitywideActive: 22,
			Departments: []staffhr.DepartmentHeadcount{
				{Code: "PTR", Name: "Public Transport", Active: 5, OnDuty: 2, Vacancies: 1},
			},
			Employees: []staffhr.StaffMember{
				{
					StaffNumber:        "STF-0000013",
					DisplayName:        "Greta Pikeworth",
					FirstName:          "Greta",
					LastName:           "Pikeworth",
					Email:              "greta.pikeworth@cmes.example",
					EmploymentStatus:   "Active",
					EmploymentType:     "FullTime",
					HomeDepartmentCode: "PTR",
					HomeDepartmentName: "Public Transport",
				},
			},
		},
		addResult: staffhr.StaffMember{
			StaffNumber:        "STF-0000099",
			DisplayName:        "Ada Lovelace",
			FirstName:          "Ada",
			LastName:           "Lovelace",
			Email:              "ada@cmes.example",
			EmploymentStatus:   "Active",
			EmploymentType:     "FullTime",
			HomeDepartmentCode: "PTR",
			HomeDepartmentName: "Public Transport",
		},
	}
	env, _ := newTestStaffHREnv(t, svc)

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

	body := `{"first_name":"Ada","last_name":"Lovelace","email":"ada@cmes.example","date_of_birth":"1990-05-01","hire_date":"2026-11-19","employment_type":"FullTime"}`

	// Fail closed when router constructed without auth dependencies.
	closedRouter := router.New(nil)
	closedRec := httptest.NewRecorder()
	closedRouter.ServeHTTP(closedRec, httptest.NewRequest(http.MethodGet, "/api/transport/v1/admin/staff", nil))
	if closedRec.Code != http.StatusUnauthorized {
		t.Fatalf("fail-closed status = %d, want 401", closedRec.Code)
	}

	// Unauthenticated -> 401
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/transport/v1/admin/staff"},
		{http.MethodPost, "/api/transport/v1/admin/staff/employees"},
		{http.MethodPut, "/api/transport/v1/admin/staff/employees"},
	} {
		rec := authedRequest(env.mux, tc.method, tc.path, body, nil, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s unauthenticated = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}

	// Non-admin -> 403
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/transport/v1/admin/staff"},
		{http.MethodPost, "/api/transport/v1/admin/staff/employees"},
		{http.MethodPut, "/api/transport/v1/admin/staff/employees"},
	} {
		rec := authedRequest(env.mux, tc.method, tc.path, body, userCookie, userCSRF)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s non-admin = %d, want 403", tc.method, tc.path, rec.Code)
		}
	}

	// Missing CSRF on POST and PUT -> 403
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		rec := authedRequest(env.mux, method, "/api/transport/v1/admin/staff/employees", body, adminCookie, "")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s missing CSRF = %d, want 403", method, rec.Code)
		}
	}

	// Admin GET returns StaffOverview and respects ?department= filter.
	getRec := authedRequest(env.mux, http.MethodGet, "/api/transport/v1/admin/staff?department=PTR", "", adminCookie, "")
	if getRec.Code != http.StatusOK || svc.overviewDept != "PTR" {
		t.Fatalf("GET /admin/staff status=%d dept=%q body=%s", getRec.Code, svc.overviewDept, getRec.Body.String())
	}
	var gotOverview staffhr.StaffOverview
	if err := json.Unmarshal(getRec.Body.Bytes(), &gotOverview); err != nil {
		t.Fatal(err)
	}
	if gotOverview.ActiveCount != 5 || len(gotOverview.Employees) != 1 {
		t.Fatalf("unexpected overview payload: %+v", gotOverview)
	}

	// Reject unknown query param on GET.
	badGetRec := authedRequest(env.mux, http.MethodGet, "/api/transport/v1/admin/staff?bogus=1", "", adminCookie, "")
	if badGetRec.Code != http.StatusBadRequest {
		t.Fatalf("GET unknown query = %d, want 400", badGetRec.Code)
	}

	// Admin POST and PUT both onboard employee and emit structured audit logs.
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		rec := authedRequest(env.mux, method, "/api/transport/v1/admin/staff/employees", body, adminCookie, adminCSRF)
		if rec.Code != http.StatusCreated {
			t.Fatalf("%s /admin/staff/employees status=%d body=%s", method, rec.Code, rec.Body.String())
		}
		var member staffhr.StaffMember
		if err := json.Unmarshal(rec.Body.Bytes(), &member); err != nil {
			t.Fatal(err)
		}
		if member.StaffNumber != "STF-0000099" {
			t.Fatalf("unexpected created member: %+v", member)
		}
	}
	if !strings.Contains(env.auditBuf.String(), `"resource_type":"staff_employee"`) || !strings.Contains(env.auditBuf.String(), `"staff_number":"STF-0000099"`) {
		t.Fatalf("expected structured audit log, got %s", env.auditBuf.String())
	}

	// Error mappings: 400, 404, 409, 503, 504, and sanitized 500.
	for _, tc := range []struct {
		err    error
		status int
	}{
		{&staffhr.InvalidInput{Message: "invalid email"}, 400},
		{staffhr.ErrNotFound, 404},
		{&staffhr.ConflictError{Message: "Work email already exists."}, 409},
		{staffhr.ErrNotConfigured, 503},
		{staffhr.ErrUpstreamUnavailable, 503},
		{staffhr.ErrUpstreamTimeout, 504},
		{errors.New("secret upstream crash details"), 500},
	} {
		svc.addErr = tc.err
		errRec := authedRequest(env.mux, http.MethodPost, "/api/transport/v1/admin/staff/employees", body, adminCookie, adminCSRF)
		if errRec.Code != tc.status {
			t.Fatalf("err=%v status=%d, want %d", tc.err, errRec.Code, tc.status)
		}
		if strings.Contains(errRec.Body.String(), "secret") {
			t.Fatalf("internal error leaked: %s", errRec.Body.String())
		}
	}
}
