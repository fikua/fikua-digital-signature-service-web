package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/fikua/fikua-digital-signature-service-web/internal/session"
)

const clientID = "fikua-sign-web"

// fakeRealm is a Keycloak realm as the portal sees it: a token endpoint that
// checks the client's assertion against the client's JWKS, and its own keys.
type fakeRealm struct {
	t        *testing.T
	srv      *httptest.Server
	issuer   string
	key      *ecdsa.PrivateKey
	client   func() []byte // the portal's JWKS
	mu       sync.Mutex
	forms    []url.Values
	nonce    string
	verifier string
	refuse   bool
	jtis     map[string]bool
}

func newFakeRealm(t *testing.T) *fakeRealm {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeRealm{t: t, key: key, jtis: map[string]bool{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /protocol/openid-connect/certs", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "realm", Algorithm: "ES256", Use: "sig"}}})
	})
	mux.HandleFunc("POST /protocol/openid-connect/token", f.token)
	f.srv = httptest.NewServer(mux)
	f.issuer = "https://auth.example/realms/fikua" // tokens name the public issuer
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeRealm) token(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	f.mu.Lock()
	f.forms = append(f.forms, r.PostForm)
	f.mu.Unlock()
	if err := f.checkAssertion(r.PostForm); err != nil || f.refuse {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		return
	}
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		if r.PostForm.Get("code_verifier") != f.verifier {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		f.answer(w, f.idToken("admin-1", f.nonce))
	case "refresh_token":
		f.answer(w, "")
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

func (f *fakeRealm) checkAssertion(form url.Values) error {
	if form.Get("client_assertion_type") != "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" ||
		form.Get("client_id") != clientID || form.Get("client_secret") != "" {
		return errors.New("client authentication")
	}
	tok, err := jwt.ParseSigned(form.Get("client_assertion"), []jose.SignatureAlgorithm{jose.ES256})
	if err != nil {
		return err
	}
	var set jose.JSONWebKeySet
	if err := json.Unmarshal(f.client(), &set); err != nil {
		return err
	}
	var c jwt.Claims
	if err := tok.Claims(set.Keys[0].Key, &c); err != nil {
		return err
	}
	if err := c.Validate(jwt.Expected{Issuer: clientID, Subject: clientID, AnyAudience: jwt.Audience{f.issuer}, Time: time.Now()}); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if c.ID == "" || f.jtis[c.ID] {
		return errors.New("assertion replayed")
	}
	f.jtis[c.ID] = true
	return nil
}

func (f *fakeRealm) idToken(sub, nonce string) string {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: jose.JSONWebKey{Key: f.key, KeyID: "realm"}}, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	now := time.Now()
	s, err := jwt.Signed(signer).Claims(jwt.Claims{Issuer: f.issuer, Subject: sub, Audience: jwt.Audience{clientID},
		IssuedAt: jwt.NewNumericDate(now), Expiry: jwt.NewNumericDate(now.Add(5 * time.Minute))}).
		Claims(map[string]any{"nonce": nonce, "name": "Ada Lovelace"}).Serialize()
	if err != nil {
		f.t.Fatal(err)
	}
	return s
}

func (f *fakeRealm) answer(w http.ResponseWriter, idToken string) {
	a := map[string]any{"access_token": "access-" + session.Random()[:8], "expires_in": 300,
		"refresh_token": "refresh-" + session.Random()[:8], "refresh_expires_in": 1800}
	if idToken != "" {
		a["id_token"] = idToken
	}
	_ = json.NewEncoder(w).Encode(a)
}

func newRealm(t *testing.T) (*Realm, *fakeRealm) {
	f := newFakeRealm(t)
	r, err := NewRealm(context.Background(), f.issuer, f.srv.URL, clientID, "https://dss-admin.example/auth/callback")
	if err != nil {
		t.Fatal(err)
	}
	f.client = func() []byte { b, _ := r.JWKS(); return b }
	return r, f
}

func TestAuthURL(t *testing.T) {
	r, f := newRealm(t)
	l := session.Login{State: "st", Nonce: "no", Verifier: "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"}
	u, err := url.Parse(r.AuthURL(l))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if !strings.HasPrefix(u.String(), f.issuer+"/protocol/openid-connect/auth?") {
		t.Errorf("not the public issuer: %s", u)
	}
	// RFC 7636, appendix B.
	if q.Get("code_challenge") != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" || q.Get("code_challenge_method") != "S256" {
		t.Errorf("PKCE: %v", q)
	}
	for k, v := range map[string]string{"client_id": clientID, "response_type": "code", "scope": "openid",
		"state": "st", "nonce": "no", "redirect_uri": "https://dss-admin.example/auth/callback"} {
		if q.Get(k) != v {
			t.Errorf("%s = %q", k, q.Get(k))
		}
	}
}

func TestExchangeAndRefresh(t *testing.T) {
	r, f := newRealm(t)
	f.nonce, f.verifier = "n-1", "v-1"
	tokens, who, err := r.Exchange(context.Background(), "code", session.Login{Nonce: "n-1", Verifier: "v-1"})
	if err != nil {
		t.Fatal(err)
	}
	if who.Subject != "admin-1" || who.Name != "Ada Lovelace" || tokens.ID == "" || tokens.Refresh == "" {
		t.Errorf("exchange: %+v %+v", who, tokens)
	}
	if d := time.Until(tokens.AccessExpiry); d < 4*time.Minute || d > 5*time.Minute {
		t.Errorf("access expiry in %v", d)
	}
	if f.forms[0].Get("redirect_uri") != "https://dss-admin.example/auth/callback" {
		t.Errorf("redirect_uri: %v", f.forms[0])
	}

	again, err := r.Refresh(context.Background(), tokens)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != tokens.ID || again.Access == tokens.Access || f.forms[1].Get("refresh_token") != tokens.Refresh {
		t.Errorf("refresh: %+v", again)
	}
}

func TestExchangeRefusals(t *testing.T) {
	r, f := newRealm(t)
	f.nonce, f.verifier = "n-1", "v-1"
	cases := map[string]session.Login{
		"other nonce":    {Nonce: "n-2", Verifier: "v-1"},
		"other verifier": {Nonce: "n-1", Verifier: "v-2"},
	}
	for name, l := range cases {
		if _, _, err := r.Exchange(context.Background(), "code", l); !errors.Is(err, ErrRefused) {
			t.Errorf("%s: %v", name, err)
		}
	}
	f.nonce = ""
	if _, _, err := r.Exchange(context.Background(), "code", session.Login{Verifier: "v-1"}); !errors.Is(err, ErrRefused) {
		t.Errorf("no nonce: %v", err)
	}
	f.refuse = true
	if _, err := r.Refresh(context.Background(), session.Tokens{Refresh: "x"}); !errors.Is(err, ErrRefused) {
		t.Errorf("refused refresh: %v", err)
	}
}

func TestIDTokenOfAnotherIssuer(t *testing.T) {
	r, f := newRealm(t)
	f.nonce, f.verifier = "n", "v"
	f.issuer = "https://evil.example/realms/fikua" // what the fake signs; the assertion now fails too
	if _, _, err := r.Exchange(context.Background(), "code", session.Login{Nonce: "n", Verifier: "v"}); err == nil {
		t.Error("token of another issuer accepted")
	}
}

func TestRealmDown(t *testing.T) {
	r, f := newRealm(t)
	f.srv.Close()
	_, err := r.Refresh(context.Background(), session.Tokens{Refresh: "x"})
	if err == nil || errors.Is(err, ErrRefused) {
		t.Errorf("an outage is not a refusal: %v", err)
	}
}

func TestLogoutURL(t *testing.T) {
	r, f := newRealm(t)
	u, _ := url.Parse(r.LogoutURL("id-token", "https://dss-admin.example/"))
	if !strings.HasPrefix(u.String(), f.issuer+"/protocol/openid-connect/logout?") ||
		u.Query().Get("id_token_hint") != "id-token" || u.Query().Get("post_logout_redirect_uri") != "https://dss-admin.example/" {
		t.Errorf("logout: %s", u)
	}
}

func TestJWKSHasOnlyThePublicKey(t *testing.T) {
	r, _ := newRealm(t)
	b, _ := r.JWKS()
	if strings.Contains(string(b), `"d"`) {
		t.Fatal("private key in the JWKS")
	}
	var set jose.JSONWebKeySet
	if err := json.Unmarshal(b, &set); err != nil || len(set.Keys) != 1 || set.Keys[0].KeyID == "" || set.Keys[0].Algorithm != "ES256" {
		t.Errorf("jwks: %s", b)
	}
}
