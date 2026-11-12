package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
)

// TestHealth verifies health.
func TestHealth(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	recorder := httptest.NewRecorder()
	newRouter(nil).ServeHTTP(recorder, request)
	response := recorder.Result()
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if got := response.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	var body map[string]string
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(&body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if len(body) != 1 || body["status"] != "ok" {
		t.Errorf("body = %v, want status:ok", body)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Errorf("expected end of JSON response, got %v", err)
	}
}

func TestBuildRouterSelectsConfiguredProvider(t *testing.T) {
	mockCfg := config.Config{
		Env: config.EnvDevelopment,
		Auth: config.AuthConfig{
			Provider:       config.AuthProviderMock,
			RequiredRole:   "TransitAdministrator",
			SessionCookie:  "transit_session",
			TrustedOrigins: []string{"http://localhost:3000"},
			JWTTrust: config.JWTTrustConfig{
				Issuer:   "transit-dev",
				Audience: "transit-api",
				Secret:   []byte("0123456789abcdef0123456789abcdef"),
			},
			Mock: config.MockAuthConfig{
				AdminPassword: "admin-pw",
				UserPassword:  "user-pw",
			},
		},
	}

	h, cleanup, err := buildRouter(mockCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	// Public health remains reachable and admin health requires authentication.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/health", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/admin/health status = %d, want 401", rec.Code)
	}
}
