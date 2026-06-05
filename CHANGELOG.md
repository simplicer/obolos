# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Major version 0 is pre-first-release: every build carries the `-unstable`
suffix and is never frozen or deployed to production. The first stable
release will be `1.0.0`.

## [Unreleased]

### Changed
- Added Architecture Decision Records under `docs/adrs/` (DDD vertical slicing,
  stdlib-only HTTP, proxy-trusted identity, signed-integer balance, layered
  JSON config, RESTful resource API).
- Relocated `SPECS.md` to `docs/SPECS.md` and added the OpenAPI 3.1 contract
  at `docs/openapi.yaml`. Reconciled `SPECS.md`, `AGENTS.md` and `README.md`
  with the implemented RESTful surface (`POST /v1/charges`,
  `/v1/users/{id}` resources) which had drifted from the original
  RPC-style draft (`/v1/meter`, `/v1/admin/.../topup`).

### Added
- Acceptance / end-to-end test suite driving the full HTTP stack over a
  loopback server with a real client: full metering journey, authorization
  boundaries (owner / admin / forbidden / unauthorized) and prepaid
  exhaustion returning `402 insufficient_credits`.
- Integration test suite for `PostgresStore` (build tag `integration`),
  runnable against a disposable Postgres container via `make test-integration`
  (`bin/run-integration-tests`).
- Project foundation: `go.mod`, `Makefile`, `LICENSE` (AGPL v3), `README.md`,
  `AGENTS.md`, directory layout (`cmd/`, `internal/`, `etc/`, `infra/`,
  `migrations/`, `var/log/`).
- Domain layer (`internal/metering/domain`): `Balance` value object
  (signed-integer credits), `Plan` (prepaid / unlimited / hybrid),
  `PricingRule` with time modifiers, `MeteringEvent` entity, `User` aggregate.
- Application layer (`internal/metering/application`): use cases `Meter`,
  `GetBalance`, `GetUsage`, `TopUp`, `SetPlan`, `GetAdminUsage`, `ListUsers`,
  driven through `ports.go` interfaces (`UserRepository`, `EventRepository`,
  `Meterer`, `PricingConfig`) with typed application errors.
- Pricing engine (`internal/metering/infrastructure/pricing_engine.go`):
  resolves per-API/per-endpoint cost with time modifiers; pricing and
  service config loaders (`pricing_loader.go`, `config_loader.go`) support
  local JSON files, remote HTTP sources with TTL refresh, and env overrides.
- HTTP infrastructure (`internal/metering/infrastructure/http_handler.go`,
  `router.go`): stdlib `net/http` handlers for all endpoints with RFC 7807
  problem+json errors and a `{ "data": ... }` envelope; Go 1.22+ ServeMux
  routing with auth, logging, recovery and request-size middleware.
- PostgreSQL persistence (`internal/metering/infrastructure/postgres_store.go`):
  `pgx/v5`-backed `UserRepository` and `EventRepository`, including atomic
  meter-and-deduct, with schema migration `migrations/001_initial_schema.sql`.
- Server entry point (`cmd/server/main.go`): loads configuration, wires the
  PostgreSQL store, pricing engine and use cases into the HTTP router with
  structured `slog` JSON logging, and runs the server with graceful shutdown.

### Removed
- `PROMPT.md` (the temporary development guide), now that the implementation
  is complete and documented under `docs/`.

### Fixed
- `make migrate-up` now applies `migrations/001_initial_schema.sql` via `psql`
  against `DATABASE_URL` instead of an invalid `go run` invocation.
- `PostgresStore` now scans `created_at` / `updated_at` (timestamptz) into
  `time.Time` instead of `string` in `FindByID`, `List` and `MeterAtomic`.
  The previous code failed at runtime against a real database
  (`cannot scan timestamptz ... into *string`); revealed by the new
  integration suite.
- Startup/shutdown logs now identify the service as `metering` instead of
  the copy-pasted `tollgate` name.
- `MeteringEvent` now serializes with `snake_case` JSON keys
  (`base_cost`, `final_cost`, `created_at`, …), consistent with every other
  response body, instead of Go's default PascalCase field names.

[Unreleased]: https://codeberg.org/simplicer/metering/compare/main
