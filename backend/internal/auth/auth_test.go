package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
	"github.com/golang-jwt/jwt/v5"
)

const (
	testIssuer        = "transit-dev"
	testAudience      = "transit-api"
	testSecret        = "0123456789abcdef0123456789abcdef"
	testOtherSecret   = "fedcba9876543210fedcba9876543210"
	testAdminPass     = "admin-secret-pw"
	testTrustedOrigin = "http://localhost:3000"
)

type fakeClock struct {
	current time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.current
}

func (c *fakeClock) Advance(d time.Duration) {
	c.current = c.current.Add(d)
}

func testTrust() config.JWTTrustConfig {
	return config.JWTTrustConfig{
		Issuer:   testIssuer,
		Audience: testAudience,
		Secret:   []byte(testSecret),
	}
}

func testMockCfg() config.MockAuthConfig {
	return config.MockAuthConfig{
		AdminPassword: testAdminPass,
	}
}

func signTestJWT(t *testing.T, method jwt.SigningMethod, key any, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(method, claims)
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func TestMockProviderAuthenticationAndJWTClaims(t *testing.T) {
	clock := &fakeClock{current: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	provider, err := NewMockProvider(testTrust(), testMockCfg(), clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := NewJWTValidator(testTrust(), clock.Now)
	if err != nil {
		t.Fatal(err)
	}

	res, err := provider.Authenticate(context.Background(), LoginRequest{
		Username:   "transit.admin",
		Password:   testAdminPass,
		RememberMe: true, // Must not extend 15-minute lifetime
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.TokenType != "Bearer" {
		t.Fatalf("TokenType = %q, want Bearer", res.TokenType)
	}
	wantExp := clock.Now().Add(15 * time.Minute)
	if !res.ExpiresAtUTC.Equal(wantExp) {
		t.Fatalf("ExpiresAtUTC = %v, want %v (rememberMe must not extend)", res.ExpiresAtUTC, wantExp)
	}

	principal, err := validator.Validate(res.AccessToken)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if principal.Subject != "dev-transit-admin-001" || principal.Username != "transit.admin" || !principal.HasRole("TransitAdministrator") {
		t.Fatalf("unexpected principal: %+v", principal)
	}

	// Wrong password and unknown user return ErrInvalidCredentials
	for _, req := range []LoginRequest{
		{Username: "transit.admin", Password: "wrong"},
		{Username: "unknown.user", Password: testAdminPass},
		{Username: "transit.user", Password: testAdminPass},
		{Username: "transit.admin", Password: ""},
	} {
		if _, err := provider.Authenticate(context.Background(), req); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("req=%q err=%v, want ErrInvalidCredentials", req.Username, err)
		}
	}
}

func TestJWTValidatorStrictValidation(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	clock := &fakeClock{current: now}
	validator, err := NewJWTValidator(testTrust(), clock.Now)
	if err != nil {
		t.Fatal(err)
	}

	baseClaims := func() jwt.MapClaims {
		return jwt.MapClaims{
			"sub":         "user-1",
			"unique_name": "alice",
			"role":        "TransitAdministrator",
			"iss":         testIssuer,
			"aud":         testAudience,
			"exp":         now.Add(15 * time.Minute).Unix(),
		}
	}

	t.Run("accepts string and array role claims", func(t *testing.T) {
		p1, err := validator.Validate(signTestJWT(t, jwt.SigningMethodHS256, []byte(testSecret), baseClaims()))
		if err != nil || len(p1.Roles) != 1 || p1.Roles[0] != "TransitAdministrator" {
			t.Fatalf("string role failed: %+v err=%v", p1, err)
		}

		c := baseClaims()
		c["role"] = []string{"User", "TransitAdministrator"}
		p2, err := validator.Validate(signTestJWT(t, jwt.SigningMethodHS256, []byte(testSecret), c))
		if err != nil || len(p2.Roles) != 2 || !p2.HasRole("TransitAdministrator") {
			t.Fatalf("array role failed: %+v err=%v", p2, err)
		}

		// Case-sensitive role matching
		if p2.HasRole("transitadministrator") {
			t.Fatal("role matching must be case-sensitive")
		}

		// Absent role treated as no privileges
		cNoRole := baseClaims()
		delete(cNoRole, "role")
		p3, err := validator.Validate(signTestJWT(t, jwt.SigningMethodHS256, []byte(testSecret), cNoRole))
		if err != nil || len(p3.Roles) != 0 {
			t.Fatalf("absent role failed: %+v err=%v", p3, err)
		}
	})

	t.Run("rejects expired JWT", func(t *testing.T) {
		c := baseClaims()
		c["exp"] = now.Add(-time.Second).Unix()
		if _, err := validator.Validate(signTestJWT(t, jwt.SigningMethodHS256, []byte(testSecret), c)); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("got %v, want ErrInvalidToken", err)
		}
	})

	t.Run("rejects wrong signature", func(t *testing.T) {
		tok := signTestJWT(t, jwt.SigningMethodHS256, []byte(testOtherSecret), baseClaims())
		if _, err := validator.Validate(tok); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("got %v, want ErrInvalidToken", err)
		}
	})

	t.Run("rejects wrong signing algorithm", func(t *testing.T) {
		tokHS384 := signTestJWT(t, jwt.SigningMethodHS384, []byte(testSecret), baseClaims())
		if _, err := validator.Validate(tokHS384); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("HS384 got %v, want ErrInvalidToken", err)
		}

		tokNone := signTestJWT(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, baseClaims())
		if _, err := validator.Validate(tokNone); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("none alg got %v, want ErrInvalidToken", err)
		}

		rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		tokRS256 := signTestJWT(t, jwt.SigningMethodRS256, rsaKey, baseClaims())
		if _, err := validator.Validate(tokRS256); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("RS256 got %v, want ErrInvalidToken", err)
		}
	})

	t.Run("rejects wrong issuer and audience", func(t *testing.T) {
		cIss := baseClaims()
		cIss["iss"] = "other-issuer"
		if _, err := validator.Validate(signTestJWT(t, jwt.SigningMethodHS256, []byte(testSecret), cIss)); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("wrong iss got %v", err)
		}

		cAud := baseClaims()
		cAud["aud"] = "other-aud"
		if _, err := validator.Validate(signTestJWT(t, jwt.SigningMethodHS256, []byte(testSecret), cAud)); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("wrong aud got %v", err)
		}
	})

	t.Run("rejects missing expiration and missing subject", func(t *testing.T) {
		cNoExp := baseClaims()
		delete(cNoExp, "exp")
		if _, err := validator.Validate(signTestJWT(t, jwt.SigningMethodHS256, []byte(testSecret), cNoExp)); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("missing exp got %v", err)
		}

		cNoSub := baseClaims()
		delete(cNoSub, "sub")
		if _, err := validator.Validate(signTestJWT(t, jwt.SigningMethodHS256, []byte(testSecret), cNoSub)); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("missing sub got %v", err)
		}
	})

	t.Run("rejects future nbf", func(t *testing.T) {
		c := baseClaims()
		c["nbf"] = now.Add(time.Minute).Unix()
		if _, err := validator.Validate(signTestJWT(t, jwt.SigningMethodHS256, []byte(testSecret), c)); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("future nbf got %v", err)
		}
	})

	t.Run("rejects tampered role payload", func(t *testing.T) {
		signed := signTestJWT(t, jwt.SigningMethodHS256, []byte(testSecret), baseClaims())
		parts := strings.Split(signed, ".")
		// Tamper with payload segment
		parts[1] = parts[1][:len(parts[1])-2] + "AA"
		tampered := strings.Join(parts, ".")
		if _, err := validator.Validate(tampered); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("tampered token got %v", err)
		}
	})

	t.Run("rejects malformed role claims", func(t *testing.T) {
		for _, badRole := range []any{123, true, nil, "", map[string]any{"name": "Admin"}, []any{"User", 42}, []any{""}} {
			c := baseClaims()
			c["role"] = badRole
			if _, err := validator.Validate(signTestJWT(t, jwt.SigningMethodHS256, []byte(testSecret), c)); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("badRole=%#v got %v, want ErrInvalidToken", badRole, err)
			}
		}
	})
}

func TestRegistrationProviderHTTPContracts(t *testing.T) {
	now := time.Now().UTC()
	trust := testTrust()
	validator, err := NewJWTValidator(trust, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	t.Run("authenticates and ignores top-level role metadata", func(t *testing.T) {
		signedJWT := signTestJWT(t, jwt.SigningMethodHS256, []byte(testSecret), jwt.MapClaims{
			"sub":         "reg-user-1",
			"unique_name": "jdoe",
			"role":        []string{"User", "TransitAdministrator"},
			"iss":         testIssuer,
			"aud":         testAudience,
			"exp":         now.Add(15 * time.Minute).Unix(),
		})
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/api/Login/authenticate" {
				t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessToken":  signedJWT,
				"tokenType":    "Bearer",
				"role":         "User", // Top-level role says User, signed JWT has TransitAdministrator
				"expiresAtUtc": now.Add(15 * time.Minute).Format(time.RFC3339),
			})
		}))
		defer server.Close()

		provider, err := NewRegistrationProvider(config.RegistrationAuthConfig{ServiceURL: server.URL, UpstreamIssuer: testIssuer, UpstreamAudience: testAudience, LocalTrust: trust, AdminUsers: []string{"jdoe"}, RequiredRole: "TransitAdministrator"}, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		res, err := provider.Authenticate(context.Background(), LoginRequest{Username: "jdoe", Password: "pw"})
		if err != nil {
			t.Fatal(err)
		}
		principal, err := validator.Validate(res.AccessToken)
		if err != nil {
			t.Fatal(err)
		}
		if !principal.HasRole("TransitAdministrator") {
			t.Fatalf("expected signed TransitAdministrator role, got %+v", principal)
		}
	})

	t.Run("maps 401 to ErrInvalidCredentials", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Account is temporarily locked."}`))
		}))
		defer server.Close()

		provider, _ := NewRegistrationProvider(config.RegistrationAuthConfig{ServiceURL: server.URL, UpstreamIssuer: testIssuer, UpstreamAudience: testAudience, LocalTrust: trust, AdminUsers: []string{"jdoe"}, RequiredRole: "TransitAdministrator"}, server.Client())
		_, err := provider.Authenticate(context.Background(), LoginRequest{Username: "a", Password: "b"})
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("got %v, want ErrInvalidCredentials", err)
		}
	})

	t.Run("handles 5xx, timeout, malformed JSON, and rejects redirects", func(t *testing.T) {
		// 503 upstream failure
		s503 := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		p503, _ := NewRegistrationProvider(config.RegistrationAuthConfig{ServiceURL: s503.URL, UpstreamIssuer: testIssuer, UpstreamAudience: testAudience, LocalTrust: trust, AdminUsers: []string{"jdoe"}, RequiredRole: "TransitAdministrator"}, s503.Client())
		_, err := p503.Authenticate(context.Background(), LoginRequest{Username: "a", Password: "b"})
		s503.Close()
		if !errors.Is(err, ErrProviderUnavailable) {
			t.Fatalf("503 got %v, want ErrProviderUnavailable", err)
		}

		// Timeout
		sSlow := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(150 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer sSlow.Close()
		slowClient := sSlow.Client()
		slowClient.Timeout = 30 * time.Millisecond
		pSlow, _ := NewRegistrationProvider(config.RegistrationAuthConfig{ServiceURL: sSlow.URL, UpstreamIssuer: testIssuer, UpstreamAudience: testAudience, LocalTrust: trust, AdminUsers: []string{"jdoe"}, RequiredRole: "TransitAdministrator"}, slowClient)
		_, err = pSlow.Authenticate(context.Background(), LoginRequest{Username: "a", Password: "b"})
		if !errors.Is(err, ErrProviderTimeout) {
			t.Fatalf("timeout got %v, want ErrProviderTimeout", err)
		}

		// Malformed response & wrong tokenType
		sMalformed := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"accessToken":"abc","tokenType":"Basic","expiresAtUtc":"2026-11-10T12:15:00Z"}`))
		}))
		defer sMalformed.Close()
		pMalformed, _ := NewRegistrationProvider(config.RegistrationAuthConfig{ServiceURL: sMalformed.URL, UpstreamIssuer: testIssuer, UpstreamAudience: testAudience, LocalTrust: trust, AdminUsers: []string{"jdoe"}, RequiredRole: "TransitAdministrator"}, sMalformed.Client())
		_, err = pMalformed.Authenticate(context.Background(), LoginRequest{Username: "a", Password: "b"})
		if !errors.Is(err, ErrInvalidUpstreamResponse) {
			t.Fatalf("wrong tokenType got %v, want ErrInvalidUpstreamResponse", err)
		}

		// Redirect rejected
		redirectHits := 0
		target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			redirectHits++
			w.WriteHeader(http.StatusOK)
		}))
		defer target.Close()
		redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
		}))
		defer redirector.Close()
		pRedirect, _ := NewRegistrationProvider(config.RegistrationAuthConfig{ServiceURL: redirector.URL, UpstreamIssuer: testIssuer, UpstreamAudience: testAudience, LocalTrust: trust, AdminUsers: []string{"jdoe"}, RequiredRole: "TransitAdministrator"}, redirector.Client())
		_, err = pRedirect.Authenticate(context.Background(), LoginRequest{Username: "a", Password: "b"})
		if !errors.Is(err, ErrInvalidUpstreamResponse) || redirectHits != 0 {
			t.Fatalf("redirect got %v hits=%d, want ErrInvalidUpstreamResponse and 0 redirect hits", err, redirectHits)
		}
	})
}

func TestSessionStoreExpiryRotationAndAuthHandlers(t *testing.T) {
	clock := &fakeClock{current: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	provider, err := NewMockProvider(testTrust(), testMockCfg(), clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := NewJWTValidator(testTrust(), clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	sessions := NewInMemorySessionStore(clock.Now, time.Hour, 100)
	defer sessions.Close()

	csrf := NewCSRFProtector([]string{testTrustedOrigin})
	throttler := NewLoginThrottler(3, time.Minute, false, clock.Now)
	handler := NewHandler(provider, validator, sessions, csrf, throttler, "transit_session", true, "TransitAdministrator", clock.Now)
	mw := NewMiddleware(sessions, validator, "transit_session", true)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/login", handler.Login)
	mux.Handle("GET /api/auth/me", mw.RequireAuthenticated(http.HandlerFunc(handler.Me)))
	mux.HandleFunc("POST /api/auth/logout", handler.Logout)
	mux.Handle("POST /api/admin/action", mw.RequireAuthenticated(mw.RequireRole("TransitAdministrator")(csrf.RequireMutationCSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))))

	// 1. Login succeeds for mock admin, sets HttpOnly/SameSite=Lax/Path=/api/Secure cookie, no JWT in body
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"transit.admin","password":"`+testAdminPass+`","rememberMe":false}`))
	loginReq.Header.Set("Origin", testTrustedOrigin)
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	mux.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginRec.Code, loginRec.Body.String())
	}
	if loginRec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing Cache-Control: no-store")
	}
	if strings.Contains(loginRec.Body.String(), "accessToken") || strings.Contains(loginRec.Body.String(), "eyJ") {
		t.Fatalf("JWT leaked in login response: %s", loginRec.Body.String())
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	c := cookies[0]
	if c.Name != "transit_session" || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/api" || c.Domain != "" || len(c.Value) < 32 {
		t.Fatalf("unexpected cookie attributes: %+v", c)
	}
	// Stored only as hash (raw ID must not appear as map key)
	sessions.mu.RLock()
	_, rawKeyExists := sessions.sessions[c.Value]
	sessions.mu.RUnlock()
	if rawKeyExists {
		t.Fatal("raw session ID must not be stored directly as key")
	}

	var loginBody SessionResponse
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginBody); err != nil {
		t.Fatal(err)
	}
	if loginBody.User.Username != "transit.admin" || loginBody.CSRFToken == "" {
		t.Fatalf("unexpected login body: %+v", loginBody)
	}

	// 2. Session rotation on second login invalidates first cookie
	loginReq2 := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"transit.admin","password":"`+testAdminPass+`"}`))
	loginReq2.Header.Set("Origin", testTrustedOrigin)
	loginReq2.Header.Set("Content-Type", "application/json")
	loginReq2.AddCookie(c)
	loginRec2 := httptest.NewRecorder()
	mux.ServeHTTP(loginRec2, loginReq2)
	if loginRec2.Code != http.StatusOK {
		t.Fatalf("second login failed: %d", loginRec2.Code)
	}
	c2 := loginRec2.Result().Cookies()[0]
	if c2.Value == c.Value {
		t.Fatal("expected session rotation on login")
	}
	var loginBody2 SessionResponse
	_ = json.Unmarshal(loginRec2.Body.Bytes(), &loginBody2)

	// Old session cookie is now rejected
	meOld := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meOld.AddCookie(c)
	meOldRec := httptest.NewRecorder()
	mux.ServeHTTP(meOldRec, meOld)
	if meOldRec.Code != http.StatusUnauthorized {
		t.Fatalf("old rotated cookie returned %d, want 401", meOldRec.Code)
	}

	// 3. CSRF protection on admin write
	writeNoCSRF := httptest.NewRequest(http.MethodPost, "/api/admin/action", nil)
	writeNoCSRF.Header.Set("Origin", testTrustedOrigin)
	writeNoCSRF.AddCookie(c2)
	writeNoCSRFRec := httptest.NewRecorder()
	mux.ServeHTTP(writeNoCSRFRec, writeNoCSRF)
	if writeNoCSRFRec.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF returned %d, want 403", writeNoCSRFRec.Code)
	}

	writeBadOrigin := httptest.NewRequest(http.MethodPost, "/api/admin/action", nil)
	writeBadOrigin.Header.Set("Origin", "https://evil.example")
	writeBadOrigin.Header.Set(CSRFHeaderName, loginBody2.CSRFToken)
	writeBadOrigin.AddCookie(c2)
	writeBadOriginRec := httptest.NewRecorder()
	mux.ServeHTTP(writeBadOriginRec, writeBadOrigin)
	if writeBadOriginRec.Code != http.StatusForbidden {
		t.Fatalf("untrusted origin returned %d, want 403", writeBadOriginRec.Code)
	}

	writeValid := httptest.NewRequest(http.MethodPost, "/api/admin/action", nil)
	writeValid.Header.Set("Origin", testTrustedOrigin)
	writeValid.Header.Set(CSRFHeaderName, loginBody2.CSRFToken)
	writeValid.AddCookie(c2)
	writeValidRec := httptest.NewRecorder()
	mux.ServeHTTP(writeValidRec, writeValid)
	if writeValidRec.Code != http.StatusNoContent {
		t.Fatalf("valid admin write returned %d, want 204", writeValidRec.Code)
	}

	// 4. Logout invalidates session and requires CSRF when session is valid
	logoutNoCSRF := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logoutNoCSRF.Header.Set("Origin", testTrustedOrigin)
	logoutNoCSRF.AddCookie(c2)
	logoutNoCSRFRec := httptest.NewRecorder()
	mux.ServeHTTP(logoutNoCSRFRec, logoutNoCSRF)
	if logoutNoCSRFRec.Code != http.StatusForbidden {
		t.Fatalf("logout without CSRF returned %d, want 403", logoutNoCSRFRec.Code)
	}

	logoutOK := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logoutOK.Header.Set("Origin", testTrustedOrigin)
	logoutOK.Header.Set(CSRFHeaderName, loginBody2.CSRFToken)
	logoutOK.AddCookie(c2)
	logoutOKRec := httptest.NewRecorder()
	mux.ServeHTTP(logoutOKRec, logoutOK)
	if logoutOKRec.Code != http.StatusOK {
		t.Fatalf("logout returned %d", logoutOKRec.Code)
	}
	cleared := logoutOKRec.Result().Cookies()[0]
	if cleared.MaxAge != -1 || cleared.Path != "/api" || !cleared.HttpOnly || !cleared.Secure || cleared.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cleared cookie attributes mismatch: %+v", cleared)
	}

	meAfterLogout := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meAfterLogout.AddCookie(c2)
	meAfterLogoutRec := httptest.NewRecorder()
	mux.ServeHTTP(meAfterLogoutRec, meAfterLogout)
	if meAfterLogoutRec.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout returned %d, want 401", meAfterLogoutRec.Code)
	}

	// 5. Session expiration never exceeds JWT expiration (15m)
	// First reset throttle window by advancing clock 2 minutes
	clock.Advance(2 * time.Minute)
	loginReq3 := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"transit.admin","password":"`+testAdminPass+`"}`))
	loginReq3.Header.Set("Origin", testTrustedOrigin)
	loginReq3.Header.Set("Content-Type", "application/json")
	loginRec3 := httptest.NewRecorder()
	mux.ServeHTTP(loginRec3, loginReq3)
	c3 := loginRec3.Result().Cookies()[0]

	clock.Advance(15 * time.Minute)
	meExpired := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meExpired.AddCookie(c3)
	meExpiredRec := httptest.NewRecorder()
	mux.ServeHTTP(meExpiredRec, meExpired)
	if meExpiredRec.Code != http.StatusUnauthorized {
		t.Fatalf("expired session returned %d, want 401", meExpiredRec.Code)
	}

	// 6. Login throttling returns 429 and ignores spoofed X-Forwarded-For when TrustedProxy=false
	for i := 0; i < 3; i++ {
		badReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"transit.admin","password":"wrong"}`))
		badReq.RemoteAddr = "198.51.100.10:1234"
		badReq.Header.Set("X-Forwarded-For", "203.0.113."+string(rune('1'+i)))
		badReq.Header.Set("Origin", testTrustedOrigin)
		badReq.Header.Set("Content-Type", "application/json")
		badRec := httptest.NewRecorder()
		mux.ServeHTTP(badRec, badReq)
		if badRec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d returned %d, want 401", i+1, badRec.Code)
		}
	}
	throttledReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"transit.admin","password":"wrong"}`))
	throttledReq.RemoteAddr = "198.51.100.10:5555"
	throttledReq.Header.Set("X-Forwarded-For", "203.0.113.99")
	throttledReq.Header.Set("Origin", testTrustedOrigin)
	throttledReq.Header.Set("Content-Type", "application/json")
	throttledRec := httptest.NewRecorder()
	mux.ServeHTTP(throttledRec, throttledReq)
	if throttledRec.Code != http.StatusTooManyRequests {
		t.Fatalf("4th attempt returned %d, want 429", throttledRec.Code)
	}
}

func TestSessionStoreBoundsExpiryToEarlierJWT(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	store := NewInMemorySessionStore(func() time.Time { return now }, time.Hour, 10)
	defer store.Close()

	jwtExp := now.Add(5 * time.Minute)
	laterProviderExp := now.Add(30 * time.Minute)

	rawID, session, err := store.Create(Principal{
		Subject:   "sub-1",
		Username:  "alice",
		Roles:     []string{"TransitAdministrator"},
		ExpiresAt: jwtExp,
	}, "jwt", laterProviderExp)
	if err != nil {
		t.Fatal(err)
	}
	if !session.ExpiresAt.Equal(jwtExp) {
		t.Fatalf("session.ExpiresAt = %v, want %v (capped at JWT exp)", session.ExpiresAt, jwtExp)
	}
	if _, ok := store.Get(rawID); !ok {
		t.Fatal("expected session to exist before expiry")
	}
}

func TestPrincipalHasAnyRole(t *testing.T) {
	p := Principal{Roles: []string{"Dispatcher"}}
	if !p.HasAnyRole([]string{"Administrator", "Dispatcher"}) {
		t.Fatal("expected HasAnyRole to match Dispatcher")
	}
	if p.HasAnyRole([]string{"dispatcher", "Administrator"}) {
		t.Fatal("expected HasAnyRole to enforce case-sensitive exact matching")
	}
	if p.HasAnyRole(nil) {
		t.Fatal("expected empty allowed slice to return false")
	}
}

func TestRegistrationProviderValidateBearer(t *testing.T) {
	now := time.Now().UTC()
	trust := testTrust()
	newTestProvider := func(url string, client *http.Client) *RegistrationProvider {
		t.Helper()
		p, err := NewRegistrationProvider(config.RegistrationAuthConfig{
			ServiceURL:       url,
			UpstreamIssuer:   testIssuer,
			UpstreamAudience: testAudience,
			LocalTrust:       trust,
			AdminUsers:       []string{"jdoe"},
			RequiredRole:     "TransitAdministrator",
		}, client)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("accepts short JWT claim names", func(t *testing.T) {
		var gotAuth string
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/api/Login/validate" {
				t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			}
			gotAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authenticated": true,
				"claims": []map[string]string{
					{"type": "sub", "value": "svc-emg-01"},
					{"type": "unique_name", "value": "emergency-service"},
					{"type": "role", "value": "Dispatcher"},
					{"type": "role", "value": "Dispatcher"},
					{"type": "iss", "value": testIssuer},
					{"type": "aud", "value": testAudience},
					{"type": "exp", "value": "1900000000"},
				},
			})
		}))
		defer server.Close()

		p := newTestProvider(server.URL, server.Client())
		principal, err := p.ValidateBearer(context.Background(), "valid.jwt.token")
		if err != nil {
			t.Fatalf("ValidateBearer failed: %v", err)
		}
		if gotAuth != "Bearer valid.jwt.token" {
			t.Fatalf("Authorization = %q, want Bearer valid.jwt.token", gotAuth)
		}
		if principal.Subject != "svc-emg-01" || principal.Username != "emergency-service" {
			t.Fatalf("unexpected principal identity: %+v", principal)
		}
		if len(principal.Roles) != 1 || principal.Roles[0] != "Dispatcher" {
			t.Fatalf("unexpected roles: %+v", principal.Roles)
		}
		if principal.ExpiresAt.Unix() != 1900000000 {
			t.Fatalf("ExpiresAt = %v, want unix 1900000000", principal.ExpiresAt)
		}
	})

	t.Run("accepts ASP.NET ClaimTypes URI claim names", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authenticated": true,
				"claims": []map[string]string{
					{"type": claimTypeNameIdentifier, "value": "svc-sec-01"},
					{"type": claimTypeName, "value": "security-service"},
					{"type": claimTypeRole, "value": "Security Administrator"},
				},
			})
		}))
		defer server.Close()

		p := newTestProvider(server.URL, server.Client())
		principal, err := p.ValidateBearer(context.Background(), "asp.net.jwt")
		if err != nil {
			t.Fatalf("ValidateBearer failed: %v", err)
		}
		if principal.Subject != "svc-sec-01" || principal.Username != "security-service" || !principal.HasRole("Security Administrator") {
			t.Fatalf("unexpected principal: %+v", principal)
		}
	})

	t.Run("rejects unauthenticated, 401/403, and wrong iss/aud with ErrInvalidToken", func(t *testing.T) {
		cases := []struct {
			name   string
			status int
			body   string
		}{
			{"authenticated false", http.StatusOK, `{"authenticated":false,"claims":[]}`},
			{"http 401", http.StatusUnauthorized, `{}`},
			{"http 403", http.StatusForbidden, `{}`},
			{"wrong iss", http.StatusOK, `{"authenticated":true,"claims":[{"type":"sub","value":"s1"},{"type":"iss","value":"evil-issuer"}]}`},
			{"wrong aud", http.StatusOK, `{"authenticated":true,"claims":[{"type":"sub","value":"s1"},{"type":"aud","value":"other-aud"}]}`},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				}))
				defer server.Close()

				p := newTestProvider(server.URL, server.Client())
				if _, err := p.ValidateBearer(context.Background(), "some.token"); !errors.Is(err, ErrInvalidToken) {
					t.Fatalf("%s: got err=%v, want ErrInvalidToken", tc.name, err)
				}
			})
		}
	})

	t.Run("maps redirect, malformed JSON, missing identity, 400, 503, and timeout", func(t *testing.T) {
		redirectHits := 0
		target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			redirectHits++
			w.WriteHeader(http.StatusOK)
		}))
		defer target.Close()
		redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
		}))
		defer redirector.Close()
		if _, err := newTestProvider(redirector.URL, redirector.Client()).ValidateBearer(context.Background(), "tok"); !errors.Is(err, ErrInvalidUpstreamResponse) || redirectHits != 0 {
			t.Fatalf("redirect got err=%v hits=%d, want ErrInvalidUpstreamResponse and 0 hits", err, redirectHits)
		}

		for _, tc := range []struct {
			name    string
			status  int
			body    string
			wantErr error
		}{
			{"malformed json", http.StatusOK, `{not-json`, ErrInvalidUpstreamResponse},
			{"missing identity", http.StatusOK, `{"authenticated":true,"claims":[{"type":"role","value":"Administrator"}]}`, ErrInvalidUpstreamResponse},
			{"http 400", http.StatusBadRequest, `{}`, ErrInvalidUpstreamResponse},
			{"http 503", http.StatusServiceUnavailable, `{}`, ErrProviderUnavailable},
		} {
			t.Run(tc.name, func(t *testing.T) {
				s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				}))
				defer s.Close()
				if _, err := newTestProvider(s.URL, s.Client()).ValidateBearer(context.Background(), "tok"); !errors.Is(err, tc.wantErr) {
					t.Fatalf("%s got err=%v, want %v", tc.name, err, tc.wantErr)
				}
			})
		}

		sSlow := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(120 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer sSlow.Close()
		slowClient := sSlow.Client()
		slowClient.Timeout = 25 * time.Millisecond
		if _, err := newTestProvider(sSlow.URL, slowClient).ValidateBearer(context.Background(), "tok"); !errors.Is(err, ErrProviderTimeout) {
			t.Fatalf("timeout got err=%v, want ErrProviderTimeout", err)
		}
	})

	t.Run("rejects token with whitespace or CRLF locally before HTTP call", func(t *testing.T) {
		called := false
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		p := newTestProvider(server.URL, server.Client())
		for _, bad := range []string{"", " ", "a b", "a\r\nX-Injected: 1", "a\tb", now.Format(time.RFC3339) + " extra"} {
			if _, err := p.ValidateBearer(context.Background(), bad); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("bad=%q got err=%v, want ErrInvalidToken", bad, err)
			}
		}
		if called {
			t.Fatal("expected dirty bearer tokens to be rejected locally without network call")
		}
	})
}

type stubBearerValidator struct {
	principal Principal
	err       error
	calls     int
}

func (s *stubBearerValidator) ValidateBearer(_ context.Context, _ string) (Principal, error) {
	s.calls++
	return s.principal, s.err
}

func TestMiddlewareRequireIntegrationAccess(t *testing.T) {
	clock := &fakeClock{current: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	validator, err := NewJWTValidator(testTrust(), clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	sessions := NewInMemorySessionStore(clock.Now, time.Hour, 10)
	defer sessions.Close()
	csrf := NewCSRFProtector([]string{testTrustedOrigin})
	mw := NewMiddleware(sessions, validator, "transit_session", false)

	signRole := func(sub string, roles any) string {
		claims := jwt.MapClaims{
			"sub": sub,
			"iss": testIssuer,
			"aud": testAudience,
			"exp": clock.Now().Add(15 * time.Minute).Unix(),
		}
		if roles != nil {
			claims["role"] = roles
		}
		return signTestJWT(t, jwt.SigningMethodHS256, []byte(testSecret), claims)
	}

	var gotPrincipal Principal
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFromContext(r.Context())
		if !ok {
			t.Fatal("expected principal in context")
		}
		gotPrincipal = p
		w.WriteHeader(http.StatusOK)
	})

	mutationHandler := mw.RequireIntegrationAccess(
		"TransitAdministrator",
		config.DefaultIntegrationRoles(),
		csrf,
		true,
	)(okHandler)

	readHandler := mw.RequireIntegrationAccess(
		"TransitAdministrator",
		config.DefaultIntegrationRoles(),
		csrf,
		false,
	)(okHandler)

	t.Run("accepts all default allowed integration roles without cookie, Origin, or CSRF", func(t *testing.T) {
		for _, role := range config.DefaultIntegrationRoles() {
			token := signRole("svc-"+role, role)
			req := httptest.NewRequest(http.MethodPost, "/api/transport/v1/integrations/emergency/incidents", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			mutationHandler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("role %q returned %d (%s), want 200", role, rec.Code, rec.Body.String())
			}
			if gotPrincipal.Subject != "svc-"+role || !gotPrincipal.HasRole(role) {
				t.Fatalf("unexpected principal for role %q: %+v", role, gotPrincipal)
			}
		}

		// Lowercase bearer scheme also accepted.
		req := httptest.NewRequest(http.MethodGet, "/api/transport/v1/integrations/service-status", nil)
		req.Header.Set("Authorization", "bearer "+signRole("svc-lower", "Dispatcher"))
		rec := httptest.NewRecorder()
		readHandler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("lowercase bearer returned %d, want 200", rec.Code)
		}
	})

	t.Run("rejects User, officer roles, and absent role claim with 403", func(t *testing.T) {
		for _, role := range []any{"User", "Police Officer", "Sergeant", "Police Chief", nil} {
			token := signRole("user-1", role)
			req := httptest.NewRequest(http.MethodPost, "/api/transport/v1/integrations/emergency/incidents", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			mutationHandler.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("disallowed role %v returned %d, want 403", role, rec.Code)
			}
		}
	})

	t.Run("rejects invalid, expired, wrong-secret, or malformed Authorization without cookie fallback", func(t *testing.T) {
		adminJWT := signRole("admin-1", "TransitAdministrator")
		adminPrincipal, err := validator.Validate(adminJWT)
		if err != nil {
			t.Fatal(err)
		}
		adminSID, adminSession, err := sessions.Create(adminPrincipal, adminJWT, clock.Now().Add(15*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		adminCookie := &http.Cookie{Name: "transit_session", Value: adminSID}

		expired := signTestJWT(t, jwt.SigningMethodHS256, []byte(testSecret), jwt.MapClaims{
			"sub": "svc", "role": "Administrator", "iss": testIssuer, "aud": testAudience,
			"exp": clock.Now().Add(-time.Minute).Unix(),
		})
		wrongSecret := signTestJWT(t, jwt.SigningMethodHS256, []byte(testOtherSecret), jwt.MapClaims{
			"sub": "svc", "role": "Administrator", "iss": testIssuer, "aud": testAudience,
			"exp": clock.Now().Add(15 * time.Minute).Unix(),
		})

		for _, header := range []string{
			"Bearer " + expired,
			"Bearer " + wrongSecret,
			"Basic dXNlcjpwYXNz",
			"Bearer",
			"Bearer two tokens",
		} {
			req := httptest.NewRequest(http.MethodPost, "/api/transport/v1/integrations/emergency/incidents", nil)
			req.Header.Set("Authorization", header)
			// Even with a valid admin session cookie + CSRF, invalid Authorization must return 401.
			req.Header.Set("Origin", testTrustedOrigin)
			req.Header.Set(CSRFHeaderName, adminSession.CSRFToken)
			req.AddCookie(adminCookie)
			rec := httptest.NewRecorder()
			mutationHandler.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("Authorization=%q returned %d, want 401 (must not fall back to cookie)", header, rec.Code)
			}
		}
	})

	t.Run("maps upstream BearerTokenValidator failures and empty allowedRoles fail-closed", func(t *testing.T) {
		for _, tc := range []struct {
			err  error
			want int
		}{
			{ErrProviderTimeout, http.StatusGatewayTimeout},
			{ErrProviderUnavailable, http.StatusServiceUnavailable},
			{ErrInvalidUpstreamResponse, http.StatusBadGateway},
			{ErrInvalidToken, http.StatusUnauthorized},
		} {
			stub := &stubBearerValidator{err: tc.err}
			stubMW := NewMiddlewareWithBearerValidator(sessions, validator, stub, "transit_session", false)
			h := stubMW.RequireIntegrationAccess("TransitAdministrator", []string{"Dispatcher"}, csrf, true)(okHandler)
			req := httptest.NewRequest(http.MethodPost, "/integrations", nil)
			req.Header.Set("Authorization", "Bearer external.jwt")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("err=%v returned %d, want %d", tc.err, rec.Code, tc.want)
			}
		}

		emptyAllowed := mw.RequireIntegrationAccess("TransitAdministrator", nil, csrf, true)(okHandler)
		req := httptest.NewRequest(http.MethodPost, "/integrations", nil)
		req.Header.Set("Authorization", "Bearer "+signRole("svc", "Administrator"))
		rec := httptest.NewRecorder()
		emptyAllowed.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("empty bearerAllowedRoles returned %d, want 403", rec.Code)
		}
	})

	t.Run("preserves cookie, role, and CSRF enforcement when Authorization header is absent", func(t *testing.T) {
		adminJWT := signRole("admin-2", "TransitAdministrator")
		adminP, _ := validator.Validate(adminJWT)
		adminSID, adminSess, _ := sessions.Create(adminP, adminJWT, clock.Now().Add(15*time.Minute))
		adminCookie := &http.Cookie{Name: "transit_session", Value: adminSID}

		userJWT := signRole("user-2", "User")
		userP, _ := validator.Validate(userJWT)
		userSID, userSess, _ := sessions.Create(userP, userJWT, clock.Now().Add(15*time.Minute))
		userCookie := &http.Cookie{Name: "transit_session", Value: userSID}

		// No auth -> 401
		rec := httptest.NewRecorder()
		mutationHandler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/integrations", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated returned %d, want 401", rec.Code)
		}

		// User cookie -> 403
		reqUser := httptest.NewRequest(http.MethodPost, "/integrations", nil)
		reqUser.Header.Set("Origin", testTrustedOrigin)
		reqUser.Header.Set(CSRFHeaderName, userSess.CSRFToken)
		reqUser.AddCookie(userCookie)
		recUser := httptest.NewRecorder()
		mutationHandler.ServeHTTP(recUser, reqUser)
		if recUser.Code != http.StatusForbidden {
			t.Fatalf("non-admin cookie returned %d, want 403", recUser.Code)
		}

		// Admin cookie missing CSRF -> 403 on mutation
		reqNoCSRF := httptest.NewRequest(http.MethodPost, "/integrations", nil)
		reqNoCSRF.Header.Set("Origin", testTrustedOrigin)
		reqNoCSRF.AddCookie(adminCookie)
		recNoCSRF := httptest.NewRecorder()
		mutationHandler.ServeHTTP(recNoCSRF, reqNoCSRF)
		if recNoCSRF.Code != http.StatusForbidden {
			t.Fatalf("missing CSRF returned %d, want 403", recNoCSRF.Code)
		}

		// Admin cookie with Origin + CSRF -> 200 on mutation
		reqOK := httptest.NewRequest(http.MethodPost, "/integrations", nil)
		reqOK.Header.Set("Origin", testTrustedOrigin)
		reqOK.Header.Set(CSRFHeaderName, adminSess.CSRFToken)
		reqOK.AddCookie(adminCookie)
		recOK := httptest.NewRecorder()
		mutationHandler.ServeHTTP(recOK, reqOK)
		if recOK.Code != http.StatusOK {
			t.Fatalf("valid admin cookie+CSRF returned %d, want 200", recOK.Code)
		}

		// Admin cookie without CSRF -> 200 on read
		reqReadOK := httptest.NewRequest(http.MethodGet, "/integrations/service-status", nil)
		reqReadOK.AddCookie(adminCookie)
		recReadOK := httptest.NewRecorder()
		readHandler.ServeHTTP(recReadOK, reqReadOK)
		if recReadOK.Code != http.StatusOK {
			t.Fatalf("valid admin cookie read returned %d, want 200", recReadOK.Code)
		}
	})
}
