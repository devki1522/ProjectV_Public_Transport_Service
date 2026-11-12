package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type providerFunc func(context.Context, LoginRequest) (AuthResult, error)

func (f providerFunc) Authenticate(ctx context.Context, request LoginRequest) (AuthResult, error) {
	return f(ctx, request)
}

func TestSessionCapabilityUsesConfiguredRole(t *testing.T) {
	for _, role := range []string{"TransportOperator", "User", "transportoperator"} {
		t.Run(role, func(t *testing.T) {
			now := time.Now().UTC()
			exp := now.Add(15 * time.Minute)
			validator, err := NewJWTValidator(testTrust(), nil)
			if err != nil {
				t.Fatal(err)
			}
			token := signTestJWT(t, jwt.SigningMethodHS256, []byte(testSecret), jwt.MapClaims{
				"sub": "external-user", "role": role, "iss": testIssuer, "aud": testAudience, "exp": exp.Unix(),
			})
			provider := providerFunc(func(context.Context, LoginRequest) (AuthResult, error) {
				return AuthResult{AccessToken: token, TokenType: "Bearer", ExpiresAtUTC: exp}, nil
			})
			store := NewInMemorySessionStore(nil, time.Hour, 10)
			t.Cleanup(store.Close)
			csrf := NewCSRFProtector([]string{testTrustedOrigin})
			h := NewHandler(provider, validator, store, csrf, nil, "session", false, "TransportOperator", nil)
			middleware := NewMiddleware(store, validator, "session", false)
			request := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"username":"external-user","password":"password"}`))
			request.Header.Set("Origin", testTrustedOrigin)
			request.Header.Set("Content-Type", "application/json")
			login := httptest.NewRecorder()
			h.Login(login, request)
			if login.Code != 200 {
				t.Fatalf("login returned %d", login.Code)
			}
			meRequest := httptest.NewRequest("GET", "/api/auth/me", nil)
			meRequest.AddCookie(login.Result().Cookies()[0])
			me := httptest.NewRecorder()
			middleware.RequireAuthenticated(http.HandlerFunc(h.Me)).ServeHTTP(me, meRequest)
			for _, response := range []*httptest.ResponseRecorder{login, me} {
				var payload SessionResponse
				if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if response.Code != 200 || payload.IsAdmin != (role == "TransportOperator") {
					t.Fatalf("role=%s status=%d capability=%t", role, response.Code, payload.IsAdmin)
				}
			}
		})
	}
}

func TestInvalidLoginNeverCreatesSession(t *testing.T) {
	validator, _ := NewJWTValidator(testTrust(), nil)
	store := NewInMemorySessionStore(nil, time.Hour, 10)
	defer store.Close()
	provider := providerFunc(func(context.Context, LoginRequest) (AuthResult, error) {
		return AuthResult{}, ErrInvalidCredentials
	})
	h := NewHandler(provider, validator, store, NewCSRFProtector([]string{testTrustedOrigin}), nil, "session", false, "TransitAdministrator", nil)
	request := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"username":"unknown","password":"wrong"}`))
	request.Header.Set("Origin", testTrustedOrigin)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	h.Login(response, request)
	if response.Code != 401 || len(response.Result().Cookies()) != 0 || len(store.sessions) != 0 {
		t.Fatalf("invalid login created a session or returned %d", response.Code)
	}
}
