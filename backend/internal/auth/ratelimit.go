package auth

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultMaxAttempts    = 10
	defaultThrottleWindow = time.Minute
	defaultMaxTrackers    = 4096
)

type rateBucket struct {
	count   int
	resetAt time.Time
}

// LoginThrottler bounds repeated login attempts per client IP address.
type LoginThrottler struct {
	mu           sync.Mutex
	buckets      map[string]rateBucket
	maxAttempts  int
	window       time.Duration
	maxTrackers  int
	trustedProxy bool
	now          func() time.Time
}

// NewLoginThrottler creates a bounded per-client login rate limiter.
func NewLoginThrottler(maxAttempts int, window time.Duration, trustedProxy bool, now func() time.Time) *LoginThrottler {
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}
	if window <= 0 {
		window = defaultThrottleWindow
	}
	if now == nil {
		now = time.Now
	}
	return &LoginThrottler{
		buckets:      make(map[string]rateBucket),
		maxAttempts:  maxAttempts,
		window:       window,
		maxTrackers:  defaultMaxTrackers,
		trustedProxy: trustedProxy,
		now:          now,
	}
}

// Allow reports whether a login attempt from the request's client IP is within the rate limit.
func (t *LoginThrottler) Allow(r *http.Request) bool {
	key := t.clientKey(r)
	now := t.now()

	t.mu.Lock()
	defer t.mu.Unlock()

	bucket, exists := t.buckets[key]
	if !exists || !now.Before(bucket.resetAt) {
		if !exists && len(t.buckets) >= t.maxTrackers {
			t.evictExpiredLocked(now)
			if len(t.buckets) >= t.maxTrackers {
				for k := range t.buckets {
					delete(t.buckets, k)
					break
				}
			}
		}
		t.buckets[key] = rateBucket{count: 1, resetAt: now.Add(t.window)}
		return true
	}

	if bucket.count >= t.maxAttempts {
		return false
	}

	bucket.count++
	t.buckets[key] = bucket
	return true
}

func (t *LoginThrottler) evictExpiredLocked(now time.Time) {
	for k, v := range t.buckets {
		if !now.Before(v.resetAt) {
			delete(t.buckets, k)
		}
	}
}

func (t *LoginThrottler) clientKey(r *http.Request) string {
	if t.trustedProxy {
		if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
			first, _, _ := strings.Cut(xff, ",")
			if ip := net.ParseIP(strings.TrimSpace(first)); ip != nil {
				return ip.String()
			}
		}
		if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
			if ip := net.ParseIP(xrip); ip != nil {
				return ip.String()
			}
		}
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if r.RemoteAddr != "" {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return "unknown"
}
