package auth

import (
	"fmt"
	"strings"
	"time"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
	"github.com/golang-jwt/jwt/v5"
)

// JWTValidator strictly validates HS256 JWTs against a single configured trust policy.
type JWTValidator struct {
	issuer   string
	audience string
	secret   []byte
	now      func() time.Time
}

// NewJWTValidator creates a validator bound to a single active JWT trust configuration.
func NewJWTValidator(trust config.JWTTrustConfig, now func() time.Time) (*JWTValidator, error) {
	if strings.TrimSpace(trust.Issuer) == "" || strings.TrimSpace(trust.Audience) == "" {
		return nil, fmt.Errorf("jwt issuer and audience are required")
	}
	if len(trust.Secret) < config.MinJWTSecretBytes {
		return nil, fmt.Errorf("jwt secret must be at least %d bytes", config.MinJWTSecretBytes)
	}
	if now == nil {
		now = time.Now
	}
	key := make([]byte, len(trust.Secret))
	copy(key, trust.Secret)
	return &JWTValidator{
		issuer:   trust.Issuer,
		audience: trust.Audience,
		secret:   key,
		now:      now,
	}, nil
}

// Validate verifies algorithm, signature, issuer, audience, expiration, nbf, subject, and role claims.
func (v *JWTValidator) Validate(tokenString string) (Principal, error) {
	if strings.TrimSpace(tokenString) == "" {
		return Principal{}, ErrInvalidToken
	}

	claims := jwt.MapClaims{}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(v.now),
	)

	// Never inspect unvalidated claims to choose a verification key.
	token, err := parser.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("%w: unexpected signing method", ErrInvalidToken)
		}
		return v.secret, nil
	})
	if err != nil || token == nil || !token.Valid {
		return Principal{}, ErrInvalidToken
	}

	sub, err := claims.GetSubject()
	if err != nil || strings.TrimSpace(sub) == "" {
		return Principal{}, ErrInvalidToken
	}

	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil || !exp.Time.After(v.now()) {
		return Principal{}, ErrInvalidToken
	}

	username, err := parseOptionalStringClaim(claims, "unique_name")
	if err != nil {
		return Principal{}, err
	}
	if username == "" {
		username = sub
	}

	roles, err := parseRolesClaim(claims)
	if err != nil {
		return Principal{}, err
	}

	return Principal{
		Subject:   sub,
		Username:  username,
		Roles:     roles,
		ExpiresAt: exp.Time.UTC(),
	}, nil
}

func parseOptionalStringClaim(claims jwt.MapClaims, key string) (string, error) {
	raw, exists := claims[key]
	if !exists || raw == nil {
		return "", nil
	}
	s, ok := raw.(string)
	if !ok {
		return "", ErrInvalidToken
	}
	return strings.TrimSpace(s), nil
}

func parseRolesClaim(claims jwt.MapClaims) ([]string, error) {
	raw, exists := claims["role"]
	if !exists {
		return []string{}, nil
	}
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, ErrInvalidToken
		}
		return []string{v}, nil
	case []any:
		roles := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok || strings.TrimSpace(s) == "" {
				return nil, ErrInvalidToken
			}
			roles = append(roles, s)
		}
		return roles, nil
	case []string:
		roles := make([]string, 0, len(v))
		for _, s := range v {
			if strings.TrimSpace(s) == "" {
				return nil, ErrInvalidToken
			}
			roles = append(roles, s)
		}
		return roles, nil
	default:
		return nil, ErrInvalidToken
	}
}
