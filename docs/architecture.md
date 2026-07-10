# Architecture

`it-tools-portal` is structured as a Statora-style monorepo with one deployable binary.

## Runtime Shape

The frontend is built from `apps/web` with Vite. The build output is copied into `internal/handlers/web/dist`, where Go embeds it into the server binary. Gin serves health endpoints and falls back to the embedded `index.html` for frontend routes.

## Local Stack

Docker Compose starts two services:

- `app`: the Go binary containing the embedded frontend
- `mongodb`: local MongoDB for later registry/audit-log work

## Future FRE-30 Flow

Later issues will add Authentik OIDC login, MongoDB-backed tool registry, token exchange, JWKS publication, audit logging, and launch-in-new-tab behavior. Target tools are launched in new tabs and are never embedded inside this app.
