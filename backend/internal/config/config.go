/*
Package config provides a standardized way to get config values from .env files,
while providing fallback values if the variable is not present.
*/
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	EnvDevelopment = "development"
	EnvStaging     = "staging"
	EnvProduction  = "production"

	AuthProviderMock         = "mock"
	AuthProviderRegistration = "registration"

	MinJWTSecretBytes = 32
)

// Config contains the environment, server, database, and authentication settings for the API.
type Config struct {
	Env      string         // Application environment: development, staging, or production.
	Server   ServerConfig   // HTTP server settings.
	Database DatabaseConfig // Database connection settings.
	Auth     AuthConfig     // Authentication, session, and CSRF settings.
}

// ServerConfig contains HTTP server settings.
type ServerConfig struct {
	Port int // TCP port on which the API listens.
}

// DatabaseConfig contains database connection pool settings.
type DatabaseConfig struct {
	DSN             string        // PostgreSQL connection string.
	MaxIdleConns    int           // Maximum number of idle connections.
	MaxOpenCons     int           // Maximum number of open connections.
	ConnMaxLifetime time.Duration // Maximum amount of time a connection may be reused.
}

// JWTTrustConfig holds the single active HS256 JWT validation configuration.
type JWTTrustConfig struct {
	Issuer   string // Expected JWT issuer (iss).
	Audience string // Expected JWT audience (aud).
	Secret   []byte // HS256 signing key (at least 32 bytes).
}

// String redacts the JWT signing secret when formatted.
func (c JWTTrustConfig) String() string {
	return fmt.Sprintf("JWTTrustConfig{Issuer:%q, Audience:%q, Secret:[REDACTED len=%d]}", c.Issuer, c.Audience, len(c.Secret))
}

// MockAuthConfig holds development-only mock user credentials.
type MockAuthConfig struct {
	AdminPassword string // Password for the mock TransitAdministrator user.
}

// String redacts mock passwords when formatted.
func (c MockAuthConfig) String() string {
	return "MockAuthConfig{AdminPassword:[REDACTED]}"
}

// RegistrationAuthConfig holds settings for the external Registration Service provider.
type RegistrationAuthConfig struct {
	ServiceURL string // Base URL of the Registration Service (HTTPS required outside local development).
}

// AuthConfig contains validated authentication, JWT trust, cookie, and CSRF settings.
type AuthConfig struct {
	Provider       string                 // Active auth provider: mock or registration.
	RequiredRole   string                 // Required role claim for administrator access.
	SessionCookie  string                 // Name of the opaque HTTP-only session cookie.
	CookieSecure   bool                   // Whether the session cookie sets the Secure attribute.
	TrustedOrigins []string               // Allowed browser origins for CSRF origin verification.
	TrustedProxy   bool                   // Whether forwarded client-IP headers are trusted for login throttling.
	JWTTrust       JWTTrustConfig         // Single active JWT validation trust configuration.
	Mock           MockAuthConfig         // Populated only when Provider is mock in development.
	Registration   RegistrationAuthConfig // Populated only when Provider is registration.
}

// Load reads and validates the API configuration from environment variables.
func Load() (Config, error) {
	port, err := strconv.Atoi(getEnv("PORT", "8080"))
	if err != nil {
		return Config{}, fmt.Errorf("PORT: %w", err)
	}

	maxIdle, err := strconv.Atoi(getEnv("DB_MAX_IDLE_CONNS", "5"))
	if err != nil {
		return Config{}, fmt.Errorf("DB_MAX_IDLE_CONNS: %w", err)
	}

	maxOpen, err := strconv.Atoi(getEnv("DB_MAX_OPEN_CONNS", "20"))
	if err != nil {
		return Config{}, fmt.Errorf("DB_MAX_OPEN_CONNS: %w", err)
	}

	lifetime, err := time.ParseDuration(getEnv("DB_CONN_MAX_LIFETIME", "1h"))
	if err != nil {
		return Config{}, fmt.Errorf("DB_CONN_MAX_LIFETIME: %w", err)
	}

	if port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("PORT must be between 1 and 65535")
	}

	if maxIdle < 0 || maxOpen <= 0 || maxIdle > maxOpen || lifetime <= 0 {
		return Config{}, fmt.Errorf("invalid database pool configuration")
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	env, err := loadAppEnv()
	if err != nil {
		return Config{}, err
	}

	authCfg, err := loadAuthConfig(env)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Env: env,
		Server: ServerConfig{
			Port: port,
		},
		Database: DatabaseConfig{
			DSN:             dsn,
			MaxIdleConns:    maxIdle,
			MaxOpenCons:     maxOpen,
			ConnMaxLifetime: lifetime,
		},
		Auth: authCfg,
	}, nil
}

func loadAppEnv() (string, error) {
	env := strings.ToLower(strings.TrimSpace(getEnv("APP_ENV", EnvProduction)))
	switch env {
	case EnvDevelopment, EnvStaging, EnvProduction:
		return env, nil
	default:
		return "", fmt.Errorf("APP_ENV %q is invalid; must be development, staging, or production", env)
	}
}

func loadAuthConfig(env string) (AuthConfig, error) {
	defaultProvider := AuthProviderRegistration
	if env == EnvDevelopment {
		defaultProvider = AuthProviderMock
	}
	provider := strings.ToLower(strings.TrimSpace(getEnv("AUTH_PROVIDER", defaultProvider)))
	switch provider {
	case AuthProviderMock, AuthProviderRegistration:
	default:
		return AuthConfig{}, fmt.Errorf("AUTH_PROVIDER %q is invalid; must be mock or registration", provider)
	}

	if provider == AuthProviderMock && env != EnvDevelopment {
		return AuthConfig{}, fmt.Errorf("AUTH_PROVIDER=mock is only allowed when APP_ENV=development")
	}

	if env != EnvDevelopment && hasAnyMockSetting() {
		return AuthConfig{}, fmt.Errorf("MOCK_* authentication settings are forbidden outside APP_ENV=development")
	}

	requiredRole := strings.TrimSpace(getEnv("AUTH_REQUIRED_ROLE", "TransitAdministrator"))
	if requiredRole == "" {
		return AuthConfig{}, fmt.Errorf("AUTH_REQUIRED_ROLE must not be empty")
	}

	sessionCookie := strings.TrimSpace(getEnv("AUTH_SESSION_COOKIE", "transit_session"))
	if sessionCookie == "" {
		return AuthConfig{}, fmt.Errorf("AUTH_SESSION_COOKIE must not be empty")
	}

	defaultSecure := env != EnvDevelopment
	cookieSecure, err := parseBoolEnv("AUTH_COOKIE_SECURE", defaultSecure)
	if err != nil {
		return AuthConfig{}, err
	}
	if env != EnvDevelopment && !cookieSecure {
		return AuthConfig{}, fmt.Errorf("AUTH_COOKIE_SECURE must be true outside development")
	}

	trustedProxy, err := parseBoolEnv("AUTH_TRUSTED_PROXY", false)
	if err != nil {
		return AuthConfig{}, err
	}

	rawOrigins := os.Getenv("AUTH_TRUSTED_ORIGINS")
	if strings.TrimSpace(rawOrigins) == "" && env == EnvDevelopment {
		rawOrigins = "http://localhost:3000"
	}
	trustedOrigins, err := parseTrustedOrigins(rawOrigins, env)
	if err != nil {
		return AuthConfig{}, err
	}

	cfg := AuthConfig{
		Provider:       provider,
		RequiredRole:   requiredRole,
		SessionCookie:  sessionCookie,
		CookieSecure:   cookieSecure,
		TrustedOrigins: trustedOrigins,
		TrustedProxy:   trustedProxy,
	}

	switch provider {
	case AuthProviderMock:
		issuer := strings.TrimSpace(os.Getenv("MOCK_JWT_ISSUER"))
		audience := strings.TrimSpace(os.Getenv("MOCK_JWT_AUDIENCE"))
		secret := os.Getenv("MOCK_JWT_SECRET")
		adminPassword := os.Getenv("MOCK_ADMIN_PASSWORD")

		if issuer == "" || audience == "" {
			return AuthConfig{}, fmt.Errorf("MOCK_JWT_ISSUER and MOCK_JWT_AUDIENCE are required when AUTH_PROVIDER=mock")
		}
		if len(secret) < MinJWTSecretBytes {
			return AuthConfig{}, fmt.Errorf("MOCK_JWT_SECRET must be at least %d bytes", MinJWTSecretBytes)
		}
		if adminPassword == "" {
			return AuthConfig{}, fmt.Errorf("MOCK_ADMIN_PASSWORD is required when AUTH_PROVIDER=mock")
		}

		cfg.JWTTrust = JWTTrustConfig{
			Issuer:   issuer,
			Audience: audience,
			Secret:   []byte(secret),
		}
		cfg.Mock = MockAuthConfig{
			AdminPassword: adminPassword,
		}

	case AuthProviderRegistration:
		serviceURL, err := parseRegistrationURL(os.Getenv("REGISTRATION_SERVICE_URL"), env)
		if err != nil {
			return AuthConfig{}, err
		}
		issuer := strings.TrimSpace(os.Getenv("REGISTRATION_JWT_ISSUER"))
		audience := strings.TrimSpace(os.Getenv("REGISTRATION_JWT_AUDIENCE"))
		secret := os.Getenv("REGISTRATION_JWT_SECRET")

		if issuer == "" || audience == "" {
			return AuthConfig{}, fmt.Errorf("REGISTRATION_JWT_ISSUER and REGISTRATION_JWT_AUDIENCE are required when AUTH_PROVIDER=registration")
		}
		if len(secret) < MinJWTSecretBytes {
			return AuthConfig{}, fmt.Errorf("REGISTRATION_JWT_SECRET must be at least %d bytes", MinJWTSecretBytes)
		}

		cfg.JWTTrust = JWTTrustConfig{
			Issuer:   issuer,
			Audience: audience,
			Secret:   []byte(secret),
		}
		cfg.Registration = RegistrationAuthConfig{
			ServiceURL: serviceURL,
		}
	}

	return cfg, nil
}

func hasAnyMockSetting() bool {
	for _, key := range []string{
		"MOCK_JWT_ISSUER",
		"MOCK_JWT_AUDIENCE",
		"MOCK_JWT_SECRET",
		"MOCK_ADMIN_PASSWORD",
	} {
		if os.Getenv(key) != "" {
			return true
		}
	}
	return false
}

func parseBoolEnv(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", key, err)
	}
	return value, nil
}

func parseTrustedOrigins(raw, env string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("AUTH_TRUSTED_ORIGINS is required")
	}
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}
		parsed, err := url.Parse(candidate)
		if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("AUTH_TRUSTED_ORIGINS contains invalid origin %q", candidate)
		}
		scheme := strings.ToLower(parsed.Scheme)
		if scheme != "http" && scheme != "https" {
			return nil, fmt.Errorf("AUTH_TRUSTED_ORIGINS origin %q must use http or https", candidate)
		}
		if env == EnvProduction && scheme != "https" {
			return nil, fmt.Errorf("AUTH_TRUSTED_ORIGINS origin %q must use https in production", candidate)
		}
		normalized := scheme + "://" + strings.ToLower(parsed.Host)
		if _, exists := seen[normalized]; !exists {
			seen[normalized] = struct{}{}
			origins = append(origins, normalized)
		}
	}
	if len(origins) == 0 {
		return nil, fmt.Errorf("AUTH_TRUSTED_ORIGINS must contain at least one valid origin")
	}
	return origins, nil
}

func parseRegistrationURL(raw, env string) (string, error) {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return "", fmt.Errorf("REGISTRATION_SERVICE_URL is required when AUTH_PROVIDER=registration")
	}
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("REGISTRATION_SERVICE_URL %q is invalid", candidate)
	}
	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "https":
	case "http":
		host := strings.ToLower(parsed.Hostname())
		isLoopback := host == "localhost" || host == "127.0.0.1" || host == "::1"
		if env != EnvDevelopment || !isLoopback {
			return "", fmt.Errorf("REGISTRATION_SERVICE_URL must use https outside explicit local development")
		}
	default:
		return "", fmt.Errorf("REGISTRATION_SERVICE_URL must use https (or http for local development)")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
