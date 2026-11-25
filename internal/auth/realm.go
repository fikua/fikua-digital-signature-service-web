// Package auth signs administrators in with the Keycloak realm (authorization
// code with PKCE) and keeps their tokens fresh. The portal authenticates to the
// realm with a key of its own (private_key_jwt): there is no shared secret.
package auth

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/fikua/fikua-digital-signature-service-web/internal/session"
)

// Realm is the portal's client in one realm.
type Realm struct {
	issuer   string // in the tokens; the browser's side
	internal string // the same realm from the server
	clientID string
	redirect string
	http     *http.Client
	key      *ecdsa.PrivateKey
	keyID    string
	verifier *oidc.IDTokenVerifier
	now      func() time.Time
}

// ErrRefused is the realm saying no: a code, a refresh token or the client is not valid.
var ErrRefused = errors.New("refused by the realm")

// NewRealm makes the client with a new signing key: a restart makes another one,
// which the realm reads from JWKS on the next request.
func NewRealm(ctx context.Context, issuer, internal, clientID, redirect string) (*Realm, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("signing key: %w", err)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	keys := oidc.NewRemoteKeySet(oidc.ClientContext(ctx, client), internal+"/protocol/openid-connect/certs")
	return &Realm{
		issuer: issuer, internal: internal, clientID: clientID, redirect: redirect, http: client,
		key: key, keyID: thumbprint(&key.PublicKey),
		verifier: oidc.NewVerifier(issuer, keys, &oidc.Config{
			ClientID:             clientID,
			SupportedSigningAlgs: []string{oidc.RS256, oidc.ES256},
		}),
		now: time.Now,
	}, nil
}

// JWKS is the public half of the portal's key, for the realm.
func (r *Realm) JWKS() ([]byte, error) {
	return json.Marshal(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key: &r.key.PublicKey, KeyID: r.keyID, Algorithm: string(jose.ES256), Use: "sig",
	}}})
}

// AuthURL is the realm's sign-in page for one login: PKCE S256, state and nonce.
func (r *Realm) AuthURL(l session.Login) string {
	q := url.Values{
		"client_id":             {r.clientID},
		"response_type":         {"code"},
		"scope":                 {"openid"},
		"redirect_uri":          {r.redirect},
		"state":                 {l.State},
		"nonce":                 {l.Nonce},
		"code_challenge":        {challenge(l.Verifier)},
		"code_challenge_method": {"S256"},
	}
	return r.issuer + "/protocol/openid-connect/auth?" + q.Encode()
}

// LogoutURL ends the session at the realm too, and comes back to the portal.
func (r *Realm) LogoutURL(idToken, back string) string {
	q := url.Values{"client_id": {r.clientID}, "post_logout_redirect_uri": {back}}
	if idToken != "" {
		q.Set("id_token_hint", idToken)
	}
	return r.issuer + "/protocol/openid-connect/logout?" + q.Encode()
}

// Identity is who signed in, from a verified ID token.
type Identity struct {
	Subject string
	Name    string
}

// Exchange turns the code into tokens and checks the ID token and its nonce.
func (r *Realm) Exchange(ctx context.Context, code string, l session.Login) (session.Tokens, Identity, error) {
	t, err := r.token(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {r.redirect},
		"code_verifier": {l.Verifier},
	})
	if err != nil {
		return session.Tokens{}, Identity{}, err
	}
	id, err := r.verifier.Verify(ctx, t.ID)
	if err != nil {
		return session.Tokens{}, Identity{}, fmt.Errorf("%w: id token: %v", ErrRefused, err)
	}
	if id.Nonce == "" || id.Nonce != l.Nonce {
		return session.Tokens{}, Identity{}, fmt.Errorf("%w: nonce", ErrRefused)
	}
	var claims struct {
		Name string `json:"name"`
	}
	if err := id.Claims(&claims); err != nil {
		return session.Tokens{}, Identity{}, fmt.Errorf("id token claims: %w", err)
	}
	return t, Identity{Subject: id.Subject, Name: claims.Name}, nil
}

// Refresh gets new tokens with the refresh token; the ID token is kept when none comes.
func (r *Realm) Refresh(ctx context.Context, old session.Tokens) (session.Tokens, error) {
	t, err := r.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {old.Refresh}})
	if err != nil {
		return session.Tokens{}, err
	}
	if t.ID == "" {
		t.ID = old.ID
	}
	if t.Refresh == "" {
		t.Refresh, t.RefreshExpiry = old.Refresh, old.RefreshExpiry
	}
	return t, nil
}

type tokenAnswer struct {
	AccessToken      string `json:"access_token"`
	ExpiresIn        int64  `json:"expires_in"`
	RefreshToken     string `json:"refresh_token"`
	RefreshExpiresIn int64  `json:"refresh_expires_in"`
	IDToken          string `json:"id_token"`
	Error            string `json:"error"`
}

func (r *Realm) token(ctx context.Context, form url.Values) (session.Tokens, error) {
	assertion, err := r.assertion()
	if err != nil {
		return session.Tokens{}, err
	}
	form.Set("client_id", r.clientID)
	form.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer")
	form.Set("client_assertion", assertion)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.internal+"/protocol/openid-connect/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return session.Tokens{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := r.http.Do(req)
	if err != nil {
		return session.Tokens{}, fmt.Errorf("token endpoint: %w", err)
	}
	defer res.Body.Close()
	var a tokenAnswer
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&a); err != nil {
		return session.Tokens{}, fmt.Errorf("token endpoint answered %d: %w", res.StatusCode, err)
	}
	switch {
	case res.StatusCode == http.StatusBadRequest || res.StatusCode == http.StatusUnauthorized:
		return session.Tokens{}, fmt.Errorf("%w: %s", ErrRefused, a.Error)
	case res.StatusCode != http.StatusOK || a.AccessToken == "":
		return session.Tokens{}, fmt.Errorf("token endpoint answered %d", res.StatusCode)
	}
	now := r.now()
	return session.Tokens{
		Access:        a.AccessToken,
		AccessExpiry:  now.Add(time.Duration(a.ExpiresIn) * time.Second),
		Refresh:       a.RefreshToken,
		RefreshExpiry: now.Add(time.Duration(a.RefreshExpiresIn) * time.Second),
		ID:            a.IDToken,
	}, nil
}

// assertion is the portal's proof of identity for one token request (RFC 7523).
func (r *Realm) assertion() (string, error) {
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.ES256, Key: jose.JSONWebKey{Key: r.key, KeyID: r.keyID}},
		(&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		return "", fmt.Errorf("signer: %w", err)
	}
	now := r.now()
	return jwt.Signed(signer).Claims(jwt.Claims{
		Issuer:   r.clientID,
		Subject:  r.clientID,
		Audience: jwt.Audience{r.issuer},
		ID:       session.Random(),
		IssuedAt: jwt.NewNumericDate(now),
		Expiry:   jwt.NewNumericDate(now.Add(time.Minute)),
	}).Serialize()
}

func challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func thumbprint(pub *ecdsa.PublicKey) string {
	t, err := (&jose.JSONWebKey{Key: pub}).Thumbprint(crypto.SHA256)
	if err != nil {
		panic(err) // a P-256 public key always has a thumbprint
	}
	return base64.RawURLEncoding.EncodeToString(t)
}
