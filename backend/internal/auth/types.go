// Package auth provides authentication providers, HS256 JWT validation,
// server-side sessions, CSRF protection, login throttling, and HTTP middleware.
package auth

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrInvalidCredentials indicates rejected username/password credentials.
	ErrInvalidCredentials = errors.New("invalid username or password")
	// ErrInvalidToken indicates a JWT failed signature, claim, or expiration validation.
	ErrInvalidToken = errors.New("invalid access token")
	// ErrProviderUnavailable indicates the upstream authentication provider could not be reached or failed.
	ErrProviderUnavailable = errors.New("authentication provider unavailable")
	// ErrProviderTimeout indicates the upstream authentication provider timed out.
	ErrProviderTimeout = errors.New("authentication provider timed out")
	// ErrInvalidUpstreamResponse indicates the upstream provider returned a malformed response.
	ErrInvalidUpstreamResponse = errors.New("invalid upstream authentication response")
	// ErrUnauthenticated indicates a missing, unknown, invalid, or expired session.
	ErrUnauthenticated = errors.New("authentication required")
	// ErrForbidden indicates an authenticated user lacks the required role.
	ErrForbidden = errors.New("forbidden")
	// ErrCSRFInvalid indicates a missing or invalid origin or CSRF token.
	ErrCSRFInvalid = errors.New("invalid CSRF token or origin")
	// ErrRateLimited indicates login attempts exceeded the allowed rate.
	ErrRateLimited = errors.New("too many login attempts")
)

// LoginRequest is the credential payload sent to an authentication provider.
type LoginRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	RememberMe bool   `json:"rememberMe"`
}

// AuthResult contains the raw token metadata returned by an authentication provider.
type AuthResult struct {
	AccessToken  string
	TokenType    string
	ExpiresAtUTC time.Time
}

// Principal represents a validated authenticated user attached to a session and request context.
type Principal struct {
	Subject   string    `json:"id"`
	Username  string    `json:"username"`
	Roles     []string  `json:"roles"`
	ExpiresAt time.Time `json:"-"`
}

// HasRole reports whether the principal has the exact case-sensitive role.
func (p Principal) HasRole(role string) bool {
	if role == "" {
		return false
	}
	for _, r := range p.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// Provider authenticates user credentials and returns a signed JWT result.
type Provider interface {
	Authenticate(context.Context, LoginRequest) (AuthResult, error)
}
