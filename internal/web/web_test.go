package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	Headers(http.NotFoundHandler()).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	csp := w.Header().Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'none'", "script-src 'self'", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP without %q: %s", want, csp)
		}
	}
	if strings.Contains(csp, "unsafe") {
		t.Errorf("CSP allows unsafe sources: %s", csp)
	}
	for h, v := range map[string]string{"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY", "Referrer-Policy": "no-referrer"} {
		if w.Header().Get(h) != v {
			t.Errorf("%s = %q", h, w.Header().Get(h))
		}
	}
}

func TestAssetsCaching(t *testing.T) {
	h := Assets()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/README.txt", nil))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("file: %d %s", w.Code, w.Header().Get("Cache-Control"))
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /: %d", w.Code)
	}
	if !strings.Contains(cacheControl("/assets/index-3f2a.js"), "immutable") || cacheControl("/") != "no-cache" {
		t.Error("cache of hashed assets")
	}
}
