package session

import (
	"context"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newStore() (*Store, *clock) {
	c := &clock{t: time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC)}
	s := NewStore(30*time.Minute, 8*time.Hour)
	s.now = c.now
	return s, c
}

func TestLoginIsTakenOnce(t *testing.T) {
	s, _ := newStore()
	key := s.StartLogin(Login{State: "st"})
	if l, ok := s.TakeLogin(key); !ok || l.State != "st" {
		t.Fatal("login not found")
	}
	if _, ok := s.TakeLogin(key); ok {
		t.Error("login taken twice")
	}
	if _, ok := s.TakeLogin("unknown"); ok {
		t.Error("unknown login taken")
	}
}

func TestLoginExpires(t *testing.T) {
	s, c := newStore()
	key := s.StartLogin(Login{State: "st"})
	c.t = c.t.Add(LoginTTL + time.Second)
	if _, ok := s.TakeLogin(key); ok {
		t.Error("expired login taken")
	}
}

func TestSessionIdleAndMax(t *testing.T) {
	s, c := newStore()
	a := s.Create("sub", "Ada", Tokens{Access: "a"})
	for range 19 { // active every 25 minutes for 7 h 55 min: idle never reached
		c.t = c.t.Add(25 * time.Minute)
		if _, ok := s.Get(a.ID); !ok {
			t.Fatalf("active session closed at %v", c.t)
		}
	}
	c.t = c.t.Add(6 * time.Minute) // 8 h 01 min after the start
	if _, ok := s.Get(a.ID); ok {
		t.Error("session lasted beyond its maximum")
	}

	b := s.Create("sub", "Ada", Tokens{})
	c.t = c.t.Add(31 * time.Minute)
	if _, ok := s.Get(b.ID); ok {
		t.Error("idle session still open")
	}
}

func TestUpdateAndDelete(t *testing.T) {
	s, _ := newStore()
	a := s.Create("sub", "Ada", Tokens{Access: "old"})
	s.UpdateTokens(a.ID, Tokens{Access: "new"})
	got, _ := s.Get(a.ID)
	if got.Tokens.Access != "new" {
		t.Errorf("tokens not updated: %s", got.Tokens.Access)
	}
	s.Delete(a.ID)
	if _, ok := s.Get(a.ID); ok {
		t.Error("deleted session found")
	}
}

func TestSweep(t *testing.T) {
	s, c := newStore()
	s.Create("sub", "Ada", Tokens{})
	s.StartLogin(Login{})
	c.t = c.t.Add(time.Hour)
	s.sweep()
	if len(s.sessions) != 0 || len(s.logins) != 0 {
		t.Errorf("left: %d sessions, %d logins", len(s.sessions), len(s.logins))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.Sweep(ctx, time.Millisecond) // returns when ctx is done
}

func TestCSRF(t *testing.T) {
	s, _ := newStore()
	a := s.Create("sub", "Ada", Tokens{})
	if !SameCSRF(*a, a.CSRF) || SameCSRF(*a, "") || SameCSRF(*a, a.CSRF+"x") {
		t.Error("CSRF comparison")
	}
	if Random() == Random() || len(Random()) != 43 {
		t.Error("random values")
	}
}
