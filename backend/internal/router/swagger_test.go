package router_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/router"
)

func TestSwaggerDocumentation(t *testing.T) {
	mux := router.New(nil)
	for _, path := range []string{"/swagger/index.html", "/swagger/swagger-ui-bundle.js", "/swagger/doc.json"} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
		if recorder.Code != 200 {
			t.Fatalf("%s returned %d", path, recorder.Code)
		}
		if path != "/swagger/doc.json" {
			continue
		}
		var spec struct {
			Paths map[string]map[string]json.RawMessage `json:"paths"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &spec); err != nil {
			t.Fatal(err)
		}
		for route, methods := range map[string][]string{
			"/api/auth/login":                  {"post"},
			"/api/auth/me":                     {"get"},
			"/api/auth/logout":                 {"post"},
			"/api/admin/health":                {"get"},
			"/api/admin/dashboard":             {"get"},
			"/api/health":                      {"get"},
			"/api/routes":                      {"get", "post"},
			"/api/routes/{id}":                 {"get", "patch", "delete"},
			"/api/routes/{id}/schedule":        {"get"},
			"/api/departures":                  {"get"},
			"/api/stops/{stopID}/departures":   {"get"},
			"/api/routes/{routeID}/departures": {"get"},
			"/api/trips/{tripID}/departures":   {"get"},
			"/api/stops/{id}":                  {"get"},
		} {
			for _, method := range methods {
				if len(spec.Paths[route][method]) == 0 {
					t.Errorf("missing %s %s", method, route)
				}
			}
		}
	}
}
