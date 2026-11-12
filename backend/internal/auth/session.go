package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

const (
	sessionIDBytes         = 32
	csrfTokenBytes         = 32
	defaultMaxSessions     = 10000
	defaultCleanupInterval = time.Minute
)

// Session represents a server-side authenticated browser session.
type Session struct {
	IDHash      string
	Principal   Principal
	AccessToken string
	ExpiresAt   time.Time
	CSRFToken   string
}

// SessionStore abstracts server-side session persistence so an in-memory store
// can later be replaced by a shared store such as Redis.
type SessionStore interface {
	Create(principal Principal, accessToken string, expiresAt time.Time) (rawID string, session Session, err error)
	Get(rawID string) (Session, bool)
	Delete(rawID string)
	Close()
}

// InMemorySessionStore stores hashed session IDs in memory for a single backend instance.
// Backend restarts invalidate all active sessions; multi-instance deployments require a shared store.
type InMemorySessionStore struct {
	mu          sync.RWMutex
	sessions    map[string]Session
	maxSessions int
	now         func() time.Time
	stopCh      chan struct{}
	stopOnce    sync.Once
	doneCh      chan struct{}
}

// NewInMemorySessionStore creates a concurrency-safe session store with background expiration cleanup.
func NewInMemorySessionStore(now func() time.Time, cleanupInterval time.Duration, maxSessions int) *InMemorySessionStore {
	if now == nil {
		now = time.Now
	}
	if maxSessions <= 0 {
		maxSessions = defaultMaxSessions
	}
	if cleanupInterval <= 0 {
		cleanupInterval = defaultCleanupInterval
	}

	store := &InMemorySessionStore{
		sessions:    make(map[string]Session),
		maxSessions: maxSessions,
		now:         now,
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}

	go store.cleanupLoop(cleanupInterval)
	return store
}

// Create generates a >=32-byte random session ID and CSRF token, bounds expiration by the JWT,
// and stores only the SHA-256 hash of the session ID.
func (s *InMemorySessionStore) Create(principal Principal, accessToken string, expiresAt time.Time) (string, Session, error) {
	now := s.now().UTC()
	effectiveExpiry := expiresAt.UTC()
	if effectiveExpiry.IsZero() || (!principal.ExpiresAt.IsZero() && principal.ExpiresAt.Before(effectiveExpiry)) {
		effectiveExpiry = principal.ExpiresAt.UTC()
	}
	if !effectiveExpiry.After(now) {
		return "", Session{}, ErrInvalidToken
	}

	rawID, err := randomToken(sessionIDBytes)
	if err != nil {
		return "", Session{}, fmt.Errorf("generate session id: %w", err)
	}
	csrfToken, err := randomToken(csrfTokenBytes)
	if err != nil {
		return "", Session{}, fmt.Errorf("generate csrf token: %w", err)
	}

	idHash := hashSessionID(rawID)
	principalCopy := principal
	principalCopy.ExpiresAt = effectiveExpiry
	if principal.Roles != nil {
		principalCopy.Roles = append([]string(nil), principal.Roles...)
	}

	record := Session{
		IDHash:      idHash,
		Principal:   principalCopy,
		AccessToken: accessToken,
		ExpiresAt:   effectiveExpiry,
		CSRFToken:   csrfToken,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictExpiredLocked(now)
	if len(s.sessions) >= s.maxSessions {
		s.evictEarliestLocked()
	}
	s.sessions[idHash] = record

	return rawID, record, nil
}

// Get returns the active session if found and unexpired.
func (s *InMemorySessionStore) Get(rawID string) (Session, bool) {
	if rawID == "" {
		return Session{}, false
	}
	idHash := hashSessionID(rawID)
	now := s.now().UTC()

	s.mu.RLock()
	record, exists := s.sessions[idHash]
	s.mu.RUnlock()
	if !exists {
		return Session{}, false
	}

	if !record.ExpiresAt.After(now) || !record.Principal.ExpiresAt.After(now) {
		s.mu.Lock()
		if current, stillExists := s.sessions[idHash]; stillExists && !current.ExpiresAt.After(now) {
			delete(s.sessions, idHash)
		}
		s.mu.Unlock()
		return Session{}, false
	}

	if record.Principal.Roles != nil {
		record.Principal.Roles = append([]string(nil), record.Principal.Roles...)
	}
	return record, true
}

// Delete removes a session by its raw opaque identifier.
func (s *InMemorySessionStore) Delete(rawID string) {
	if rawID == "" {
		return
	}
	idHash := hashSessionID(rawID)
	s.mu.Lock()
	delete(s.sessions, idHash)
	s.mu.Unlock()
}

// Close stops the background cleanup goroutine cleanly.
func (s *InMemorySessionStore) Close() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	<-s.doneCh
}

func (s *InMemorySessionStore) cleanupLoop(interval time.Duration) {
	defer close(s.doneCh)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.Lock()
			s.evictExpiredLocked(s.now().UTC())
			s.mu.Unlock()
		}
	}
}

func (s *InMemorySessionStore) evictExpiredLocked(now time.Time) {
	for idHash, record := range s.sessions {
		if !record.ExpiresAt.After(now) {
			delete(s.sessions, idHash)
		}
	}
}

func (s *InMemorySessionStore) evictEarliestLocked() {
	var oldestKey string
	var oldestTime time.Time
	first := true
	for k, v := range s.sessions {
		if first || v.ExpiresAt.Before(oldestTime) {
			oldestKey = k
			oldestTime = v.ExpiresAt
			first = false
		}
	}
	if !first {
		delete(s.sessions, oldestKey)
	}
}

func hashSessionID(rawID string) string {
	sum := sha256.Sum256([]byte(rawID))
	return hex.EncodeToString(sum[:])
}

func randomToken(byteLen int) (string, error) {
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
