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
	testUserPass      = "user-secret-pw"
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
		UserPassword:  testUserPass,
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

	userRes, err := provider.Authenticate(context.Background(), LoginRequest{
		Username: "transit.user",
		Password: testUserPass,
	})
	if err != nil {
		t.Fatal(err)
	}
	userPrincipal, err := validator.Validate(userRes.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if userPrincipal.Subject != "dev-transit-user-001" || !userPrincipal.HasRole("User") || userPrincipal.HasRole("TransitAdministrator") {
		t.Fatalf("unexpected user principal: %+v", userPrincipal)
	}

	// Wrong password and unknown user return ErrInvalidCredentials
	for _, req := range []LoginRequest{
		{Username: "transit.admin", Password: "wrong"},
		{Username: "unknown.user", Password: testAdminPass},
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
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
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
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		provider, err := NewRegistrationProvider(config.RegistrationAuthConfig{ServiceURL: server.URL}, server.Client())
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
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Account is temporarily locked."}`))
		}))
		defer server.Close()

		provider, _ := NewRegistrationProvider(config.RegistrationAuthConfig{ServiceURL: server.URL}, server.Client())
		_, err := provider.Authenticate(context.Background(), LoginRequest{Username: "a", Password: "b"})
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("got %v, want ErrInvalidCredentials", err)
		}
	})

	t.Run("handles 5xx, timeout, malformed JSON, and rejects redirects", func(t *testing.T) {
		// 503 upstream failure
		s503 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		p503, _ := NewRegistrationProvider(config.RegistrationAuthConfig{ServiceURL: s503.URL}, s503.Client())
		_, err := p503.Authenticate(context.Background(), LoginRequest{Username: "a", Password: "b"})
		s503.Close()
		if !errors.Is(err, ErrProviderUnavailable) {
			t.Fatalf("503 got %v, want ErrProviderUnavailable", err)
		}

		// Timeout
		sSlow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(150 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer sSlow.Close()
		slowClient := &http.Client{Timeout: 30 * time.Millisecond}
		pSlow, _ := NewRegistrationProvider(config.RegistrationAuthConfig{ServiceURL: sSlow.URL}, slowClient)
		_, err = pSlow.Authenticate(context.Background(), LoginRequest{Username: "a", Password: "b"})
		if !errors.Is(err, ErrProviderTimeout) {
			t.Fatalf("timeout got %v, want ErrProviderTimeout", err)
		}

		// Malformed response & wrong tokenType
		sMalformed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"accessToken":"abc","tokenType":"Basic","expiresAtUtc":"2026-11-10T12:15:00Z"}`))
		}))
		defer sMalformed.Close()
		pMalformed, _ := NewRegistrationProvider(config.RegistrationAuthConfig{ServiceURL: sMalformed.URL}, sMalformed.Client())
		_, err = pMalformed.Authenticate(context.Background(), LoginRequest{Username: "a", Password: "b"})
		if !errors.Is(err, ErrInvalidUpstreamResponse) {
			t.Fatalf("wrong tokenType got %v, want ErrInvalidUpstreamResponse", err)
		}

		// Redirect rejected
		redirectHits := 0
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			redirectHits++
			w.WriteHeader(http.StatusOK)
		}))
		defer target.Close()
		redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
		}))
		defer redirector.Close()
		pRedirect, _ := NewRegistrationProvider(config.RegistrationAuthConfig{ServiceURL: redirector.URL}, redirector.Client())
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
	handler := NewHandler(provider, validator, sessions, csrf, throttler, "transit_session", true, clock.Now)
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
