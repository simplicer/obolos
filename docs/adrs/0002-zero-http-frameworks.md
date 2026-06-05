# 0002 — Zero HTTP frameworks, Go stdlib only

- Status: Accepted
- Date: 2025-06-04

## Context

The service exposes a small HTTP surface and sits on a hot path (called by the
reverse proxy on every API request). Web frameworks and routers add
dependencies, supply-chain surface and upgrade churn for little benefit at this
size. Go 1.22+ added method-aware routing and path values to the standard
`net/http.ServeMux`.

## Decision

Use only the Go standard library for HTTP: `net/http` with `ServeMux`
(`"POST /v1/charges"`, `r.PathValue("user_id")`), `encoding/json` for
serialization and `log/slog` for logging. No third-party HTTP framework,
router or ORM. The only runtime dependency is the PostgreSQL driver
(`jackc/pgx/v5`).

## Consequences

- Minimal dependency tree and attack surface; trivial upgrades.
- Middleware (auth, logging, recovery, body-size limit) is written explicitly
  as `http.Handler` wrappers — more code, but fully transparent.
- The team must not reach for a framework when a handler grows; complexity is
  handled in the application layer, not the transport layer.
