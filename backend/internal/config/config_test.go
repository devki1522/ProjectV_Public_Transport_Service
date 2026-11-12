package config

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	testDSN           = "postgres://test:test@localhost:5432/test?sslmode=disable"
	testValidSecret   = "0123456789abcdef0123456789abcdef" // 32 bytes
	testShortSecret   = "too-short-secret"
	testRegSecret     = "fedcba9876543210fedcba9876543210"
	testAdminPassword = "dev-admin-password"
)

func clearAuthEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"PORT", "DB_MAX_IDLE_CONNS", "DB_MAX_OPEN_CONNS", "DB_CONN_MAX_LIFETIME",
		"APP_ENV", "AUTH_PROVIDER", "AUTH_REQUIRED_ROLE", "AUTH_SESSION_COOKIE",
		"AUTH_COOKIE_SECURE", "AUTH_TRUSTED_ORIGINS", "AUTH_TRUSTED_PROXY",
		"MOCK_JWT_ISSUER", "MOCK_JWT_AUDIENCE", "MOCK_JWT_SECRET",
		"MOCK_ADMIN_PASSWORD",
		"REGISTRATION_SERVICE_URL", "REGISTRATION_JWT_ISSUER",
		"REGISTRATION_JWT_AUDIENCE", "REGISTRATION_JWT_SECRET",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("DATABASE_URL", testDSN)
}

func setMockDevEnv(t *testing.T) {
	t.Helper()
	clearAuthEnv(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("MOCK_JWT_ISSUER", "transit-dev")
	t.Setenv("MOCK_JWT_AUDIENCE", "transit-api")
	t.Setenv("MOCK_JWT_SECRET", testValidSecret)
	t.Setenv("MOCK_ADMIN_PASSWORD", testAdminPassword)
}

func setRegistrationProdEnv(t *testing.T) {
	t.Helper()
	clearAuthEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("AUTH_TRUSTED_ORIGINS", "https://transit.example.com")
	t.Setenv("REGISTRATION_SERVICE_URL", "https://auth.example.com")
	t.Setenv("REGISTRATION_JWT_ISSUER", "CmRegistrationService")
	t.Setenv("REGISTRATION_JWT_AUDIENCE", "CmRegistrationClients")
	t.Setenv("REGISTRATION_JWT_SECRET", testRegSecret)
}

func TestLoadMockDevelopmentDefaults(t *testing.T) {
	setMockDevEnv(t)
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		Env:    EnvDevelopment,
		Server: ServerConfig{Port: 8080},
		Database: DatabaseConfig{
			DSN: testDSN, MaxIdleConns: 5, MaxOpenCons: 20, ConnMaxLifetime: time.Hour,
		},
		Auth: AuthConfig{
			Provider:       AuthProviderMock,
			RequiredRole:   "TransitAdministrator",
			SessionCookie:  "transit_session",
			CookieSecure:   false,
			TrustedOrigins: []string{"http://localhost:3000"},
			TrustedProxy:   false,
			JWTTrust: JWTTrustConfig{
				Issuer:   "transit-dev",
				Audience: "transit-api",
				Secret:   []byte(testValidSecret),
			},
			Mock: MockAuthConfig{
				AdminPassword: testAdminPassword,
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadProductionDefaultsToRegistration(t *testing.T) {
	setRegistrationProdEnv(t)
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Env != EnvProduction || got.Auth.Provider != AuthProviderRegistration {
		t.Fatalf("got env=%q provider=%q, want production/registration", got.Env, got.Auth.Provider)
	}
	if !got.Auth.CookieSecure {
		t.Fatal("expected CookieSecure=true in production")
	}
	if got.Auth.JWTTrust.Issuer != "CmRegistrationService" || string(got.Auth.JWTTrust.Secret) != testRegSecret {
		t.Fatalf("unexpected JWTTrust: %+v", got.Auth.JWTTrust)
	}
	if got.Auth.Mock != (MockAuthConfig{}) {
		t.Fatalf("expected empty mock config in production, got %+v", got.Auth.Mock)
	}
}

func TestLoadOverrides(t *testing.T) {
	setMockDevEnv(t)
	t.Setenv("PORT", "9090")
	t.Setenv("DATABASE_URL", "postgres://custom:custom@localhost/custom")
	t.Setenv("DB_MAX_IDLE_CONNS", "2")
	t.Setenv("DB_MAX_OPEN_CONNS", "10")
	t.Setenv("DB_CONN_MAX_LIFETIME", "30m")
	t.Setenv("AUTH_REQUIRED_ROLE", "CustomAdmin")
	t.Setenv("AUTH_SESSION_COOKIE", "custom_cookie")
	t.Setenv("AUTH_COOKIE_SECURE", "true")
	t.Setenv("AUTH_TRUSTED_ORIGINS", "https://LOCALHOST:3000/, http://127.0.0.1:3000")
	t.Setenv("AUTH_TRUSTED_PROXY", "true")
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Server.Port != 9090 || got.Database.MaxOpenCons != 10 || got.Database.ConnMaxLifetime != 30*time.Minute {
		t.Fatalf("unexpected server/db config: %+v", got)
	}
	if got.Auth.RequiredRole != "CustomAdmin" || got.Auth.SessionCookie != "custom_cookie" || !got.Auth.CookieSecure || !got.Auth.TrustedProxy {
		t.Fatalf("unexpected auth overrides: %+v", got.Auth)
	}
	wantOrigins := []string{"https://localhost:3000", "http://127.0.0.1:3000"}
	if !reflect.DeepEqual(got.Auth.TrustedOrigins, wantOrigins) {
		t.Fatalf("TrustedOrigins = %v, want %v", got.Auth.TrustedOrigins, wantOrigins)
	}
}

func TestLoadRejectsInvalidDatabaseAndServerConfiguration(t *testing.T) {
	tests := []struct {
		name, key, value, wantError string
	}{
		{"missing database URL", "DATABASE_URL", "", "DATABASE_URL is required"},
		{"nonnumeric port", "PORT", "abc", "PORT:"},
		{"negative port", "PORT", "-1", "PORT must be between"},
		{"zero port", "PORT", "0", "PORT must be between"},
		{"port above maximum", "PORT", "65536", "PORT must be between"},
		{"nonnumeric idle count", "DB_MAX_IDLE_CONNS", "abc", "DB_MAX_IDLE_CONNS:"},
		{"negative idle count", "DB_MAX_IDLE_CONNS", "-1", "invalid database pool"},
		{"idle exceeds open", "DB_MAX_IDLE_CONNS", "21", "invalid database pool"},
		{"nonnumeric open count", "DB_MAX_OPEN_CONNS", "abc", "DB_MAX_OPEN_CONNS:"},
		{"zero open count", "DB_MAX_OPEN_CONNS", "0", "invalid database pool"},
		{"negative open count", "DB_MAX_OPEN_CONNS", "-1", "invalid database pool"},
		{"invalid lifetime", "DB_CONN_MAX_LIFETIME", "abc", "DB_CONN_MAX_LIFETIME:"},
		{"zero lifetime", "DB_CONN_MAX_LIFETIME", "0s", "invalid database pool"},
		{"negative lifetime", "DB_CONN_MAX_LIFETIME", "-1h", "invalid database pool"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMockDevEnv(t)
			t.Setenv(tt.key, tt.value)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("Load() error = %v, want error containing %q", err, tt.wantError)
			}
		})
	}
}

func TestLoadRejectsAuthTrustBoundaryViolations(t *testing.T) {
	t.Run("default APP_ENV fails closed as production without registration settings", func(t *testing.T) {
		clearAuthEnv(t)
		if _, err := Load(); err == nil {
			t.Fatal("expected error when APP_ENV defaults to production without registration settings")
		}
	})

	t.Run("invalid APP_ENV", func(t *testing.T) {
		setMockDevEnv(t)
		t.Setenv("APP_ENV", "qa")
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "APP_ENV") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("unknown AUTH_PROVIDER", func(t *testing.T) {
		setMockDevEnv(t)
		t.Setenv("AUTH_PROVIDER", "oauth")
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "AUTH_PROVIDER") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("mock provider rejected in production and staging", func(t *testing.T) {
		for _, env := range []string{"production", "staging"} {
			setRegistrationProdEnv(t)
			t.Setenv("APP_ENV", env)
			t.Setenv("AUTH_PROVIDER", "mock")
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), "only allowed when APP_ENV=development") {
				t.Fatalf("env=%s got %v", env, err)
			}
		}
	})

	t.Run("mock settings rejected in production", func(t *testing.T) {
		for _, key := range []string{"MOCK_JWT_SECRET", "MOCK_JWT_ISSUER", "MOCK_JWT_AUDIENCE", "MOCK_ADMIN_PASSWORD"} {
			setRegistrationProdEnv(t)
			t.Setenv(key, "some-value")
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), "MOCK_*") {
				t.Fatalf("key=%s got %v", key, err)
			}
		}
	})

	t.Run("insecure cookie rejected in production", func(t *testing.T) {
		setRegistrationProdEnv(t)
		t.Setenv("AUTH_COOKIE_SECURE", "false")
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "AUTH_COOKIE_SECURE") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("http registration URL rejected in production", func(t *testing.T) {
		setRegistrationProdEnv(t)
		t.Setenv("REGISTRATION_SERVICE_URL", "http://auth.example.com")
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "https") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("http registration URL allowed for localhost in development", func(t *testing.T) {
		clearAuthEnv(t)
		t.Setenv("APP_ENV", "development")
		t.Setenv("AUTH_PROVIDER", "registration")
		t.Setenv("REGISTRATION_SERVICE_URL", "http://localhost:5000/")
		t.Setenv("REGISTRATION_JWT_ISSUER", "CmRegistrationService")
		t.Setenv("REGISTRATION_JWT_AUDIENCE", "CmRegistrationClients")
		t.Setenv("REGISTRATION_JWT_SECRET", testRegSecret)
		got, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if got.Auth.Registration.ServiceURL != "http://localhost:5000" {
			t.Fatalf("ServiceURL = %q", got.Auth.Registration.ServiceURL)
		}
	})

	t.Run("http non-loopback registration URL rejected in development", func(t *testing.T) {
		clearAuthEnv(t)
		t.Setenv("APP_ENV", "development")
		t.Setenv("AUTH_PROVIDER", "registration")
		t.Setenv("REGISTRATION_SERVICE_URL", "http://auth.internal:5000")
		t.Setenv("REGISTRATION_JWT_ISSUER", "CmRegistrationService")
		t.Setenv("REGISTRATION_JWT_AUDIENCE", "CmRegistrationClients")
		t.Setenv("REGISTRATION_JWT_SECRET", testRegSecret)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "https") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("short mock secret rejected", func(t *testing.T) {
		setMockDevEnv(t)
		t.Setenv("MOCK_JWT_SECRET", testShortSecret)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "32 bytes") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("short registration secret rejected", func(t *testing.T) {
		setRegistrationProdEnv(t)
		t.Setenv("REGISTRATION_JWT_SECRET", testShortSecret)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "32 bytes") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("missing mock administrator password rejected", func(t *testing.T) {
		for _, key := range []string{"MOCK_ADMIN_PASSWORD"} {
			setMockDevEnv(t)
			t.Setenv(key, "")
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("key=%s got %v", key, err)
			}
		}
	})

	t.Run("http origin rejected in production", func(t *testing.T) {
		setRegistrationProdEnv(t)
		t.Setenv("AUTH_TRUSTED_ORIGINS", "http://transit.example.com")
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "https") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("real provider ignores mock trust config in development", func(t *testing.T) {
		setMockDevEnv(t)
		t.Setenv("AUTH_PROVIDER", "registration")
		t.Setenv("REGISTRATION_SERVICE_URL", "https://auth.example.com")
		t.Setenv("REGISTRATION_JWT_ISSUER", "real-iss")
		t.Setenv("REGISTRATION_JWT_AUDIENCE", "real-aud")
		t.Setenv("REGISTRATION_JWT_SECRET", testRegSecret)
		got, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if got.Auth.JWTTrust.Issuer != "real-iss" || string(got.Auth.JWTTrust.Secret) != testRegSecret {
			t.Fatalf("unexpected JWTTrust: %+v", got.Auth.JWTTrust)
		}
	})
}

func TestSecretsAreRedactedWhenFormatted(t *testing.T) {
	setMockDevEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	formatted := fmt.Sprintf("%+v", cfg.Auth)
	for _, secret := range []string{testValidSecret, testAdminPassword} {
		if strings.Contains(formatted, secret) {
			t.Fatalf("secret %q leaked into formatted config: %s", secret, formatted)
		}
	}
}

func TestLoadAcceptsBoundaryValues(t *testing.T) {
	for _, port := range []string{"1", "65535"} {
		t.Run(port, func(t *testing.T) {
			setMockDevEnv(t)
			t.Setenv("PORT", port)
			t.Setenv("DB_MAX_IDLE_CONNS", "0")
			t.Setenv("DB_MAX_OPEN_CONNS", "1")
			if _, err := Load(); err != nil {
				t.Fatalf("valid boundary configuration rejected: %v", err)
			}
		})
	}
	t.Run("idle equals open", func(t *testing.T) {
		setMockDevEnv(t)
		t.Setenv("DB_MAX_IDLE_CONNS", "20")
		if _, err := Load(); err != nil {
			t.Fatalf("equal idle and open limits rejected: %v", err)
		}
	})
}
