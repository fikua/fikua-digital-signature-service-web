# Fikua Sign · administration portal

Portal to manage the users, credentials and certificates of [Fikua Sign](https://github.com/fikua/fikua-digital-signature-service) (fikua-digital-signature-service#11), for Fikua's team. `prototype/` keeps the navigable mockup it started from.

## How it works

One Go binary serves the page (HTML, CSS and JavaScript built by Vite, embedded) and is its backend (BFF):

```
browser ──cookie──▶ portal (Go) ──Bearer──▶ Fikua Sign /admin/v1 (port 9092, not routed)
                       │
                       └── Keycloak, realm fikua: code + PKCE, private_key_jwt
```

- **Sign-in**: authorization code with PKCE (S256), `state` and `nonce`, client `fikua-sign-web` of the realm `fikua`. The portal authenticates to the realm with a key it makes at start (`private_key_jwt`, ES256): there is no shared secret. Keycloak reads the public key from `/auth/jwks`. TOTP and the group `dss-admins` are the realm's.
- **Session**: on the server, in memory; the browser only keeps an `HttpOnly`, `Secure`, `SameSite=Strict` cookie with the `__Host-` prefix. The tokens never reach the browser. 30 minutes idle, 8 hours at most; a restart signs everyone out.
- **API**: `/api/v1/*` goes to `/admin/v1/*` of Fikua Sign with the administrator's access token, renewed with the refresh token. Requests that change something also need the session's CSRF token (`X-CSRF-Token`) and the portal's `Origin`. Only `Accept`, `Content-Type` and `Content-Length` pass; never the browser's cookies.
- **Page**: CSP without inline script or style and no other origin; fonts self-hosted.

Fikua Sign checks the token again (issuer, audience `https://dss-admin.fikua.com`, group `dss-admins`) and audits every operation.

## Configuration

| Variable | Default | |
|---|---|---|
| `PUBLIC_URL` | | Where browsers reach the portal: `https://dss-admin.fikua.com` |
| `OIDC_ISSUER` | | The realm, as in its tokens: `https://auth.fikua.com/realms/fikua` |
| `OIDC_INTERNAL_ISSUER` | `OIDC_ISSUER` | The same realm from the server (token and keys): `http://keycloak:8080/realms/fikua` |
| `OIDC_CLIENT_ID` | `fikua-sign-web` | |
| `ADMIN_API_URL` | | Fikua Sign's administration port: `http://fikua-digital-signature-service:9092` |
| `SESSION_IDLE`, `SESSION_MAX` | `30m`, `8h` | |
| `SECURE_COOKIES` | `true` | `false` only on a local machine over plain HTTP |
| `LISTEN_ADDR` | `:8080` | |

The client's `jwks.url` in Keycloak must reach `/auth/jwks` (in production `http://fikua-sign-web:8080/auth/jwks`, on `traefik-public`).

## Development

```bash
pre-commit install        # blocks commits with secrets
make test                 # Go tests (with -race) and the page's helpers (node --test)
make web                  # builds the page into internal/web/dist
make image                # the production image
```

Locally, with a Keycloak with the realms of `fikua-platform-iac/tofu/keycloak` and a Fikua Sign with its administration API:

```bash
go run ./cmd/portal       # PUBLIC_URL=http://localhost:5173 SECURE_COOKIES=false OIDC_ISSUER=… ADMIN_API_URL=…
cd web && npm run dev     # http://localhost:5173, /auth and /api go to :8080
```

## Image

`ghcr.io/fikua/fikua-digital-signature-service-web`, published by CI on every push to `main` and on releases. Deployed from `fikua-platform-iac/projects/fikua-digital-signature-service-web`.

## License

Proprietary software, source available: all rights reserved. Publishing it does not grant permission to copy, modify, run or distribute it. See [LICENSE](LICENSE). Contact: hola@fikua.com
