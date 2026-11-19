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
			"/api/transport/v1/auth/login":                           {"post"},
			"/api/transport/v1/auth/me":                              {"get"},
			"/api/transport/v1/auth/logout":                          {"post"},
			"/api/transport/v1/admin/health":                         {"get"},
			"/api/transport/v1/admin/dashboard":                      {"get"},
			"/api/transport/v1/health":                               {"get"},
			"/api/transport/v1/routes":                               {"get", "post"},
			"/api/transport/v1/routes/{id}":                          {"get", "patch", "delete"},
			"/api/transport/v1/routes/{id}/schedule":                 {"get"},
			"/api/transport/v1/departures":                           {"get"},
			"/api/transport/v1/stops/{stopID}/departures":            {"get"},
			"/api/transport/v1/routes/{routeID}/departures":          {"get"},
			"/api/transport/v1/trips/{tripID}/departures":            {"get"},
			"/api/transport/v1/stops":                                {"get"},
			"/api/transport/v1/stops/{id}":                           {"get"},
			"/api/transport/v1/journeys":                             {"get"},
			"/api/transport/v1/vehicles":                             {"get"},
			"/api/transport/v1/vehicles/locations":                   {"post"},
			"/api/transport/v1/vehicles/simulator":                   {"get"},
			"/api/transport/v1/routes/{id}/vehicles":                 {"get"},
			"/api/transport/v1/trips/{id}/vehicles":                  {"get"},
			"/api/transport/v1/trips/{id}/arrivals":                  {"get"},
			"/api/transport/v1/stops/{id}/arrivals":                  {"get"},
			"/api/transport/v1/routes/{id}/shape":                    {"get"},
			"/api/transport/v1/admin/vehicles/simulator/start":       {"post"},
			"/api/transport/v1/admin/vehicles/simulator/stop":        {"post"},
			"/api/transport/v1/admin/vehicles/simulator/step":        {"post"},
			"/api/transport/v1/admin/vehicles/simulator/reset":       {"post"},
			"/api/transport/v1/disruptions":                          {"get", "post"},
			"/api/transport/v1/disruptions/active":                   {"get"},
			"/api/transport/v1/disruptions/affected":                 {"get"},
			"/api/transport/v1/disruptions/{id}":                     {"get", "patch"},
			"/api/transport/v1/road-closures":                        {"get", "post"},
			"/api/transport/v1/service-status":                       {"get"},
			"/api/transport/v1/routes/{id}/status":                   {"get"},
			"/api/transport/v1/stops/{id}/status":                    {"get"},
			"/api/transport/v1/trips/{id}/status":                    {"get"},
			"/api/transport/v1/integrations/utilities/notices":       {"post"},
			"/api/transport/v1/integrations/emergency/incidents":     {"post"},
			"/api/transport/v1/integrations/security/traffic-alerts": {"post"},
			"/api/transport/v1/integrations/service-status":          {"get"},
		} {
			for _, method := range methods {
				if len(spec.Paths[route][method]) == 0 {
					t.Errorf("missing %s %s", method, route)
				}
			}
		}
	}
}
