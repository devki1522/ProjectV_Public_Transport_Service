/* Package config loads and validates environment configuration. */
package config

import (
	"crypto/rand"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	EnvDevelopment           = "development"
	EnvStaging               = "staging"
	EnvProduction            = "production"
	AuthProviderMock         = "mock"
	AuthProviderRegistration = "registration"
	MinJWTSecretBytes        = 32
)

type Config struct {
	Env      string
	Server   ServerConfig
	Database DatabaseConfig
	Auth     AuthConfig
	Vehicles VehiclesConfig
}
type VehiclesConfig struct {
	Simulate             bool
	SimulateVehicleCount int
	SimulateTickInterval time.Duration
}
type ServerConfig struct{ Port int }
type DatabaseConfig struct {
	DSN             string
	MaxIdleConns    int
	MaxOpenCons     int
	ConnMaxLifetime time.Duration
}
type JWTTrustConfig struct {
	Issuer   string
	Audience string
	Secret   []byte
}

func (c JWTTrustConfig) String() string {
	return fmt.Sprintf("JWTTrustConfig{Issuer:%q, Audience:%q, Secret:[REDACTED len=%d]}", c.Issuer, c.Audience, len(c.Secret))
}

type MockAuthConfig struct{ AdminPassword string }

func (c MockAuthConfig) String() string { return "MockAuthConfig{AdminPassword:[REDACTED]}" }

type RegistrationAuthConfig struct {
	ServiceURL string
	// DirectHTTPS is a development-only credential exchange, never a client token validator.
	DirectHTTPS      bool
	UpstreamIssuer   string
	UpstreamAudience string
	LocalTrust       JWTTrustConfig
	AdminUsers       []string
	RequiredRole     string
}
type AuthConfig struct {
	Provider       string
	RequiredRole   string
	SessionCookie  string
	CookieSecure   bool
	TrustedOrigins []string
	TrustedProxy   bool
	JWTTrust       JWTTrustConfig
	Mock           MockAuthConfig
	Registration   RegistrationAuthConfig
}

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
	vehiclesCfg, err := loadVehiclesConfig(env)
	if err != nil {
		return Config{}, err
	}
	return Config{Env: env, Server: ServerConfig{Port: port}, Database: DatabaseConfig{DSN: dsn, MaxIdleConns: maxIdle, MaxOpenCons: maxOpen, ConnMaxLifetime: lifetime}, Auth: authCfg, Vehicles: vehiclesCfg}, nil
}
func loadVehiclesConfig(env string) (VehiclesConfig, error) {
	simulate, err := parseBoolEnv("SIMULATE_VEHICLES", env == EnvDevelopment)
	if err != nil {
		return VehiclesConfig{}, err
	}
	count, err := strconv.Atoi(getEnv("SIMULATE_VEHICLES_COUNT", "24"))
	if err != nil {
		return VehiclesConfig{}, fmt.Errorf("SIMULATE_VEHICLES_COUNT: %w", err)
	}
	if count < 1 || count > 200 {
		return VehiclesConfig{}, fmt.Errorf("SIMULATE_VEHICLES_COUNT must be between 1 and 200")
	}
	interval, err := time.ParseDuration(getEnv("SIMULATE_TICK_INTERVAL", "3s"))
	if err != nil {
		return VehiclesConfig{}, fmt.Errorf("SIMULATE_TICK_INTERVAL: %w", err)
	}
	if interval < 500*time.Millisecond || interval > 60*time.Second {
		return VehiclesConfig{}, fmt.Errorf("SIMULATE_TICK_INTERVAL must be between 500ms and 60s")
	}
	return VehiclesConfig{Simulate: simulate, SimulateVehicleCount: count, SimulateTickInterval: interval}, nil
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
	cookieSecure, err := parseBoolEnv("AUTH_COOKIE_SECURE", env != EnvDevelopment)
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
	cfg := AuthConfig{Provider: provider, RequiredRole: requiredRole, SessionCookie: sessionCookie, CookieSecure: cookieSecure, TrustedOrigins: trustedOrigins, TrustedProxy: trustedProxy}
	direct, err := parseBoolEnv("REGISTRATION_TRUST_HTTPS", false)
	if err != nil {
		return AuthConfig{}, err
	}
	if direct && (env != EnvDevelopment || provider != AuthProviderRegistration) {
		return AuthConfig{}, fmt.Errorf("REGISTRATION_TRUST_HTTPS requires development and registration provider")
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
		cfg.JWTTrust = JWTTrustConfig{Issuer: issuer, Audience: audience, Secret: []byte(secret)}
		cfg.Mock = MockAuthConfig{AdminPassword: adminPassword}
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
		cfg.Registration = RegistrationAuthConfig{ServiceURL: serviceURL}
		if direct {
			if !strings.HasPrefix(serviceURL, "https://") {
				return AuthConfig{}, fmt.Errorf("REGISTRATION_TRUST_HTTPS requires HTTPS")
			}
			if secret != "" {
				return AuthConfig{}, fmt.Errorf("leave REGISTRATION_JWT_SECRET empty in direct HTTPS mode")
			}
			var users []string
			for _, raw := range strings.Split(os.Getenv("AUTH_ADMIN_WHITELIST"), ",") {
				if u := strings.TrimSpace(raw); u != "" {
					users = append(users, u)
				}
			}
			if len(users) == 0 {
				return AuthConfig{}, fmt.Errorf("AUTH_ADMIN_WHITELIST must contain at least one username")
			}
			key := make([]byte, MinJWTSecretBytes)
			if _, err := rand.Read(key); err != nil {
				return AuthConfig{}, fmt.Errorf("generate local JWT key: %w", err)
			}
			cfg.JWTTrust = JWTTrustConfig{Issuer: "public-transport-local", Audience: "public-transport-api", Secret: key}
			cfg.Registration = RegistrationAuthConfig{ServiceURL: serviceURL, DirectHTTPS: true, UpstreamIssuer: issuer, UpstreamAudience: audience, LocalTrust: cfg.JWTTrust, AdminUsers: users, RequiredRole: requiredRole}
		} else {
			if len(secret) < MinJWTSecretBytes {
				return AuthConfig{}, fmt.Errorf("REGISTRATION_JWT_SECRET must be at least %d bytes", MinJWTSecretBytes)
			}
			cfg.JWTTrust = JWTTrustConfig{Issuer: issuer, Audience: audience, Secret: []byte(secret)}
		}
	}
	return cfg, nil
}
func hasAnyMockSetting() bool {
	for _, key := range []string{"MOCK_JWT_ISSUER", "MOCK_JWT_AUDIENCE", "MOCK_JWT_SECRET", "MOCK_ADMIN_PASSWORD"} {
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
	switch strings.ToLower(parsed.Scheme) {
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
