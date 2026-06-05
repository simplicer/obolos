# AGENTS.md

Architecture rules for the Metering service. Agents and developers MUST follow these rules.

## Architecture

- **DDD**: Domain-Driven Design with vertical slicing and Screaming Architecture.
- **Bounded Context**: `metering` — the sole context in this project.
- **Slice Location**: `internal/metering/`

## Layer Separation

### Domain (`internal/metering/domain/`)
- Pure business logic only.
- No framework dependencies.
- No infrastructure imports.
- Contains: Balance, Plan, MeteringEvent value objects and entities.
- Tests: colocated `*_test.go` files.

### Application (`internal/metering/application/`)
- Use cases only. One use case = one file.
- No persistence logic.
- No framework logic.
- No HTTP adapters.
- Contains: Meter, GetBalance, GetUsage, AdminTopUp, AdminCreatePlan use cases and their ports.
- Tests: colocated `*_test.go` files.

### Infrastructure (`internal/metering/infrastructure/`)
- HTTP handlers, routers, middleware.
- PostgreSQL store adapter.
- Configuration loader (local JSON + remote HTTP).
- Adapters that implement application ports.
- Tests: colocated `*_test.go` files.

## Dependency Direction

```
Infrastructure → Application → Domain
```

**NEVER** import infrastructure from domain or application.

## Naming Conventions

- Directories: `kebab-case`
- Files: `snake_case.go` (Go convention)
- Types/Interfaces: `PascalCase`
- Tests: `*_test.go` alongside source

## Testing Philosophy

- **Unit**: Every use case tested in isolation with mocked ports.
- **Integration**: PostgreSQL store tested with a real DB in a container.
- **Acceptance**: Full metering flow tested end-to-end via HTTP.
- **E2E**: Full server behavior tested via HTTP requests.

Tests colocated with source (Go convention). No separate `tests/` directory.

## Logging

- Structured JSON logging via `log/slog`.
- Loki-compatible format.
- Every metering event MUST log: `owner_id`, `api`, `endpoint`, `cost`, `balance_after`.
- Logs stored under `var/log/`.

## Configuration

- Configuration files under `etc/`.
- Primary format: JSON (strict schema, typed, validated).
- `etc/config.json` — service configuration (port, log level, DB connection).
- There is **no pricing configuration**: request cost is reported by the
  caller via `X-Cost` (ADR 0008).
- **Config sources** (priority order):
  1. Remote HTTP endpoint (dynamic, hot-reloadable)
  2. Local JSON file (fallback, reloadable on SIGHUP)
  3. Environment variables (overrides)
- Config MUST be reloadable without restart.

## Deployment & infra layout

- Deployment and container artifacts live under `infra/` and MUST use
  vendor-neutral, intent-revealing names (Screaming Architecture): the
  structure screams "deployable service", not the tool that runs it.
  - `infra/image/Containerfile` — OCI build recipe (not `Dockerfile`).
  - `infra/compose.yaml` — runtime stack per the Compose Specification
    (not `docker-compose.yml`).
  - `infra/compose.test.yaml` — full test suite run inside containers.
- Tool/spec-mandated tokens that are not ours (the `docker compose` CLI, the
  Compose `dockerfile:` key, the Postgres image's `/docker-entrypoint-initdb.d`)
  are unavoidable and exempt.
- All container operations are driven through the Makefile (`image-build`,
  `stack-up`, `stack-down`, `test-compose`). See ADR 0007.

## Versioning

- Semantic Versioning: `X.Y.Z`
- Version stored in `VERSION` file.
- Conventional Commits for all changes.

## External Dependencies

- **Allowed** (pragmatic minimum):
  - `jackc/pgx/v5` — PostgreSQL driver
  - `golang.org/x/crypto/acme/autocert` — Let's Encrypt (if needed)
- **Forbidden**:
  - HTTP frameworks (use `net/http` stdlib)
  - ORMs
  - Config frameworks (use `encoding/json` stdlib)
- Go stdlib for: HTTP server, JSON, logging, testing.

## License

- GNU AGPL v3 — same as vatguard.

## Identity

- This service does NOT manage identity.
- It receives `X-User-Id` headers from the reverse proxy (which validated the token against the IdP).
- It trusts the proxy's authentication — no token introspection here.
- The only concern is: "user X called endpoint Y, what does it cost?"

## Billing Models

The domain MUST support three plan types:

1. **Prepaid**: Fixed credit balance. Requests are blocked (HTTP 402) when balance reaches 0.
2. **Unlimited**: No credit deduction. Requests always pass. Usage is logged for analytics only.
3. **Hybrid (Prepaid + Overage)**: Prepaid credits until exhausted, then switches to postpaid billing at a per-unit overage rate. Requests are never blocked — balance can go negative.

Every user has exactly one active plan. Plans define:
- Plan type (prepaid, unlimited, hybrid)
- Included credits (for prepaid/hybrid)
- Entitlement rules (in `plan_config`): time windows — free / multiply / block;
  periodic grant — reset / rollover / rolling (lazy, once per period);
  rate limits (roadmap)

The *price* of a request is NOT a plan concern — it is reported by the caller
(`X-Cost`). The plan governs how the client's credits may be spent, not what
a call costs.

## Cost

This service does not price requests. The trusted caller reports the cost in
credits via the `X-Cost` header on `POST /v1/charges` (ADR 0008). Metering
validates `cost ≥ 0` and enforces the plan's credit rules against it.

## API Surface

- `POST /v1/charges` — Meter a request (called by reverse proxy as forwardAuth)
- `GET /v1/users/{user_id}` — Get current balance (owner or admin)
- `GET /v1/users/{user_id}/charges` — Get usage history (owner or admin)
- `GET /v1/users` — List users (admin)
- `POST /v1/users/{user_id}/credits` — Add credits to a user (admin)
- `PUT /v1/users/{user_id}/plan` — Set plan for a user (admin)
- `GET /health` — Health check
- `GET /version` — Version info

The canonical, machine-readable contract is `docs/openapi.yaml`. Keep it in
sync with the router on every endpoint change (its `info.version` must equal
the `VERSION` file).
