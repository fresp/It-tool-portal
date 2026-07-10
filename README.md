# IT Tools Portal

FRE-31 scaffold for a standalone SSO app launcher. The repository is a single deployable artifact: a Go Gin server serves API routes and the embedded React/Vite/Tailwind frontend from one binary.

## Structure

```text
apps/web/                  React 18 + TypeScript + Vite + Tailwind CSS
cmd/server/                Go entrypoint
internal/handlers/         HTTP router and embedded frontend assets
internal/services/         Future business logic
internal/repositories/     Future MongoDB repositories
internal/middleware/       Future Authentik/session middleware
configs/                   Future runtime config templates
docs/                      Architecture notes
scripts/                   Future automation scripts
```

## Local Development

```bash
cp .env.example .env
make build
make run
```

The app listens on `http://localhost:8080` by default.

## Verification

```bash
make test
make build
docker compose up --build
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/
```

Use `make docker-down` to stop Compose services.

## Current Scope

This scaffold intentionally contains no tool registry, OIDC, token exchange, or MongoDB business logic yet. Later FRE-30 child issues will fill those layers.
