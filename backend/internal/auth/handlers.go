package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
)

const maxLoginBodyBytes = 64 * 1024

// Handler serves authentication endpoints: login, me, and logout.
type Handler struct {
	provider   Provider
	validator  *JWTValidator
	sessions   SessionStore
	csrf       *CSRFProtector
	throttler  *LoginThrottler
	cookieName string
	secure     bool
	now        func() time.Time
}

// NewHandler creates an HTTP handler for /api/auth/* endpoints.
func NewHandler(
	provider Provider,
	validator *JWTValidator,
	sessions SessionStore,
	csrf *CSRFProtector,
	throttler *LoginThrottler,
	cookieName string,
	secure bool,
	now func() time.Time,
) *Handler {
	if now == nil {
		now = time.Now
	}
	return &Handler{
		provider:   provider,
		validator:  validator,
		sessions:   sessions,
		csrf:       csrf,
		throttler:  throttler,
		cookieName: cookieName,
		secure:     secure,
		now:        now,
	}
}

// SessionResponse is the normalized user, expiration, and CSRF payload returned by login and /me.
// It deliberately never includes the JWT access token or opaque session ID.
type SessionResponse struct {
	User         Principal `json:"user"`
	ExpiresAtUTC string    `json:"expiresAtUtc"`
	CSRFToken    string    `json:"csrfToken"`
}

// Login authenticates credentials through the configured provider, validates the JWT,
// rotates any prior session, sets an HttpOnly session cookie, and returns user + CSRF metadata.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	setNoStore(w)

	if !h.csrf.ValidateOrigin(r) {
		writeAuthError(w, http.StatusForbidden, "request origin is not allowed")
		return
	}

	if !isJSONContentType(r.Header.Get("Content-Type")) {
		writeAuthError(w, http.StatusBadRequest, "Content-Type must be application/json")
		return
	}

	if h.throttler != nil && !h.throttler.Allow(r) {
		writeAuthError(w, http.StatusTooManyRequests, "too many login attempts; please try again shortly")
		return
	}

	req, ok := decodeLoginRequest(w, r)
	if !ok {
		return
	}

	result, err := h.provider.Authenticate(r.Context(), req)
	if err != nil {
		writeProviderError(w, err)
		return
	}

	principal, err := h.validator.Validate(result.AccessToken)
	if err != nil {
		writeAuthError(w, http.StatusBadGateway, "authentication provider returned an invalid token")
		return
	}

	effectiveExpiry := principal.ExpiresAt
	if !result.ExpiresAtUTC.IsZero() && result.ExpiresAtUTC.Before(effectiveExpiry) {
		effectiveExpiry = result.ExpiresAtUTC.UTC()
	}

	rawSessionID, session, err := h.sessions.Create(principal, result.AccessToken, effectiveExpiry)
	if err != nil {
		writeAuthError(w, http.StatusBadGateway, "unable to establish session")
		return
	}

	// Rotate any previous browser session after successful replacement.
	if prevCookie, cookieErr := r.Cookie(h.cookieName); cookieErr == nil && prevCookie.Value != "" {
		h.sessions.Delete(prevCookie.Value)
	}

	setSessionCookie(w, h.cookieName, rawSessionID, session.ExpiresAt, h.now(), h.secure)
	writeAuthJSON(w, http.StatusOK, sessionResponseFrom(session))
}

// Me returns the active authenticated principal and CSRF token.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	setNoStore(w)
	session, ok := SessionFromContext(r.Context())
	if !ok {
		writeAuthError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	writeAuthJSON(w, http.StatusOK, sessionResponseFrom(session))
}

// Logout invalidates the local session and clears the browser session cookie.
// If an active session exists, CSRF token validation is enforced.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	setNoStore(w)

	if !h.csrf.ValidateOrigin(r) {
		writeAuthError(w, http.StatusForbidden, "request origin is not allowed")
		return
	}

	if cookie, err := r.Cookie(h.cookieName); err == nil && cookie.Value != "" {
		if session, exists := h.sessions.Get(cookie.Value); exists {
			if !h.csrf.ValidateToken(r, session) {
				writeAuthError(w, http.StatusForbidden, "missing or invalid CSRF token")
				return
			}
			h.sessions.Delete(cookie.Value)
		}
	}

	clearSessionCookie(w, h.cookieName, h.secure)
	writeAuthJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func sessionResponseFrom(session Session) SessionResponse {
	return SessionResponse{
		User:         session.Principal,
		ExpiresAtUTC: session.ExpiresAt.UTC().Format(time.RFC3339),
		CSRFToken:    session.CSRFToken,
	}
}

func decodeLoginRequest(w http.ResponseWriter, r *http.Request) (LoginRequest, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxLoginBodyBytes)
	decoder := json.NewDecoder(r.Body)

	var object map[string]json.RawMessage
	if err := decoder.Decode(&object); err != nil || object == nil {
		writeAuthError(w, http.StatusBadRequest, "request body must be a JSON object")
		return LoginRequest{}, false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeAuthError(w, http.StatusBadRequest, "request body must contain a single JSON object")
		return LoginRequest{}, false
	}
	for _, value := range object {
		if string(value) == "null" {
			writeAuthError(w, http.StatusBadRequest, "fields cannot be null")
			return LoginRequest{}, false
		}
	}

	raw, _ := json.Marshal(object)
	fields := json.NewDecoder(bytes.NewReader(raw))
	fields.DisallowUnknownFields()

	var req LoginRequest
	if err := fields.Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "request body contains an unknown field or invalid type")
		return LoginRequest{}, false
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeAuthError(w, http.StatusBadRequest, "username and password are required")
		return LoginRequest{}, false
	}

	return req, true
}

func isJSONContentType(header string) bool {
	if strings.TrimSpace(header) == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(header)
	return err == nil && mediaType == "application/json"
}

func writeProviderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		writeAuthError(w, http.StatusUnauthorized, "invalid username or password")
	case errors.Is(err, ErrProviderTimeout):
		writeAuthError(w, http.StatusGatewayTimeout, "authentication provider timed out")
	case errors.Is(err, ErrInvalidUpstreamResponse):
		writeAuthError(w, http.StatusBadGateway, "invalid response from authentication provider")
	default:
		writeAuthError(w, http.StatusServiceUnavailable, "authentication provider is unavailable")
	}
}

func setSessionCookie(w http.ResponseWriter, name, value string, expiresAt, now time.Time, secure bool) {
	maxAge := int(expiresAt.Sub(now).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/api",
		Expires:  expiresAt.UTC(),
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, name string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/api",
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}
