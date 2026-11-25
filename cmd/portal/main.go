// Command portal serves Fikua Sign's administration portal and its backend:
// sign-in with the realm, the session, and the administration API behind it.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fikua/fikua-digital-signature-service-web/internal/api"
	"github.com/fikua/fikua-digital-signature-service-web/internal/auth"
	"github.com/fikua/fikua-digital-signature-service-web/internal/config"
	"github.com/fikua/fikua-digital-signature-service-web/internal/session"
	"github.com/fikua/fikua-digital-signature-service-web/internal/web"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("portal stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	realm, err := auth.NewRealm(ctx, cfg.Issuer, cfg.InternalIssuer, cfg.ClientID, cfg.RedirectURL())
	if err != nil {
		return err
	}
	jwks, err := realm.JWKS()
	if err != nil {
		return err
	}
	store := session.NewStore(cfg.SessionIdle, cfg.SessionMax)
	go store.Sweep(ctx, time.Minute)
	sign := auth.NewHandlers(realm, store, cfg.PublicURL.String(), log)
	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           routes(sign, api.NewProxy(cfg.AdminAPI, log), jwks),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Info("portal listening", "addr", cfg.Listen, "public_url", cfg.PublicURL.String())
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// routes is the whole portal: health, sign-in, the API behind the session, the page.
func routes(sign *auth.Handlers, proxy *api.Proxy, jwks []byte) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/health+json")
		_, _ = w.Write([]byte(`{"status":"pass"}`))
	})
	mux.HandleFunc("GET /auth/jwks", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/jwk-set+json")
		_, _ = w.Write(jwks)
	})
	mux.HandleFunc("GET /auth/login", sign.Login)
	mux.HandleFunc("GET /auth/callback", sign.Callback)
	mux.HandleFunc("GET /auth/session", sign.Session)
	mux.HandleFunc("POST /auth/logout", sign.Logout)
	mux.Handle(api.Prefix, sign.Authorized(proxy.Serve))
	mux.Handle("/", web.Assets())
	return web.Headers(mux)
}
