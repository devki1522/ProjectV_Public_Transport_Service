package auth

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
)

const (
	defaultUpstreamTimeout  = 10 * time.Second
	maxUpstreamBodyBytes    = 64 * 1024
	claimTypeName           = "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name"
	claimTypeNameIdentifier = "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/nameidentifier"
	claimTypeRole           = "http://schemas.microsoft.com/ws/2008/06/identity/claims/role"
)

var errRedirectsDisabled = errors.New("redirects are not allowed for credential submission")

// RegistrationProvider authenticates against the external Registration Service HTTP API.
type RegistrationProvider struct {
	endpoint         string
	validateEndpoint string
	upstreamIssuer   string
	upstreamAudience string
	client           *http.Client
	direct           *directRegistrationExchange
}

// NewRegistrationProvider rejects redirects so credentials cannot be forwarded elsewhere.
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
	safeClient.CheckRedirect = func(*http.Request, []*http.Request) error { return errRedirectsDisabled }
	p := &RegistrationProvider{
		endpoint:         baseURL + "/api/Login/authenticate",
		validateEndpoint: baseURL + "/api/Login/validate",
		upstreamIssuer:   cfg.UpstreamIssuer,
		upstreamAudience: cfg.UpstreamAudience,
		client:           safeClient,
	}
	if !strings.HasPrefix(baseURL, "https://") {
		return nil, fmt.Errorf("direct registration exchange requires HTTPS")
	}
	// Always verify the server certificate and hostname. An injected client may
	// supply a trusted CA pool, but cannot disable TLS verification or intercept credentials.
	transport := &http.Transport{TLSHandshakeTimeout: defaultUpstreamTimeout}
	if supplied, ok := safeClient.Transport.(*http.Transport); ok && supplied.TLSClientConfig != nil {
		transport.TLSClientConfig = &tls.Config{RootCAs: supplied.TLSClientConfig.RootCAs, MinVersion: tls.VersionTLS12}
	}
	p.client = &http.Client{Timeout: safeClient.Timeout, CheckRedirect: safeClient.CheckRedirect, Transport: transport}

	exchange, err := newDirectRegistrationExchange(cfg)
	if err != nil {
		return nil, err
	}
	p.direct = exchange
	return p, nil
}

type registrationResponse struct {
	AccessToken  string `json:"accessToken"`
	TokenType    string `json:"tokenType"`
	ExpiresAtUTC string `json:"expiresAtUtc"`
	// Role intentionally ignored for authorization decisions.
}

type registrationClaim struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type registrationValidationResponse struct {
	Authenticated bool                `json:"authenticated"`
	Claims        []registrationClaim `json:"claims"`
}

// ValidateBearer validates a Registration Service Bearer token by calling POST /api/Login/validate
// over HTTPS and converting its claims into a Principal.
func (p *RegistrationProvider) ValidateBearer(ctx context.Context, tokenString string) (Principal, error) {
	if !isCleanBearerToken(tokenString) {
		return Principal{}, ErrInvalidToken
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.validateEndpoint, nil)
	if err != nil {
		return Principal{}, fmt.Errorf("build token validation request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+tokenString)
	httpReq.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return Principal{}, ctx.Err()
		}
		if errors.Is(err, errRedirectsDisabled) {
			return Principal{}, ErrInvalidUpstreamResponse
		}
		if errors.Is(err, context.DeadlineExceeded) || isTimeoutError(err) {
			return Principal{}, ErrProviderTimeout
		}
		return Principal{}, ErrProviderUnavailable
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamBodyBytes+1))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || isTimeoutError(err) {
			return Principal{}, ErrProviderTimeout
		}
		return Principal{}, ErrInvalidUpstreamResponse
	}
	if len(body) > maxUpstreamBodyBytes {
		return Principal{}, ErrInvalidUpstreamResponse
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return Principal{}, ErrInvalidToken
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return Principal{}, ErrInvalidUpstreamResponse
	case resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests:
		return Principal{}, ErrProviderUnavailable
	case resp.StatusCode != http.StatusOK:
		return Principal{}, ErrInvalidUpstreamResponse
	}

	var parsed registrationValidationResponse
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&parsed); err != nil {
		return Principal{}, ErrInvalidUpstreamResponse
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Principal{}, ErrInvalidUpstreamResponse
	}
	if !parsed.Authenticated {
		return Principal{}, ErrInvalidToken
	}

	return principalFromValidationClaims(parsed.Claims, p.upstreamIssuer, p.upstreamAudience)
}

func principalFromValidationClaims(claims []registrationClaim, expectedIssuer, expectedAudience string) (Principal, error) {
	var (
		sub        string
		nameID     string
		uniqueName string
		name       string
		xmlName    string
		hasIss     bool
		hasAud     bool
		audMatches bool
		expiresAt  time.Time
	)
	roles := make([]string, 0)
	seenRoles := make(map[string]struct{})

	for _, claim := range claims {
		val := strings.TrimSpace(claim.Value)
		switch claim.Type {
		case "sub":
			if sub == "" && val != "" {
				sub = val
			}
		case claimTypeNameIdentifier:
			if nameID == "" && val != "" {
				nameID = val
			}
		case "unique_name":
			if uniqueName == "" && val != "" {
				uniqueName = val
			}
		case "name":
			if name == "" && val != "" {
				name = val
			}
		case claimTypeName:
			if xmlName == "" && val != "" {
				xmlName = val
			}
		case "role", claimTypeRole:
			if val != "" {
				if _, exists := seenRoles[val]; !exists {
					seenRoles[val] = struct{}{}
					roles = append(roles, val)
				}
			}
		case "iss":
			hasIss = true
			if expectedIssuer != "" && val != expectedIssuer {
				return Principal{}, ErrInvalidToken
			}
		case "aud":
			hasAud = true
			if expectedAudience != "" && val == expectedAudience {
				audMatches = true
			}
		case "exp":
			if val != "" {
				if unixSec, err := strconv.ParseInt(val, 10, 64); err == nil {
					expiresAt = time.Unix(unixSec, 0).UTC()
				}
			}
		}
	}

	if hasIss && expectedIssuer == "" {
		return Principal{}, ErrInvalidToken
	}
	if hasAud && expectedAudience != "" && !audMatches {
		return Principal{}, ErrInvalidToken
	}

	if sub == "" {
		sub = nameID
	}
	username := uniqueName
	if username == "" {
		username = name
	}
	if username == "" {
		username = xmlName
	}
	if sub == "" {
		sub = username
	}
	if username == "" {
		username = sub
	}
	if sub == "" {
		return Principal{}, ErrInvalidUpstreamResponse
	}

	return Principal{
		Subject:   sub,
		Username:  username,
		Roles:     roles,
		ExpiresAt: expiresAt,
	}, nil
}

// Authenticate sends credentials once to the configured upstream, without retries.
func (p *RegistrationProvider) Authenticate(ctx context.Context, req LoginRequest) (AuthResult, error) {
	payload, err := json.Marshal(LoginRequest{Username: req.Username, Password: req.Password, RememberMe: false})
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
	return p.direct.exchange(parsed.AccessToken, req.Username, expiresAt, time.Now().UTC())
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
