package auth

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
)

// CSRFHeaderName is the custom header carrying the session-bound CSRF token.
const CSRFHeaderName = "X-CSRF-Token"

// CSRFProtector validates trusted browser origins and session-bound CSRF tokens.
type CSRFProtector struct {
	trustedOrigins map[string]struct{}
}

// NewCSRFProtector constructs a CSRF protector for the normalized trusted origins.
func NewCSRFProtector(origins []string) *CSRFProtector {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		if normalized, ok := normalizeOrigin(origin); ok {
			allowed[normalized] = struct{}{}
		}
	}
	return &CSRFProtector{trustedOrigins: allowed}
}

// ValidateOrigin enforces that the Origin header is present, non-null, and in TrustedOrigins.
func (p *CSRFProtector) ValidateOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" || strings.EqualFold(origin, "null") {
		return false
	}
	normalized, ok := normalizeOrigin(origin)
	if !ok {
		return false
	}
	_, allowed := p.trustedOrigins[normalized]
	return allowed
}

// ValidateToken verifies the request CSRF header against the session token in constant time.
func (p *CSRFProtector) ValidateToken(r *http.Request, session Session) bool {
	provided := strings.TrimSpace(r.Header.Get(CSRFHeaderName))
	if provided == "" || session.CSRFToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(session.CSRFToken)) == 1
}

// RequireMutationCSRF rejects browser mutations missing a trusted origin or valid CSRF token.
func (p *CSRFProtector) RequireMutationCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setNoStore(w)
		if !p.ValidateOrigin(r) {
			writeAuthError(w, http.StatusForbidden, "request origin is not allowed")
			return
		}
		session, ok := SessionFromContext(r.Context())
		if !ok || !p.ValidateToken(r, session) {
			writeAuthError(w, http.StatusForbidden, "missing or invalid CSRF token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func normalizeOrigin(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	return scheme + "://" + strings.ToLower(parsed.Host), true
}
