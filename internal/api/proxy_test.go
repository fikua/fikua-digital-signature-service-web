package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestForwardsToTheAdministrationAPI(t *testing.T) {
	var got *http.Request
	var body string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.Header().Set("Set-Cookie", "x=y")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()
	base, _ := url.Parse(upstream.URL)
	p := NewProxy(base, slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest("PUT", "/api/v1/credentials/c-1/certificate?x=1", strings.NewReader("PEM"))
	req.Header.Set("Content-Type", "application/x-pem-file")
	req.Header.Set("Cookie", "__Host-fikua-sign=secret")
	req.Header.Set("Authorization", "Bearer from-the-browser")
	req.Header.Set("X-CSRF-Token", "csrf")
	req.Header.Set("Origin", "https://dss-admin.example")
	w := httptest.NewRecorder()
	p.Serve(w, req, "access")

	if w.Code != 201 || w.Body.String() != `{"ok":true}` || w.Header().Get("Set-Cookie") != "" {
		t.Errorf("answer: %d %s %v", w.Code, w.Body, w.Header())
	}
	if got.URL.Path != "/admin/v1/credentials/c-1/certificate" || got.URL.RawQuery != "x=1" || got.Method != "PUT" {
		t.Errorf("upstream request: %s %s", got.Method, got.URL)
	}
	if got.Header.Get("Authorization") != "Bearer access" || got.Header.Get("Content-Type") != "application/x-pem-file" || body != "PEM" {
		t.Errorf("upstream headers: %v body %q", got.Header, body)
	}
	for _, h := range []string{"Cookie", "X-Csrf-Token", "Origin"} {
		if got.Header.Get(h) != "" {
			t.Errorf("%s reached the API", h)
		}
	}
}

func TestRefusals(t *testing.T) {
	base, _ := url.Parse("http://127.0.0.1:1") // nothing listens
	p := NewProxy(base, slog.New(slog.NewTextHandler(io.Discard, nil)))
	w := httptest.NewRecorder()
	p.Serve(w, httptest.NewRequest("GET", "/api/v1/users", nil), "t")
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "temporarily_unavailable") {
		t.Errorf("down: %d %s", w.Code, w.Body)
	}
	req := httptest.NewRequest("GET", "/api/v1/users", nil)
	req.URL.Path = "/api/v1/../../actuator"
	w = httptest.NewRecorder()
	p.Serve(w, req, "t")
	if w.Code != http.StatusNotFound {
		t.Errorf("dot-dot: %d", w.Code)
	}
}

func TestBodyLimit(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		}
	}))
	defer upstream.Close()
	base, _ := url.Parse(upstream.URL)
	p := NewProxy(base, slog.New(slog.NewTextHandler(io.Discard, nil)))
	w := httptest.NewRecorder()
	p.Serve(w, httptest.NewRequest("POST", "/api/v1/users", strings.NewReader(strings.Repeat("a", maxBody+1))), "t")
	if w.Code < 400 {
		t.Errorf("large body accepted: %d", w.Code)
	}
}
