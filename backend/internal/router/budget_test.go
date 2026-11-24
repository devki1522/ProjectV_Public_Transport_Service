package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/auth"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/budget"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/handlers"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/router"
	"github.com/go-chi/chi/v5"
)

type fakeBudgetService struct {
	allocResult   budget.Allocation
	allocErr      error
	expenseInput  budget.ExpenseInput
	expenseResult budget.ExpenseRecord
	expenseErr    error
	revenueInput  budget.RevenueInput
	revenueResult budget.RevenueRecord
	revenueErr    error
	fundingInput  budget.FundingRequestInput
	fundingResult budget.FundingRequestRecord
	fundingErr    error
}

func (f *fakeBudgetService) GetAllocation(_ context.Context) (budget.Allocation, error) {
	return f.allocResult, f.allocErr
}

func (f *fakeBudgetService) SubmitExpense(_ context.Context, input budget.ExpenseInput) (budget.ExpenseRecord, error) {
	f.expenseInput = input
	return f.expenseResult, f.expenseErr
}

func (f *fakeBudgetService) SubmitRevenue(_ context.Context, input budget.RevenueInput) (budget.RevenueRecord, error) {
	f.revenueInput = input
	return f.revenueResult, f.revenueErr
}

func (f *fakeBudgetService) SubmitFundingRequest(_ context.Context, input budget.FundingRequestInput) (budget.FundingRequestRecord, error) {
	f.fundingInput = input
	return f.fundingResult, f.fundingErr
}

func newTestBudgetEnv(t *testing.T, svc handlers.BudgetService) (*testAuthEnv, *fakeBudgetService) {
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
	handler := handlers.NewBudgetHandlerWithAudit(svc, auditLogger)

	mux := chi.NewRouter()
	mux.Route(router.APIBasePath(""), func(api chi.Router) {
		router.RegisterAuthRoutes(api, authHandler, authMW)
		router.RegisterBudgetRoutes(api, handler, authMW, csrf, "TransitAdministrator")
	})

	fakeSvc, _ := svc.(*fakeBudgetService)
	return &testAuthEnv{
		mux:       mux,
		sessions:  sessions,
		validator: validator,
		csrf:      csrf,
		auditBuf:  &auditBuf,
	}, fakeSvc
}

func TestBudgetEndpointsEnforceAuthCSRFAndContract(t *testing.T) {
	svc := &fakeBudgetService{
		allocResult: budget.Allocation{
			DepartmentCode: "TRANSIT",
			Allocated:      250000,
			Spent:          90000,
			Remaining:      160000,
			Currency:       "CAD",
		},
		expenseResult: budget.ExpenseRecord{
			ID:             "exp-1",
			DepartmentCode: "TRANSIT",
			Amount:         1000,
			Description:    "Fuel",
			Status:         "approved",
		},
		revenueResult: budget.RevenueRecord{
			ID:             "rev-1",
			Source:         "FARES",
			DepartmentCode: "TRANSIT",
			Amount:         3500,
			Description:    "Bus fares",
		},
		fundingResult: budget.FundingRequestRecord{
			ID:             "fund-1",
			DepartmentCode: "TRANSIT",
			Amount:         50000,
			Reason:         "Winter tires",
			Urgent:         true,
			Status:         "pending",
		},
	}
	env, fakeSvc := newTestBudgetEnv(t, svc)

	// Unauthenticated request is rejected.
	req := httptest.NewRequest(http.MethodGet, "/api/transport/v1/admin/budget", nil)
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauthenticated, got %d", rec.Code)
	}

	cookie, csrfToken := loginAs(t, env.mux, "transit.admin", routerAdminPass)

	// GET allocation succeeds for admin.
	req = httptest.NewRequest(http.MethodGet, "/api/transport/v1/admin/budget", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET /admin/budget, got %d (%s)", rec.Code, rec.Body.String())
	}
	var alloc budget.Allocation
	if err := json.Unmarshal(rec.Body.Bytes(), &alloc); err != nil {
		t.Fatalf("decode allocation: %v", err)
	}
	if alloc.DepartmentCode != "TRANSIT" || alloc.Remaining != 160000 {
		t.Fatalf("unexpected allocation: %+v", alloc)
	}

	// POST without CSRF token is rejected with 403.
	req = httptest.NewRequest(http.MethodPost, "/api/transport/v1/admin/budget/expenses", strings.NewReader(`{"amount":1000,"description":"Fuel"}`))
	req.AddCookie(cookie)
	req.Header.Set("Origin", routerTestOrigin)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without CSRF token, got %d", rec.Code)
	}

	// POST expense with CSRF succeeds.
	req = httptest.NewRequest(http.MethodPost, "/api/transport/v1/admin/budget/expenses", strings.NewReader(`{"amount":1000,"description":"Fuel"}`))
	req.AddCookie(cookie)
	req.Header.Set("Origin", routerTestOrigin)
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for expense, got %d (%s)", rec.Code, rec.Body.String())
	}
	if fakeSvc.expenseInput.Amount != 1000 || fakeSvc.expenseInput.Description != "Fuel" {
		t.Fatalf("unexpected expense input: %+v", fakeSvc.expenseInput)
	}

	// POST revenue with CSRF succeeds.
	req = httptest.NewRequest(http.MethodPost, "/api/transport/v1/admin/budget/revenue", strings.NewReader(`{"amount":3500,"description":"Bus fares"}`))
	req.AddCookie(cookie)
	req.Header.Set("Origin", routerTestOrigin)
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for revenue, got %d (%s)", rec.Code, rec.Body.String())
	}

	// POST funding request with CSRF succeeds.
	req = httptest.NewRequest(http.MethodPost, "/api/transport/v1/admin/budget/funding-requests", strings.NewReader(`{"amount":50000,"reason":"Winter tires","urgent":true}`))
	req.AddCookie(cookie)
	req.Header.Set("Origin", routerTestOrigin)
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for funding request, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Over-budget rejection propagates 400 error message.
	fakeSvc.expenseErr = &budget.RejectedError{
		StatusCode: http.StatusBadRequest,
		Message:    "Expense exceeds remaining transit budget",
		TraceID:    "trace-999",
	}
	req = httptest.NewRequest(http.MethodPost, "/api/transport/v1/admin/budget/expenses", strings.NewReader(`{"amount":999999,"description":"Fleet"}`))
	req.AddCookie(cookie)
	req.Header.Set("Origin", routerTestOrigin)
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Expense exceeds remaining transit budget") {
		t.Fatalf("expected 400 over-budget error, got %d (%s)", rec.Code, rec.Body.String())
	}
}
