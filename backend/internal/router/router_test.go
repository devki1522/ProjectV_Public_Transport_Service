package router_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/handlers"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/models"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/router"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/services"
	"github.com/go-chi/chi/v5"
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

// List returns the transit routes available to clients.
func (f *fakeTransitRoutes) List(context.Context) ([]models.Route, error) {
	f.called = "list"
	return nil, f.err
}

// Get returns one transit route by identifier.
func (f *fakeTransitRoutes) Get(_ context.Context, id int) (models.Route, error) {
	f.called, f.id = "get", id
	return f.route, f.err
}

// Create validates and creates a transit route.
func (f *fakeTransitRoutes) Create(_ context.Context, input services.CreateTransitRouteInput) (models.Route, error) {
	f.called, f.create = "create", input
	return f.route, f.err
}

// Update validates and updates a transit route.
func (f *fakeTransitRoutes) Update(_ context.Context, id int, input services.UpdateTransitRouteInput) (models.Route, error) {
	f.called, f.id, f.update = "update", id, input
	return f.route, f.err
}

// Deactivate records a fake route deactivation.
func (f *fakeTransitRoutes) Deactivate(_ context.Context, id int) error {
	f.called, f.id = "deactivate", id
	return f.err
}

func request(service *fakeTransitRoutes, method, path, body string) *httptest.ResponseRecorder {
	mux := chi.NewRouter()
	router.RegisterTransitRoutes(mux, handlers.NewTransitRouteHandler(service))
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(method, path, strings.NewReader(body)))
	return recorder
}

// TestRouteEndpoints verifies route endpoints.
func TestRouteEndpoints(t *testing.T) {
	for _, tc := range []struct {
		method, path, body, call string
		status                   int
	}{
		{"GET", "/api/routes", "", "list", 200},
		{"GET", "/api/routes/42", "", "get", 200},
		{"POST", "/api/routes", `{"number":"7","name":"Main","direction":"direction_0"}`, "create", 201},
		{"PATCH", "/api/routes/42", `{"is_active":false}`, "update", 200},
		{"DELETE", "/api/routes/42", "", "deactivate", 204},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			service := &fakeTransitRoutes{route: models.Route{ID: 42, Number: "7", Name: "Main", Direction: "direction_0"}}
			response := request(service, tc.method, tc.path, tc.body)
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
		})
	}
}

// TestRejectInvalidRouteRequests verifies reject invalid route requests.
func TestRejectInvalidRouteRequests(t *testing.T) {
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/api/routes/0", ""}, {"GET", "/api/routes/nope", ""}, {"GET", "/api/routes/2147483648", ""},
		{"POST", "/api/routes", "{"}, {"POST", "/api/routes", "null"}, {"POST", "/api/routes", "[]"},
		{"POST", "/api/routes", `{"id":42}`}, {"POST", "/api/routes", `{"number":7}`},
		{"PATCH", "/api/routes/42", `{"is_active":null}`},
		{"PATCH", "/api/routes/42", `{} {}`},
	} {
		service := &fakeTransitRoutes{}
		response := request(service, tc.method, tc.path, tc.body)
		if response.Code != 400 || service.called != "" {
			t.Errorf("%s %s %s: status=%d call=%s", tc.method, tc.path, tc.body, response.Code, service.called)
		}
	}
}

// TestRouteErrorResponses verifies route error responses.
func TestRouteErrorResponses(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{services.ErrTransitRouteNotFound, 404}, {services.ErrTransitRouteConflict, 409},
		{&services.ValidationError{Message: "invalid route"}, 400}, {errors.New("secret database details"), 500},
	} {
		response := request(&fakeTransitRoutes{err: tc.err}, http.MethodGet, "/api/routes/42", "")
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
