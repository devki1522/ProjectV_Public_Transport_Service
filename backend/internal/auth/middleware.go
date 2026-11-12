package auth

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
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

// Middleware enforces cookie-backed sessions, JWT revalidation, and role checks.
type Middleware struct {
	sessions   SessionStore
	validator  *JWTValidator
	cookieName string
	secure     bool
}

// NewMiddleware constructs authentication and authorization middleware.
func NewMiddleware(sessions SessionStore, validator *JWTValidator, cookieName string, secure bool) *Middleware {
	return &Middleware{
		sessions:   sessions,
		validator:  validator,
		cookieName: cookieName,
		secure:     secure,
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
