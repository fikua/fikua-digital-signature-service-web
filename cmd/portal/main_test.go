package main

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/fikua/fikua-digital-signature-service-web/internal/api"
	"github.com/fikua/fikua-digital-signature-service-web/internal/auth"
	"github.com/fikua/fikua-digital-signature-service-web/internal/session"
)

func TestRoutes(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	realm, err := auth.NewRealm(context.Background(), "https://auth.example/realms/fikua", "http://127.0.0.1:1/realms/fikua",
		"fikua-sign-web", "https://dss-admin.example/auth/callback")
	if err != nil {
		t.Fatal(err)
	}
	jwks, _ := realm.JWKS()
	base, _ := url.Parse("http://127.0.0.1:1")
	h := routes(auth.NewHandlers(realm, session.NewStore(time.Minute, time.Hour), "https://dss-admin.example", true, log),
		api.NewProxy(base, log), jwks)

	for _, c := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/health", 200},
		{"GET", "/auth/jwks", 200},
		{"GET", "/auth/login", 302},
		{"GET", "/auth/session", 401},
		{"POST", "/auth/logout", 200},
		{"GET", "/api/v1/users", 401},
		{"POST", "/api/v1/users", 401},
		{"GET", "/", 200},
		{"POST", "/", 405},
		{"POST", "/health", 405},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(c.method, c.path, nil))
		if w.Code != c.status {
			t.Errorf("%s %s: %d, want %d", c.method, c.path, w.Code, c.status)
		}
		if w.Header().Get("Content-Security-Policy") == "" {
			t.Errorf("%s %s without security headers", c.method, c.path)
		}
	}
}
