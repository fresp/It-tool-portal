# Architecture

`it-tools-portal` is a standalone SSO app launcher built as a Statora-style monorepo. A single Go binary serves both the API and the embedded React frontend.

## Runtime Shape

```
it-tools-portal/
├── apps/web/                  # React 18 + TypeScript + Vite SPA
├── cmd/server/main.go         # Go entrypoint
├── internal/
│   ├── handlers/              # HTTP handlers (Gin)
│   │   ├── auth.go            # OIDC login/callback/logout
│   │   ├── exchange.go        # Token exchange endpoint
│   │   ├── jwks.go            # JWKS publication endpoint
│   │   ├── tools.go           # Tool registry CRUD
│   │   ├── router.go          # Route registration + frontend embedding
│   │   └── web/dist/          # go:embed target for built frontend
│   ├── middleware/
│   │   ├── auth.go            # JWT session validation + OIDC flow helpers
│   │   ├── jwks.go            # Authentik JWKS fetcher with cache
│   │   └── ratelimit.go       # Per-user rate limiter
│   ├── models/                # Domain models (Tool, AuditLog)
│   ├── repositories/          # MongoDB access layer
│   └── services/
│       └── token_signer.go    # Tool-scoped JWT minting
├── Dockerfile                 # Multi-stage: Vite build → go:embed
├── docker-compose.yml         # App + MongoDB
└── Makefile                   # build, test, docker targets
```

The frontend is built from `apps/web` with Vite. The build output is copied into `internal/handlers/web/dist`, where Go embeds it into the server binary via `go:embed`. Gin serves the API and falls back to the embedded `index.html` for all non-API frontend routes. One origin, no CORS.

## Auth Model — Standalone OIDC Login

This app runs its **own full OIDC Authorization Code flow** against Authentik. It does not share cookies with any other app (Admin, target tools, etc.) because those apps may live on different root domains where cookies cannot be shared.

### Login Flow

```
1. Browser loads it-tools-portal (any domain: localhost, staging, production)
2. Frontend calls GET /api/tools
3. If no valid session cookie → middleware returns 401 for API routes
4. Frontend redirects to Authentik's authorize endpoint:
   GET {AUTHENTIK_AUTHORIZE_URL}?response_type=code&client_id=...&redirect_uri=...&scope=openid+profile+email&state=...
5. User authenticates at Authentik (2FA if configured)
6. Authentik redirects back to GET /callback?code=...&state=...
7. Server validates state (CSRF protection), exchanges code for tokens at Authentik's token endpoint
8. Server validates the id_token against Authentik's JWKS
9. Server sets a first-party session cookie (the Authentik id_token itself, JWT-signed by Authentik)
10. Redirect to / — frontend reloads, now authenticated
```

### Why Not a Shared Cookie

The Admin dashboard, this app, and target tools may live on genuinely different root domains (e.g. `ocaindonesia.co.id`, `fresp.my.id`, `localhost`). Cookies cannot be shared across different root domains regardless of `SameSite`/`Domain` tuning — that only works for subdomains under the same root. Each app maintains its own first-party session.

### Session Validation

The auth middleware (`internal/middleware/auth.go`) validates every protected request:

1. Reads the `it_tools_session` cookie (the Authentik id_token JWT)
2. Fetches Authentik's JWKS from the configured URL (cached with configurable TTL)
3. Validates the JWT signature, issuer, and expiry
4. Extracts claims: `sub`, `email`, `name`, `groups`
5. Sets claims on the Gin context for downstream handlers

If the session is missing or invalid:
- API routes (`/api/*`) → 401 JSON response
- Page routes → 302 redirect to Authentik's authorize endpoint

## Token Exchange

When a user clicks a tool tile, the frontend calls `POST /api/auth/exchange` to get a short-lived, tool-scoped SSO token.

### Exchange Flow

```
1. Frontend sends POST /api/auth/exchange { tool_id: "..." }
2. Middleware validates the session cookie (Authentik JWT)
3. Handler looks up the tool in MongoDB
4. Checks: tool exists, tool is active, user's groups overlap tool's allowed_groups
5. Live revocation check: calls Authentik's /userinfo/ endpoint with the session token
   - If userinfo returns non-200 → user is revoked → 403
6. Mints a new tool-scoped JWT signed with this app's RSA private key:
   - Claims: sub, email, name, role (user/admin), aud (tool_id), iat, exp (90s), jti (unique UUID)
   - Algorithm: RS256
7. Returns { launch_url: "https://{tool_base_url}/sso-callback?token={jwt}" }
8. Frontend opens the launch_url in a new tab (window.open, target="_blank")
```

### Revocation Check

Every exchange request performs a **live** call to Authentik's userinfo endpoint. There is no local denylist or cache for revocation — simplicity over latency. If the userinfo endpoint returns anything other than 200, the exchange is denied.

### Token Properties

| Property | Value |
|----------|-------|
| Algorithm | RS256 |
| Expiry | 90 seconds (configurable via `JWT_EXPIRY_SECONDS`) |
| JTI | Unique UUID per token (prevents replay) |
| Audience | Target tool's ID |
| Signing key | This app's RSA private key (from `JWT_PRIVATE_KEY_PATH` or ephemeral for dev) |

## JWKS Publication

`GET /.well-known/jwks.json` publishes this app's public key(s) so target tools can verify minted tokens without manual key distribution.

The endpoint returns a standard JWK Set with:
- The public key corresponding to this app's signing key
- `kid` (key ID) matching the one used in token headers
- `alg: RS256`, `use: sig`

Key rotation is supported: the `TokenSigner` can hold multiple keys (different `kid` values), and the JWKS endpoint publishes all active public keys. Tokens signed with an old `kid` remain verifiable until the old key is removed.

## Embeddability

This app is designed to be embedded in Admin's iframe (or any other authorized parent).

### CSP frame-ancestors

The `frameAncestorsMiddleware()` in `router.go` sets `Content-Security-Policy: frame-ancestors {FRAME_ANCESTORS}` on every response. The `FRAME_ANCESTORS` env var lists every domain expected to embed this app.

There is no `X-Frame-Options: DENY` — the CSP directive is the modern replacement and provides finer-grained control.

### Cookie Scope

The session cookie (`it_tools_session`) is scoped to this app's own origin. Since the OIDC login flow happens within this app's own context (not a cross-domain shared cookie), there's no need for `SameSite=None` or cross-domain cookie configuration.

### Iframe OIDC Redirect

If the login redirect happens inside a third-party iframe context, some browsers restrict cookie behavior during the redirect chain. This should be tested specifically in Safari (ITP) and Chrome. The redirect is a full navigation within the iframe's own context — the resulting session cookie is first-party to this app's origin.

## Rate Limiting

`POST /api/auth/exchange` is rate-limited per user (10 requests/minute by default, configurable via `RATE_LIMIT_EXCHANGE_PER_MINUTE`). The rate limiter is an in-memory sliding window keyed by user ID.

## Audit Logging

Every token exchange attempt (success or failure) is recorded in the `audit_logs` MongoDB collection:

| Field | Description |
|-------|-------------|
| `user_id` | The caller's subject ID |
| `tool_id` | The target tool ID |
| `ip` | Client IP address |
| `user_agent` | Client user agent |
| `result` | `"success"` or `"denied"` |
| `reason` | Denial reason (e.g. `"group mismatch"`, `"revoked"`, `"tool inactive"`) |
| `timestamp` | UTC timestamp |

Audit logs are indexed by `user_id`, `tool_id`, and `timestamp` for efficient querying.

## Tool Registry

MongoDB-backed registry in the `tools` collection. Tools have: `id, name, base_url, icon_url, allowed_groups[], health_check_url, is_active, created_at, updated_at`.

- `GET /api/tools` returns tools filtered by the caller's groups (from session claims) and `is_active: true`
- Admin CRUD endpoints (`/api/admin/tools/*`) require the `admin` group

## Sequence Diagram: Tile Click to Target-Tool Session

```
Browser                    it-tools-portal              Authentik           Target Tool
  |                             |                          |                    |
  |-- POST /api/auth/exchange ->|                          |                    |
  |   (with session cookie)     |                          |                    |
  |                             |-- validate JWT (JWKS) -->|                    |
  |                             |<--- public keys ---------|                    |
  |                             |                          |                    |
  |                             |-- GET /userinfo/ ------->|                    |
  |                             |   (Bearer session token) |                    |
  |                             |<--- 200 (not revoked) ---|                    |
  |                             |                          |                    |
  |                             |-- check groups overlap   |                    |
  |                             |-- mint tool-scoped JWT   |                    |
  |                             |   (RS256, 90s, unique jti)                    |
  |                             |                          |                    |
  |<-- { launch_url } ----------|                          |                    |
  |                             |                          |                    |
  |-- window.open(launch_url) ------------------------------------------------>|
  |                             |                          |    /sso-callback?token=...
  |                             |                          |                    |
  |                             |                          |    target tool validates
  |                             |                          |    token against
  |                             |                          |    /.well-known/jwks.json
  |                             |                          |                    |
  |                             |                          |    creates local session
  |                             |                          |    redirects to tool UI
```

## Environment Configuration

All environment-specific values are driven via `.env`:

| Variable | Description | Default |
|----------|-------------|---------|
| `AUTHENTIK_ISSUER_URL` | Authentik OIDC issuer URL | (required) |
| `AUTHENTIK_JWKS_URL` | Authentik JWKS endpoint | (required) |
| `AUTHENTIK_AUTHORIZE_URL` | Authentik authorize endpoint | (required) |
| `AUTHENTIK_TOKEN_URL` | Authentik token endpoint | (required) |
| `AUTHENTIK_CLIENT_ID` | OIDC client ID | (required) |
| `AUTHENTIK_CLIENT_SECRET` | OIDC client secret | (required) |
| `AUTHENTIK_REDIRECT_URI` | Callback URL for this app | (required) |
| `JWKS_CACHE_TTL_SECONDS` | JWKS cache TTL | 3600 |
| `JWT_PRIVATE_KEY_PATH` | Path to RSA private key file | (ephemeral for dev) |
| `JWT_KID` | Key ID for minted tokens | v1 |
| `JWT_EXPIRY_SECONDS` | Token expiry in seconds | 90 |
| `FRAME_ANCESTORS` | CSP frame-ancestors directive | (none) |
| `RATE_LIMIT_EXCHANGE_PER_MINUTE` | Exchange rate limit | 10 |
| `MONGODB_URI` | MongoDB connection string | (required) |
| `MONGODB_DB` | Database name | it_tools_portal |
| `PORT` | Server port | 8080 |

## Local Development

Docker Compose starts two services:
- `app`: the Go binary with embedded frontend
- `mongodb`: local MongoDB

```bash
make build          # Build frontend + Go binary
make test           # Build frontend + run all Go tests
make docker-up      # Start app + MongoDB
make docker-down    # Stop services
```

For local OIDC testing, register `http://localhost:8080/callback` as an additional allowed redirect URI on the Authentik client config.
