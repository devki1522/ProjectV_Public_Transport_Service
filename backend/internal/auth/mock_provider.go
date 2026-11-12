package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
	"github.com/golang-jwt/jwt/v5"
)

const mockTokenLifetime = 15 * time.Minute

type mockIdentity struct {
	subject      string
	username     string
	role         string
	passwordHash [32]byte
}

// MockProvider issues genuine HS256 JWTs for development identities.
type MockProvider struct {
	issuer   string
	audience string
	secret   []byte
	users    map[string]mockIdentity
	now      func() time.Time
}

// NewMockProvider constructs the development-only mock authentication provider.
func NewMockProvider(trust config.JWTTrustConfig, mockCfg config.MockAuthConfig, now func() time.Time) (*MockProvider, error) {
	if strings.TrimSpace(trust.Issuer) == "" || strings.TrimSpace(trust.Audience) == "" {
		return nil, fmt.Errorf("mock jwt issuer and audience are required")
	}
	if len(trust.Secret) < config.MinJWTSecretBytes {
		return nil, fmt.Errorf("mock jwt secret must be at least %d bytes", config.MinJWTSecretBytes)
	}
	if mockCfg.AdminPassword == "" || mockCfg.UserPassword == "" {
		return nil, fmt.Errorf("mock user passwords are required")
	}
	if now == nil {
		now = time.Now
	}

	key := make([]byte, len(trust.Secret))
	copy(key, trust.Secret)

	return &MockProvider{
		issuer:   trust.Issuer,
		audience: trust.Audience,
		secret:   key,
		now:      now,
		users: map[string]mockIdentity{
			"transit.admin": {
				subject:      "dev-transit-admin-001",
				username:     "transit.admin",
				role:         "TransitAdministrator",
				passwordHash: sha256.Sum256([]byte(mockCfg.AdminPassword)),
			},
			"transit.user": {
				subject:      "dev-transit-user-001",
				username:     "transit.user",
				role:         "User",
				passwordHash: sha256.Sum256([]byte(mockCfg.UserPassword)),
			},
		},
	}, nil
}

// Authenticate verifies mock credentials and returns a 15-minute HS256 JWT.
func (p *MockProvider) Authenticate(ctx context.Context, req LoginRequest) (AuthResult, error) {
	if err := ctx.Err(); err != nil {
		return AuthResult{}, err
	}

	username := strings.TrimSpace(req.Username)
	candidateHash := sha256.Sum256([]byte(req.Password))

	identity, exists := p.users[username]
	expectedHash := sha256.Sum256([]byte("dummy-constant-time-fallback"))
	if exists {
		expectedHash = identity.passwordHash
	}

	match := subtle.ConstantTimeCompare(candidateHash[:], expectedHash[:]) == 1
	if !exists || req.Password == "" || !match {
		return AuthResult{}, ErrInvalidCredentials
	}

	issuedAt := p.now().UTC()
	// rememberMe deliberately does not extend JWT lifetime.
	expiresAt := issuedAt.Add(mockTokenLifetime)

	jti, err := randomID(16)
	if err != nil {
		return AuthResult{}, fmt.Errorf("generate token id: %w", err)
	}

	claims := jwt.MapClaims{
		"sub":         identity.subject,
		"unique_name": identity.username,
		"jti":         jti,
		"role":        identity.role,
		"iss":         p.issuer,
		"aud":         p.audience,
		"iat":         issuedAt.Unix(),
		"exp":         expiresAt.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(p.secret)
	if err != nil {
		return AuthResult{}, fmt.Errorf("sign mock jwt: %w", err)
	}

	return AuthResult{
		AccessToken:  signed,
		TokenType:    "Bearer",
		ExpiresAtUTC: expiresAt,
	}, nil
}

func randomID(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
