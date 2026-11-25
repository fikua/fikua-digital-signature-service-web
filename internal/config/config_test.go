package config

import (
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func valid() map[string]string {
	return map[string]string{
		"PUBLIC_URL":           "https://dss-admin.fikua.com/",
		"OIDC_ISSUER":          "https://auth.fikua.com/realms/fikua",
		"OIDC_INTERNAL_ISSUER": "http://keycloak:8080/realms/fikua",
		"ADMIN_API_URL":        "http://fikua-digital-signature-service:9092",
	}
}

func TestDefaults(t *testing.T) {
	c, err := parse(env(valid()))
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != ":8080" || c.ClientID != "fikua-sign-web" || !c.SecureCookies {
		t.Errorf("defaults: %+v", c)
	}
	if c.SessionIdle != 30*time.Minute || c.SessionMax != 8*time.Hour {
		t.Errorf("session bounds: %v %v", c.SessionIdle, c.SessionMax)
	}
	if got := c.RedirectURL(); got != "https://dss-admin.fikua.com/auth/callback" {
		t.Errorf("redirect: %s", got)
	}
}

func TestInternalIssuerDefaultsToIssuer(t *testing.T) {
	m := valid()
	delete(m, "OIDC_INTERNAL_ISSUER")
	c, err := parse(env(m))
	if err != nil {
		t.Fatal(err)
	}
	if c.InternalIssuer != c.Issuer {
		t.Errorf("internal issuer: %s", c.InternalIssuer)
	}
}

func TestEveryMissingValueIsReported(t *testing.T) {
	_, err := parse(env(map[string]string{}))
	for _, name := range []string{"PUBLIC_URL", "OIDC_ISSUER", "ADMIN_API_URL"} {
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s not reported: %v", name, err)
		}
	}
}

func TestRefusesWrongValues(t *testing.T) {
	cases := map[string]map[string]string{
		"plain http with secure cookies": {"PUBLIC_URL": "http://localhost:5173"},
		"relative url":                   {"ADMIN_API_URL": "fikua-digital-signature-service:9092"},
		"duration":                       {"SESSION_IDLE": "soon"},
		"negative duration":              {"SESSION_MAX": "-1h"},
	}
	for name, change := range cases {
		m := valid()
		for k, v := range change {
			m[k] = v
		}
		if _, err := parse(env(m)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestPlainHTTPOnlyWithoutSecureCookies(t *testing.T) {
	m := valid()
	m["PUBLIC_URL"] = "http://localhost:5173"
	m["SECURE_COOKIES"] = "false"
	c, err := parse(env(m))
	if err != nil || c.SecureCookies {
		t.Fatalf("local configuration refused: %v", err)
	}
}
