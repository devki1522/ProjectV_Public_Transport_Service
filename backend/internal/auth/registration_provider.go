package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
)

const (
	defaultUpstreamTimeout = 10 * time.Second
	maxUpstreamBodyBytes   = 64 * 1024
)

var errRedirectsDisabled = errors.New("redirects are not allowed for credential submission")

// RegistrationProvider authenticates against the external Registration Service HTTP API.
type RegistrationProvider struct {
	endpoint string
	client   *http.Client
}

// NewRegistrationProvider builds a Registration Service provider using the supplied HTTP client.
// Redirects are rejected so credentials are never forwarded to an unintended destination.
func NewRegistrationProvider(cfg config.RegistrationAuthConfig, client *http.Client) (*RegistrationProvider, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.ServiceURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("registration service URL is required")
	}

	safeClient := &http.Client{Timeout: defaultUpstreamTimeout}
	if client != nil {
		copied := *client
		if copied.Timeout <= 0 {
			copied.Timeout = defaultUpstreamTimeout
		}
		safeClient = &copied
	}
	safeClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errRedirectsDisabled
	}

	return &RegistrationProvider{
		endpoint: baseURL + "/api/Login/authenticate",
		client:   safeClient,
	}, nil
}

type registrationResponse struct {
	AccessToken  string `json:"accessToken"`
	TokenType    string `json:"tokenType"`
	ExpiresAtUTC string `json:"expiresAtUtc"`
	// Role is intentionally ignored for authorization decisions.
}

// Authenticate sends credentials once to /api/Login/authenticate without retries.
func (p *RegistrationProvider) Authenticate(ctx context.Context, req LoginRequest) (AuthResult, error) {
	payload, err := json.Marshal(LoginRequest{
		Username:   req.Username,
		Password:   req.Password,
		RememberMe: false,
	})
	if err != nil {
		return AuthResult{}, fmt.Errorf("encode login request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(payload))
	if err != nil {
		return AuthResult{}, fmt.Errorf("build login request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return AuthResult{}, ctx.Err()
		}
		if errors.Is(err, errRedirectsDisabled) {
			return AuthResult{}, ErrInvalidUpstreamResponse
		}
		if errors.Is(err, context.DeadlineExceeded) || isTimeoutError(err) {
			return AuthResult{}, ErrProviderTimeout
		}
		return AuthResult{}, ErrProviderUnavailable
	}
	defer resp.Body.Close()

	// Drain bounded body cleanly.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamBodyBytes+1))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || isTimeoutError(err) {
			return AuthResult{}, ErrProviderTimeout
		}
		return AuthResult{}, ErrInvalidUpstreamResponse
	}
	if len(body) > maxUpstreamBodyBytes {
		return AuthResult{}, ErrInvalidUpstreamResponse
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return AuthResult{}, ErrInvalidCredentials
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return AuthResult{}, ErrInvalidUpstreamResponse
	case resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests:
		return AuthResult{}, ErrProviderUnavailable
	case resp.StatusCode != http.StatusOK:
		return AuthResult{}, ErrInvalidUpstreamResponse
	}

	var parsed registrationResponse
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&parsed); err != nil {
		return AuthResult{}, ErrInvalidUpstreamResponse
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return AuthResult{}, ErrInvalidUpstreamResponse
	}

	if strings.TrimSpace(parsed.AccessToken) == "" {
		return AuthResult{}, ErrInvalidUpstreamResponse
	}
	if !strings.EqualFold(strings.TrimSpace(parsed.TokenType), "Bearer") {
		return AuthResult{}, ErrInvalidUpstreamResponse
	}

	expiresAt, err := parseUTC(parsed.ExpiresAtUTC)
	if err != nil {
		return AuthResult{}, ErrInvalidUpstreamResponse
	}

	return AuthResult{
		AccessToken:  parsed.AccessToken,
		TokenType:    "Bearer",
		ExpiresAtUTC: expiresAt,
	}, nil
}

func parseUTC(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, errors.New("empty timestamp")
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed.UTC(), nil
	}
	// Fallback for .NET DateTime without trailing Z when serialized with Kind=Unspecified.
	parsed, err = time.Parse("2006-01-02T15:04:05.9999999", value)
	if err == nil {
		return parsed.UTC(), nil
	}
	return time.Time{}, err
}

func isTimeoutError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
