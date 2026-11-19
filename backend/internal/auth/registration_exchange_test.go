package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
	"github.com/golang-jwt/jwt/v5"
)

func TestDirectRegistrationExchange(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	local := config.JWTTrustConfig{Issuer: "local", Audience: "transit-api", Secret: []byte(strings.Repeat("l", 32))}
	exchange, err := newDirectRegistrationExchange(config.RegistrationAuthConfig{UpstreamIssuer: "registration", UpstreamAudience: "clients", LocalTrust: local, AdminUsers: []string{"transit"}, RequiredRole: "TransitAdministrator"})
	if err != nil {
		t.Fatal(err)
	}
	validator, err := NewJWTValidator(local, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, username, submitted, issuer, audience string
		expiry                                      time.Time
		wantAdmin, wantErr                          bool
	}{
		{"admin", "transit", "transit", "registration", "clients", now.Add(time.Hour), true, false},
		{"nonadmin even with upstream role", "resident", "resident", "registration", "clients", now.Add(time.Hour), false, false},
		{"wrong username", "transit", "resident", "registration", "clients", now.Add(time.Hour), false, true},
		{"wrong issuer", "transit", "transit", "other", "clients", now.Add(time.Hour), false, true},
		{"wrong audience", "transit", "transit", "registration", "other", now.Add(time.Hour), false, true},
		{"expired", "transit", "transit", "registration", "clients", now.Add(-time.Minute), false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "id-123", "unique_name": tc.username, "role": "TransitAdministrator", "iss": tc.issuer, "aud": tc.audience, "exp": tc.expiry.Unix()})
			raw, err := upstream.SignedString([]byte(strings.Repeat("u", 32)))
			if err != nil {
				t.Fatal(err)
			}
			result, err := exchange.exchange(raw, tc.submitted, now.Add(time.Hour), now)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected rejection")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.AccessToken == raw {
				t.Fatal("upstream token must not be reused")
			}
			if _, err := validator.Validate(raw); err == nil {
				t.Fatal("upstream token accepted as local token")
			}
			principal, err := validator.Validate(result.AccessToken)
			if err != nil {
				t.Fatal(err)
			}
			if principal.Username != tc.username || principal.HasRole("TransitAdministrator") != tc.wantAdmin {
				t.Fatalf("unexpected principal: %+v", principal)
			}
			if result.ExpiresAtUTC.After(now.Add(directSessionLifetime)) {
				t.Fatal("session lifetime exceeded")
			}
		})
	}
}
