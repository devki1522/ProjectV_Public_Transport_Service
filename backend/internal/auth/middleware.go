package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
)

type contextKey int

const (
	principalContextKey contextKey = iota + 1
	sessionContextKey
)

// PrincipalFromContext returns the authenticated principal attached to the request context.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey).(Principal)
	return principal, ok
}

// SessionFromContext returns the validated server-side session attached to the request context.
func SessionFromContext(ctx context.Context) (Session, bool) {
	session, ok := ctx.Value(sessionContextKey).(Session)
	return session, ok
}

// WithPrincipal attaches a principal to a context (also useful in isolated unit tests).
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey, principal)
}

func withSessionAndPrincipal(ctx context.Context, session Session, principal Principal) context.Context {
	ctx = context.WithValue(ctx, sessionContextKey, session)
	return context.WithValue(ctx, principalContextKey, principal)
}

// Middleware enforces cookie-backed sessions, JWT revalidation, Bearer service tokens, and role checks.
type Middleware struct {
	sessions        SessionStore
	validator       *JWTValidator
	bearerValidator BearerTokenValidator
	cookieName      string
	secure          bool
}

// NewMiddleware constructs authentication and authorization middleware.
func NewMiddleware(sessions SessionStore, validator *JWTValidator, cookieName string, secure bool) *Middleware {
	var bv BearerTokenValidator
	if validator != nil {
		bv = validator
	}
	return NewMiddlewareWithBearerValidator(sessions, validator, bv, cookieName, secure)
}

// NewMiddlewareWithBearerValidator constructs middleware with an explicit external Bearer token validator.
func NewMiddlewareWithBearerValidator(
	sessions SessionStore,
	validator *JWTValidator,
	bearerValidator BearerTokenValidator,
	cookieName string,
	secure bool,
) *Middleware {
	if bearerValidator == nil && validator != nil {
		bearerValidator = validator
	}
	return &Middleware{
		sessions:        sessions,
		validator:       validator,
		bearerValidator: bearerValidator,
		cookieName:      cookieName,
		secure:          secure,
	}
}

// RequireAuthenticated validates the opaque session cookie, revalidates the stored JWT,
// and attaches the normalized Principal and Session to the request context.
func (m *Middleware) RequireAuthenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setNoStore(w)

		cookie, err := r.Cookie(m.cookieName)
		if err != nil || cookie.Value == "" {
			writeAuthError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		session, exists := m.sessions.Get(cookie.Value)
		if !exists {
			clearSessionCookie(w, m.cookieName, m.secure)
			writeAuthError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		// Revalidate stored JWT against the fixed trust configuration and clock.
		principal, err := m.validator.Validate(session.AccessToken)
		if err != nil {
			m.sessions.Delete(cookie.Value)
			clearSessionCookie(w, m.cookieName, m.secure)
			writeAuthError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		// Enforce the earlier of the session expiry and revalidated JWT expiry.
		if !session.ExpiresAt.After(m.validator.now()) {
			m.sessions.Delete(cookie.Value)
			clearSessionCookie(w, m.cookieName, m.secure)
			writeAuthError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		principal.ExpiresAt = session.ExpiresAt
		ctx := withSessionAndPrincipal(r.Context(), session, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole rejects authenticated callers lacking the exact configured role with HTTP 403.
func (m *Middleware) RequireRole(requiredRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			setNoStore(w)

			principal, ok := PrincipalFromContext(r.Context())
			if !ok {
				writeAuthError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			if !principal.HasRole(requiredRole) {
				writeAuthError(w, http.StatusForbidden, "administrator access is required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireIntegrationAccess authenticates /integrations/* endpoints via either:
//  1. Authorization: Bearer <JWT> validated by bearerValidator with an allowed integration role (SH-S07),
//     bypassing browser cookie and CSRF checks; or
//  2. The existing browser session cookie + sessionRequiredRole (+ CSRF when requireCSRFForSession is true).
//
// When an Authorization header is present, authentication uses Bearer JWT exclusively and never
// falls back to cookies.
func (m *Middleware) RequireIntegrationAccess(
	sessionRequiredRole string,
	bearerAllowedRoles []string,
	csrf *CSRFProtector,
	requireCSRFForSession bool,
) func(http.Handler) http.Handler {
	allowedRoles := make([]string, 0, len(bearerAllowedRoles))
	for _, role := range bearerAllowedRoles {
		if role != "" {
			allowedRoles = append(allowedRoles, role)
		}
	}

	return func(next http.Handler) http.Handler {
		sessionChain := next
		if m != nil && sessionRequiredRole != "" && (!requireCSRFForSession || csrf != nil) {
			if requireCSRFForSession {
				sessionChain = csrf.RequireMutationCSRF(sessionChain)
			}
			sessionChain = m.RequireRole(sessionRequiredRole)(sessionChain)
			sessionChain = m.RequireAuthenticated(sessionChain)
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			setNoStore(w)

			if rawAuth := strings.TrimSpace(r.Header.Get("Authorization")); rawAuth != "" {
				token, ok := parseBearerToken(rawAuth)
				if !ok || m == nil || m.bearerValidator == nil {
					writeAuthError(w, http.StatusUnauthorized, "authentication required")
					return
				}
				principal, err := m.bearerValidator.ValidateBearer(r.Context(), token)
				if err != nil {
					writeBearerValidationError(w, err)
					return
				}
				if len(allowedRoles) == 0 || !principal.HasAnyRole(allowedRoles) {
					writeAuthError(w, http.StatusForbidden, "integration service access is required")
					return
				}
				next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
				return
			}

			if m == nil || sessionRequiredRole == "" || (requireCSRFForSession && csrf == nil) {
				writeAuthError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			sessionChain.ServeHTTP(w, r)
		})
	}
}

func parseBearerToken(rawAuth string) (string, bool) {
	scheme, rest, ok := strings.Cut(rawAuth, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token := strings.TrimSpace(rest)
	if !isCleanBearerToken(token) {
		return "", false
	}
	return token, true
}

func isCleanBearerToken(token string) bool {
	if token == "" {
		return false
	}
	for i := 0; i < len(token); i++ {
		if token[i] <= 0x20 || token[i] == 0x7f {
			return false
		}
	}
	return true
}

func writeBearerValidationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrProviderTimeout):
		writeAuthError(w, http.StatusGatewayTimeout, "authentication provider timed out")
	case errors.Is(err, ErrInvalidUpstreamResponse):
		writeAuthError(w, http.StatusBadGateway, "invalid response from authentication provider")
	case errors.Is(err, ErrProviderUnavailable):
		writeAuthError(w, http.StatusServiceUnavailable, "authentication provider is unavailable")
	default:
		writeAuthError(w, http.StatusUnauthorized, "authentication required")
	}
}

func setNoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	writeAuthJSON(w, status, map[string]string{"error": message})
}

func writeAuthJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("encode auth response: %v", err)
	}
}
