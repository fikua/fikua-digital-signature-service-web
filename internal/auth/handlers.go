package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fikua/fikua-digital-signature-service-web/internal/session"
)

// CSRFHeader carries the session's CSRF token on every request that changes something.
const CSRFHeader = "X-CSRF-Token"

// refreshBefore renews the access token this long before it expires.
const refreshBefore = 30 * time.Second

// Tokens is what the handlers need from the realm; Realm in production.
type Tokens interface {
	AuthURL(l session.Login) string
	LogoutURL(idToken, back string) string
	Exchange(ctx context.Context, code string, l session.Login) (session.Tokens, Identity, error)
	Refresh(ctx context.Context, old session.Tokens) (session.Tokens, error)
}

// Handlers are the portal's /auth endpoints and the session check of /api.
type Handlers struct {
	realm   Tokens
	store   *session.Store
	public  string // https://dss-admin.fikua.com
	secure  bool
	session string // cookie names
	login   string
	log     *slog.Logger
}

// NewHandlers wires the sign-in. With secure cookies their names carry the
// __Host- prefix: only over HTTPS, for this host and path / only.
func NewHandlers(realm Tokens, store *session.Store, publicURL string, secure bool, log *slog.Logger) *Handlers {
	h := &Handlers{realm: realm, store: store, public: publicURL, secure: secure, log: log,
		session: "fikua-sign", login: "fikua-sign-login"}
	if secure {
		h.session, h.login = "__Host-"+h.session, "__Host-"+h.login
	}
	return h
}

// Login sends the browser to the realm. ?return= is a path of the portal to come back to.
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	l := session.Login{State: session.Random(), Nonce: session.Random(), Verifier: session.Random(),
		Return: safeReturn(r.URL.Query().Get("return"))}
	key := h.store.StartLogin(l)
	// Lax: the realm's redirect back is a top-level navigation from another site.
	h.setCookie(w, h.login, key, http.SameSiteLaxMode, session.LoginTTL)
	noStore(w)
	http.Redirect(w, r, h.realm.AuthURL(l), http.StatusFound)
}

// Callback receives the realm's answer and opens the session.
func (h *Handlers) Callback(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	cookie, err := r.Cookie(h.login)
	h.clearCookie(w, h.login, http.SameSiteLaxMode)
	if err != nil {
		h.fail(w, r, "no sign-in in progress", http.StatusBadRequest)
		return
	}
	l, ok := h.store.TakeLogin(cookie.Value)
	q := r.URL.Query()
	if !ok || q.Get("state") != l.State {
		h.fail(w, r, "unknown or expired sign-in", http.StatusBadRequest)
		return
	}
	if e := q.Get("error"); e != "" {
		h.log.InfoContext(r.Context(), "sign-in not completed", "error", e)
		http.Redirect(w, r, "/#/sense-acces", http.StatusFound)
		return
	}
	tokens, who, err := h.realm.Exchange(r.Context(), q.Get("code"), l)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, ErrRefused) {
			status = http.StatusBadRequest
		}
		h.fail(w, r, err.Error(), status)
		return
	}
	s := h.store.Create(who.Subject, who.Name, tokens)
	h.setCookie(w, h.session, s.ID, http.SameSiteStrictMode, 0)
	h.log.InfoContext(r.Context(), "signed in", "user.id", who.Subject)
	http.Redirect(w, r, l.Return, http.StatusFound)
}

// Session tells the page who is signed in and its CSRF token; 401 when nobody is.
func (h *Handlers) Session(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	s, ok := h.current(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": s.Name, "csrf": s.CSRF})
}

// Logout closes the session here and answers where to end it at the realm.
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	s, ok := h.current(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]string{"logout": "/"})
		return
	}
	if !sameOrigin(r, h.public) || !session.SameCSRF(s, r.Header.Get(CSRFHeader)) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "csrf"})
		return
	}
	h.store.Delete(s.ID)
	h.clearCookie(w, h.session, http.SameSiteStrictMode)
	writeJSON(w, http.StatusOK, map[string]string{"logout": h.realm.LogoutURL(s.Tokens.ID, h.public+"/")})
}

// Authorized lets through requests of a signed-in administrator with a fresh
// access token; those that change something also need the CSRF token and the
// portal's own origin.
func (h *Handlers) Authorized(next func(w http.ResponseWriter, r *http.Request, accessToken string)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		noStore(w)
		s, ok := h.current(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		if !safeMethod(r.Method) && (!sameOrigin(r, h.public) || !session.SameCSRF(s, r.Header.Get(CSRFHeader))) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "csrf"})
			return
		}
		token, err := h.fresh(r.Context(), s)
		if err != nil {
			if errors.Is(err, ErrRefused) {
				h.store.Delete(s.ID)
				h.clearCookie(w, h.session, http.SameSiteStrictMode)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			h.log.WarnContext(r.Context(), "token refresh failed", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "temporarily_unavailable"})
			return
		}
		next(w, r, token)
	})
}

func (h *Handlers) fresh(ctx context.Context, s session.Session) (string, error) {
	if time.Until(s.Tokens.AccessExpiry) > refreshBefore {
		return s.Tokens.Access, nil
	}
	t, err := h.realm.Refresh(ctx, s.Tokens)
	if err != nil {
		return "", err
	}
	h.store.UpdateTokens(s.ID, t)
	return t.Access, nil
}

func (h *Handlers) current(r *http.Request) (session.Session, bool) {
	c, err := r.Cookie(h.session)
	if err != nil || c.Value == "" {
		return session.Session{}, false
	}
	return h.store.Get(c.Value)
}

func (h *Handlers) fail(w http.ResponseWriter, r *http.Request, reason string, status int) {
	h.log.WarnContext(r.Context(), "sign-in failed", "reason", reason)
	http.Error(w, "No s'ha pogut iniciar la sessió. Torna-ho a provar des de l'inici.", status)
}

func (h *Handlers) setCookie(w http.ResponseWriter, name, value string, same http.SameSite, ttl time.Duration) {
	c := &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: h.secure, SameSite: same}
	if ttl > 0 {
		c.MaxAge = int(ttl.Seconds())
	}
	http.SetCookie(w, c)
}

func (h *Handlers) clearCookie(w http.ResponseWriter, name string, same http.SameSite) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		Secure: h.secure, SameSite: same})
}

// safeReturn keeps only paths of this portal: never another host.
func safeReturn(p string) string {
	if p == "" || !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.ContainsAny(p, "\\\r\n") {
		return "/"
	}
	if u, err := url.Parse(p); err != nil || u.Host != "" || u.Scheme != "" {
		return "/"
	}
	return p
}

// sameOrigin: a browser sends Origin on every request that changes something.
func sameOrigin(r *http.Request, public string) bool {
	return r.Header.Get("Origin") == public
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
