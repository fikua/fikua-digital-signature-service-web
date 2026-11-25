package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fikua/fikua-digital-signature-service-web/internal/session"
)

const public = "https://dss-admin.example"

type stubRealm struct {
	login      session.Login
	exchange   error
	refresh    error
	refreshed  int
	accessLife time.Duration
}

func (s *stubRealm) AuthURL(l session.Login) string {
	s.login = l
	return "https://realm.example/auth?state=" + l.State
}

func (s *stubRealm) LogoutURL(idToken, back string) string {
	return "https://realm.example/logout?hint=" + idToken + "&back=" + url.QueryEscape(back)
}

func (s *stubRealm) Exchange(_ context.Context, code string, _ session.Login) (session.Tokens, Identity, error) {
	if s.exchange != nil {
		return session.Tokens{}, Identity{}, s.exchange
	}
	return session.Tokens{Access: "access-" + code, AccessExpiry: time.Now().Add(s.accessLife), ID: "id"},
		Identity{Subject: "admin-1", Name: "Ada"}, nil
}

func (s *stubRealm) Refresh(_ context.Context, old session.Tokens) (session.Tokens, error) {
	s.refreshed++
	if s.refresh != nil {
		return session.Tokens{}, s.refresh
	}
	old.Access = "refreshed"
	old.AccessExpiry = time.Now().Add(5 * time.Minute)
	return old, nil
}

type rig struct {
	realm *stubRealm
	store *session.Store
	h     *Handlers
	mux   *http.ServeMux
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{realm: &stubRealm{accessLife: 5 * time.Minute}, store: session.NewStore(30*time.Minute, 8*time.Hour)}
	r.h = NewHandlers(r.realm, r.store, public, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r.mux = http.NewServeMux()
	r.mux.HandleFunc("GET /auth/login", r.h.Login)
	r.mux.HandleFunc("GET /auth/callback", r.h.Callback)
	r.mux.HandleFunc("GET /auth/session", r.h.Session)
	r.mux.HandleFunc("POST /auth/logout", r.h.Logout)
	r.mux.Handle("/api/v1/", r.h.Authorized(func(w http.ResponseWriter, _ *http.Request, token string) {
		_, _ = w.Write([]byte(token))
	}))
	return r
}

func (r *rig) do(req *http.Request, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	r.mux.ServeHTTP(w, req)
	return w
}

func cookie(w *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// signIn runs the whole sign-in and returns the session cookie and the CSRF token.
func (r *rig) signIn(t *testing.T) (*http.Cookie, string) {
	t.Helper()
	w := r.do(httptest.NewRequest("GET", "/auth/login?return=/%23/usuaris", nil))
	login := cookie(w, "__Host-fikua-sign-login")
	w = r.do(httptest.NewRequest("GET", "/auth/callback?code=c1&state="+r.realm.login.State, nil), login)
	s := cookie(w, "__Host-fikua-sign")
	if w.Code != http.StatusFound || s == nil {
		t.Fatalf("callback: %d %s", w.Code, w.Body)
	}
	w = r.do(httptest.NewRequest("GET", "/auth/session", nil), s)
	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return s, body["csrf"]
}

func TestSignIn(t *testing.T) {
	r := newRig(t)
	w := r.do(httptest.NewRequest("GET", "/auth/login?return=/%23/usuaris", nil))
	login := cookie(w, "__Host-fikua-sign-login")
	if w.Code != http.StatusFound || login == nil || !login.HttpOnly || !login.Secure || login.SameSite != http.SameSiteLaxMode || login.Path != "/" {
		t.Fatalf("login: %d %+v", w.Code, login)
	}
	if w.Header().Get("Location") != "https://realm.example/auth?state="+r.realm.login.State {
		t.Errorf("redirect: %s", w.Header().Get("Location"))
	}
	if r.realm.login.Nonce == "" || r.realm.login.Verifier == "" || r.realm.login.State == r.realm.login.Nonce {
		t.Errorf("login values: %+v", r.realm.login)
	}

	w = r.do(httptest.NewRequest("GET", "/auth/callback?code=c1&state="+r.realm.login.State, nil), login)
	s := cookie(w, "__Host-fikua-sign")
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/#/usuaris" {
		t.Fatalf("callback: %d %s", w.Code, w.Header().Get("Location"))
	}
	if s == nil || !s.HttpOnly || !s.Secure || s.SameSite != http.SameSiteStrictMode || s.MaxAge != 0 {
		t.Fatalf("session cookie: %+v", s)
	}
	if c := cookie(w, "__Host-fikua-sign-login"); c == nil || c.MaxAge >= 0 {
		t.Error("login cookie not cleared")
	}

	w = r.do(httptest.NewRequest("GET", "/auth/session", nil), s)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"Ada"`) || w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("session: %d %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "access-") {
		t.Error("tokens reach the browser")
	}
}

func TestCallbackRefusals(t *testing.T) {
	r := newRig(t)
	start := func() *http.Cookie {
		return cookie(r.do(httptest.NewRequest("GET", "/auth/login", nil)), "__Host-fikua-sign-login")
	}
	if w := r.do(httptest.NewRequest("GET", "/auth/callback?code=c&state=x", nil)); w.Code != 400 {
		t.Errorf("without login cookie: %d", w.Code)
	}
	if w := r.do(httptest.NewRequest("GET", "/auth/callback?code=c&state=other", nil), start()); w.Code != 400 || cookie(w, "__Host-fikua-sign") != nil {
		t.Errorf("other state: %d", w.Code)
	}
	login := start()
	state := r.realm.login.State
	r.do(httptest.NewRequest("GET", "/auth/callback?code=c&state="+state, nil), login)
	if w := r.do(httptest.NewRequest("GET", "/auth/callback?code=c&state="+state, nil), login); w.Code != 400 {
		t.Errorf("replayed callback: %d", w.Code)
	}
	login = start()
	if w := r.do(httptest.NewRequest("GET", "/auth/callback?error=access_denied&state="+r.realm.login.State, nil), login); w.Code != 302 || w.Header().Get("Location") != "/#/sense-acces" {
		t.Errorf("realm error: %d %s", w.Code, w.Header().Get("Location"))
	}
	r.realm.exchange = ErrRefused
	login = start()
	if w := r.do(httptest.NewRequest("GET", "/auth/callback?code=c&state="+r.realm.login.State, nil), login); w.Code != 400 {
		t.Errorf("refused code: %d", w.Code)
	}
	r.realm.exchange = errors.New("down")
	login = start()
	if w := r.do(httptest.NewRequest("GET", "/auth/callback?code=c&state="+r.realm.login.State, nil), login); w.Code != 502 {
		t.Errorf("realm down: %d", w.Code)
	}
}

func TestReturnStaysInThePortal(t *testing.T) {
	for in, want := range map[string]string{
		"/#/usuaris": "/#/usuaris", "": "/", "https://evil.example": "/", "//evil.example": "/",
		"/\\evil.example": "/", "relative": "/", "/ok?x=1": "/ok?x=1", "/a\r\nb": "/",
	} {
		if got := safeReturn(in); got != want {
			t.Errorf("safeReturn(%q) = %q", in, got)
		}
	}
}

func api(method, path, origin, csrf string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if csrf != "" {
		req.Header.Set(CSRFHeader, csrf)
	}
	return req
}

func TestAuthorized(t *testing.T) {
	r := newRig(t)
	if w := r.do(api("GET", "/api/v1/users", "", "")); w.Code != 401 {
		t.Errorf("without session: %d", w.Code)
	}
	s, csrf := r.signIn(t)
	if w := r.do(api("GET", "/api/v1/users", "", ""), s); w.Code != 200 || w.Body.String() != "access-c1" {
		t.Errorf("read: %d %s", w.Code, w.Body)
	}
	for name, req := range map[string]*http.Request{
		"no csrf":      api("POST", "/api/v1/users", public, ""),
		"other csrf":   api("POST", "/api/v1/users", public, csrf+"x"),
		"no origin":    api("DELETE", "/api/v1/credentials/x", "", csrf),
		"other origin": api("PATCH", "/api/v1/users/x", "https://evil.example", csrf),
	} {
		if w := r.do(req, s); w.Code != 403 {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
	if w := r.do(api("POST", "/api/v1/users", public, csrf), s); w.Code != 200 {
		t.Errorf("write: %d", w.Code)
	}
	if w := r.do(api("GET", "/api/v1/users", "", ""), &http.Cookie{Name: "__Host-fikua-sign", Value: "forged"}); w.Code != 401 {
		t.Errorf("forged session: %d", w.Code)
	}
}

func TestAccessTokenIsRefreshed(t *testing.T) {
	r := newRig(t)
	r.realm.accessLife = 10 * time.Second // below refreshBefore
	s, _ := r.signIn(t)
	if w := r.do(api("GET", "/api/v1/users", "", ""), s); w.Body.String() != "refreshed" || r.realm.refreshed != 1 {
		t.Fatalf("not refreshed: %s", w.Body)
	}
	if w := r.do(api("GET", "/api/v1/users", "", ""), s); w.Body.String() != "refreshed" || r.realm.refreshed != 1 {
		t.Errorf("refreshed again: %d", r.realm.refreshed)
	}
}

func TestRefreshFailures(t *testing.T) {
	r := newRig(t)
	r.realm.accessLife = time.Second
	s, _ := r.signIn(t)
	r.realm.refresh = errors.New("down")
	if w := r.do(api("GET", "/api/v1/users", "", ""), s); w.Code != 503 {
		t.Errorf("realm down: %d", w.Code)
	}
	r.realm.refresh = ErrRefused
	w := r.do(api("GET", "/api/v1/users", "", ""), s)
	if w.Code != 401 || cookie(w, "__Host-fikua-sign") == nil {
		t.Errorf("refused refresh: %d", w.Code)
	}
	r.realm.refresh = nil
	if w := r.do(api("GET", "/api/v1/users", "", ""), s); w.Code != 401 {
		t.Errorf("session survived a refused refresh: %d", w.Code)
	}
}

func TestLogout(t *testing.T) {
	r := newRig(t)
	s, csrf := r.signIn(t)
	if w := r.do(api("POST", "/auth/logout", public, ""), s); w.Code != 403 {
		t.Errorf("logout without csrf: %d", w.Code)
	}
	w := r.do(api("POST", "/auth/logout", public, csrf), s)
	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != 200 || !strings.HasPrefix(body["logout"], "https://realm.example/logout?hint=id") {
		t.Errorf("logout: %d %s", w.Code, w.Body)
	}
	if w := r.do(api("GET", "/api/v1/users", "", ""), s); w.Code != 401 {
		t.Errorf("session after logout: %d", w.Code)
	}
}
