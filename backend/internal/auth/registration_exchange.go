package auth

import (
	"fmt"
	"strings"
	"time"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
	"github.com/golang-jwt/jwt/v5"
)

const directSessionLifetime = 15 * time.Minute

// directRegistrationExchange is used only by the server-side HTTPS provider.
// It must never be called with a token received from a browser or API caller.
type directRegistrationExchange struct {
	issuer   string
	audience string
	local    config.JWTTrustConfig
	admins   map[string]struct{}
	role     string
}

func newDirectRegistrationExchange(cfg config.RegistrationAuthConfig) (*directRegistrationExchange, error) {
	if cfg.UpstreamIssuer == "" || cfg.UpstreamAudience == "" || cfg.LocalTrust.Issuer == "" || cfg.LocalTrust.Audience == "" || len(cfg.LocalTrust.Secret) < config.MinJWTSecretBytes || cfg.RequiredRole == "" {
		return nil, fmt.Errorf("invalid direct registration exchange configuration")
	}
	admins := make(map[string]struct{})
	for _, raw := range cfg.AdminUsers {
		if u := strings.TrimSpace(raw); u != "" {
			admins[u] = struct{}{}
		}
	}
	if len(admins) == 0 {
		return nil, fmt.Errorf("empty administrator whitelist")
	}
	local := cfg.LocalTrust
	local.Secret = append([]byte(nil), local.Secret...)
	return &directRegistrationExchange{issuer: cfg.UpstreamIssuer, audience: cfg.UpstreamAudience, local: local, admins: admins, role: cfg.RequiredRole}, nil
}

func (x *directRegistrationExchange) exchange(upstreamToken, submittedUsername string, upstreamExpiry, now time.Time) (AuthResult, error) {
	// Identity is trusted because it came straight from the configured HTTPS login endpoint.
	// ParseUnverified does NOT validate the external signature. Validate the claim contract
	// separately, then issue a different token signed with our own process-local secret.
	claims := jwt.MapClaims{}
	token, _, err := jwt.NewParser().ParseUnverified(upstreamToken, claims)
	if err != nil || token == nil || token.Method == nil || token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
		return AuthResult{}, ErrInvalidUpstreamResponse
	}
	validator := jwt.NewValidator(jwt.WithIssuer(x.issuer), jwt.WithAudience(x.audience), jwt.WithExpirationRequired(), jwt.WithTimeFunc(func() time.Time { return now }))
	if err := validator.Validate(claims); err != nil {
		return AuthResult{}, ErrInvalidUpstreamResponse
	}
	sub, err := claims.GetSubject()
	if err != nil || strings.TrimSpace(sub) == "" {
		return AuthResult{}, ErrInvalidUpstreamResponse
	}
	username, ok := claims["unique_name"].(string)
	if !ok || username == "" || username != strings.TrimSpace(submittedUsername) {
		return AuthResult{}, ErrInvalidUpstreamResponse
	}
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil || upstreamExpiry.IsZero() {
		return AuthResult{}, ErrInvalidUpstreamResponse
	}
	expiry := now.Add(directSessionLifetime)
	if exp.Time.Before(expiry) {
		expiry = exp.Time
	}
	if upstreamExpiry.Before(expiry) {
		expiry = upstreamExpiry
	}
	expiry = expiry.UTC().Truncate(time.Second)
	if !expiry.After(now) {
		return AuthResult{}, ErrInvalidUpstreamResponse
	}
	// Never copy upstream roles. Administrator access is local and case-sensitive.
	roles := []string{}
	if _, allowed := x.admins[username]; allowed {
		roles = append(roles, x.role)
	}
	jti, err := randomID(16)
	if err != nil {
		return AuthResult{}, err
	}
	localClaims := jwt.MapClaims{"sub": sub, "unique_name": username, "role": roles, "iss": x.local.Issuer, "aud": x.local.Audience, "iat": now.Unix(), "exp": expiry.Unix(), "jti": jti}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, localClaims).SignedString(x.local.Secret)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{AccessToken: signed, TokenType: "Bearer", ExpiresAtUTC: expiry}, nil
}
