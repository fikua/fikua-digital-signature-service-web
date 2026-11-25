// Package api passes the portal's /api/v1 requests to Fikua Sign's
// administration API (/admin/v1) with the administrator's access token.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// Prefix is where the portal serves the administration API.
const Prefix = "/api/v1/"

// maxBody bounds what a request may send: the largest is a certificate chain.
const maxBody = 256 << 10

// Proxy forwards one request; the caller has checked the session.
type Proxy struct {
	rp *httputil.ReverseProxy
}

// NewProxy targets the administration API at base (without /admin/v1).
func NewProxy(base *url.URL, log *slog.Logger) *Proxy {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	rp := &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = base.Scheme
			pr.Out.URL.Host = base.Host
			pr.Out.URL.Path = base.Path + "/admin/v1/" + strings.TrimPrefix(pr.In.URL.Path, Prefix)
			pr.Out.URL.RawPath = ""
			pr.Out.Host = base.Host
			// Only what the API needs: never the browser's cookies nor its own credentials.
			keep := pr.Out.Header.Clone()
			pr.Out.Header = http.Header{}
			for _, h := range []string{"Accept", "Content-Type", "Content-Length", "X-Request-Id"} {
				if v := keep.Values(h); len(v) > 0 {
					pr.Out.Header[h] = v
				}
			}
			if token, ok := pr.In.Context().Value(tokenKey{}).(string); ok {
				pr.Out.Header.Set("Authorization", "Bearer "+token)
			}
		},
		ModifyResponse: func(res *http.Response) error {
			res.Header.Del("Set-Cookie")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.WarnContext(r.Context(), "administration API unreachable", "error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"temporarily_unavailable"}`))
		},
	}
	return &Proxy{rp: rp}
}

type tokenKey struct{}

// Serve forwards r with the access token.
func (p *Proxy) Serve(w http.ResponseWriter, r *http.Request, accessToken string) {
	if strings.Contains(r.URL.Path, "..") {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	p.rp.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), tokenKey{}, accessToken)))
}
