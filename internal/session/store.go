// Package session keeps the administrators' sessions on the server: the browser
// only holds a random identifier, and the tokens never leave this process.
package session

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"sync"
	"time"
)

// Tokens are what the realm gave for one sign-in.
type Tokens struct {
	Access        string
	AccessExpiry  time.Time
	Refresh       string
	RefreshExpiry time.Time
	ID            string
}

// Session is one administrator signed in from one browser.
type Session struct {
	ID      string
	CSRF    string
	Subject string
	Name    string
	Tokens  Tokens
	created time.Time
	seen    time.Time
}

// Login is a sign-in in progress, between the redirect to the realm and its answer.
type Login struct {
	State    string
	Nonce    string
	Verifier string
	Return   string
	expires  time.Time
}

// Store holds the sessions and the sign-ins in progress, in memory: a restart signs everyone out.
type Store struct {
	mu       sync.Mutex
	sessions map[string]*Session
	logins   map[string]*Login
	idle     time.Duration
	max      time.Duration
	now      func() time.Time
}

// LoginTTL is how long a sign-in may take at the realm.
const LoginTTL = 10 * time.Minute

// NewStore makes an empty store; idle and max bound every session.
func NewStore(idle, max time.Duration) *Store {
	return &Store{sessions: map[string]*Session{}, logins: map[string]*Login{}, idle: idle, max: max, now: time.Now}
}

// Random is 32 random bytes, URL-safe.
func Random() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand does not fail on supported platforms
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// StartLogin records a sign-in and returns the key the browser keeps in its login cookie.
func (s *Store) StartLogin(l Login) string {
	key := Random()
	l.expires = s.now().Add(LoginTTL)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logins[key] = &l
	return key
}

// TakeLogin returns the sign-in of that key, once.
func (s *Store) TakeLogin(key string) (Login, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.logins[key]
	delete(s.logins, key)
	if !ok || s.now().After(l.expires) {
		return Login{}, false
	}
	return *l, true
}

// Create opens a session.
func (s *Store) Create(subject, name string, t Tokens) *Session {
	now := s.now()
	sess := &Session{ID: Random(), CSRF: Random(), Subject: subject, Name: name, Tokens: t, created: now, seen: now}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.ID] = sess
	copied := *sess
	return &copied
}

// Get returns a copy of a live session and counts the request as activity.
func (s *Store) Get(id string) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return Session{}, false
	}
	now := s.now()
	if s.expired(sess, now) {
		delete(s.sessions, id)
		return Session{}, false
	}
	sess.seen = now
	return *sess, true
}

// UpdateTokens keeps refreshed tokens of a session still open.
func (s *Store) UpdateTokens(id string, t Tokens) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok {
		sess.Tokens = t
	}
}

// Delete closes a session.
func (s *Store) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

// Sweep removes expired sessions and sign-ins until ctx ends.
func (s *Store) Sweep(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.sweep()
		}
	}
}

func (s *Store) sweep() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for id, sess := range s.sessions {
		if s.expired(sess, now) {
			delete(s.sessions, id)
		}
	}
	for key, l := range s.logins {
		if now.After(l.expires) {
			delete(s.logins, key)
		}
	}
}

func (s *Store) expired(sess *Session, now time.Time) bool {
	return now.Sub(sess.seen) > s.idle || now.Sub(sess.created) > s.max
}

// SameCSRF compares a request's token with the session's in constant time.
func SameCSRF(sess Session, token string) bool {
	return token != "" && subtle.ConstantTimeCompare([]byte(sess.CSRF), []byte(token)) == 1
}
