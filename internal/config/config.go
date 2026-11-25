// Package config reads the portal's settings from the environment, once, at start.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// Config is everything the portal needs; a missing required value stops the start.
type Config struct {
	// Listen is the address of the HTTP server (portal, /auth, /api, /health).
	Listen string
	// PublicURL is where browsers reach the portal: https://dss-admin.fikua.com.
	PublicURL *url.URL
	// Issuer is the realm's issuer as it appears in the tokens; the browser goes there to sign in.
	Issuer string
	// InternalIssuer is the same realm reached from the server (token and keys); Issuer when empty.
	InternalIssuer string
	// ClientID is the portal's client in the realm.
	ClientID string
	// AdminAPI is the base URL of Fikua Sign's administration API, without /admin/v1.
	AdminAPI *url.URL
	// SessionIdle and SessionMax bound a session: without requests, and in total.
	SessionIdle time.Duration
	SessionMax  time.Duration
	// SecureCookies is false only on a local machine over plain HTTP.
	SecureCookies bool
}

// FromEnv reads the configuration; the error lists every missing or wrong value.
func FromEnv() (Config, error) {
	return parse(os.Getenv)
}

func parse(get func(string) string) (Config, error) {
	var errs []error
	value := func(name, fallback string) string {
		if v := strings.TrimSpace(get(name)); v != "" {
			return v
		}
		return fallback
	}
	required := func(name string) string {
		v := value(name, "")
		if v == "" {
			errs = append(errs, fmt.Errorf("%s is required", name))
		}
		return v
	}
	absolute := func(name, raw string) *url.URL {
		if raw == "" {
			return nil
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			errs = append(errs, fmt.Errorf("%s must be an absolute URL", name))
			return nil
		}
		return u
	}
	duration := func(name, fallback string) time.Duration {
		d, err := time.ParseDuration(value(name, fallback))
		if err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("%s must be a positive duration", name))
		}
		return d
	}

	c := Config{
		Listen:         value("LISTEN_ADDR", ":8080"),
		Issuer:         strings.TrimSuffix(required("OIDC_ISSUER"), "/"),
		InternalIssuer: strings.TrimSuffix(value("OIDC_INTERNAL_ISSUER", ""), "/"),
		ClientID:       value("OIDC_CLIENT_ID", "fikua-sign-web"),
		SessionIdle:    duration("SESSION_IDLE", "30m"),
		SessionMax:     duration("SESSION_MAX", "8h"),
		SecureCookies:  value("SECURE_COOKIES", "true") != "false",
	}
	c.PublicURL = absolute("PUBLIC_URL", strings.TrimSuffix(required("PUBLIC_URL"), "/"))
	c.AdminAPI = absolute("ADMIN_API_URL", strings.TrimSuffix(required("ADMIN_API_URL"), "/"))
	absolute("OIDC_ISSUER", c.Issuer)
	if c.InternalIssuer == "" {
		c.InternalIssuer = c.Issuer
	}
	absolute("OIDC_INTERNAL_ISSUER", c.InternalIssuer)
	if c.SecureCookies && c.PublicURL != nil && c.PublicURL.Scheme != "https" {
		errs = append(errs, errors.New("PUBLIC_URL must be https unless SECURE_COOKIES=false"))
	}
	return c, errors.Join(errs...)
}

// RedirectURL is where the realm sends the browser back after signing in.
func (c Config) RedirectURL() string {
	return c.PublicURL.String() + "/auth/callback"
}
