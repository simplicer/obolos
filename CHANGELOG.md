# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Major version 0 is pre-first-release: every build carries the `-unstable`
suffix and is never frozen or deployed to production. The first stable
release will be `1.0.0`.

## [Unreleased]

### Changed
- **BREAKING**: request cost is now supplied by the trusted caller via the
  `X-Cost` header (non-negative integer); this service no longer prices
  requests. `X-Api`/`X-Endpoint` become optional event metadata. Missing or
  invalid `X-Cost` → `400`. See ADR 0008.
- Renamed container/deploy artifacts to vendor-neutral names
  (`infra/image/Containerfile`, `infra/compose.yaml`) and Makefile targets
  (`image-build`, `stack-up`, `stack-down`); added `infra/compose.test.yaml`
  with `make test-compose` to run the whole test suite in containers
  (see ADR 0007).
- Added Architecture Decision Records under `docs/adrs/` (DDD vertical slicing,
  stdlib-only HTTP, proxy-trusted identity, signed-integer balance, layered
  JSON config, RESTful resource API, vendor-neutral infra naming,
  cost supplied by caller).

### Removed
- Pricing subsystem: pricing engine, file/remote pricing loaders,
  `domain.PricingRule` and time modifiers, `etc/pricing.example.json`, the
  `PricingConfig` port and the remote pricing refresh loop.
- Relocated `SPECS.md` to `docs/SPECS.md` and added the OpenAPI 3.1 contract
  at `docs/openapi.yaml`. Reconciled `SPECS.md`, `AGENTS.md` and `README.md`
  with the implemented RESTful surface (`POST /v1/charges`,
  `/v1/users/{id}` resources) which had drifted from the original
  RPC-style draft (`/v1/meter`, `/v1/admin/.../topup`).

### Added
- Per-user **periodic credit grant** in `entitlements.grant`: `reset` (monthly
  reset to amount, unused expire), `rollover` (monthly add amount up to an
  optional cap) or `rolling` (reset to amount every 30 days). Applied lazily on
  the next charge or balance read (no cron) and, via a row-locked compare on the
  period anchor, **at most once per period** even under concurrency.
- Per-user **entitlement time windows** (`plan_config` JSONB, migration `002`).
  Each window (UTC hours/days) applies one of three effects to a charge:
  `free` (passes, no deduction), `multiply` (scales the reported cost by a
  factor), or `block` (denied with `403 blocked_by_window`). First matching
  window wins. Configured via `PUT /v1/users/{id}/plan` under
  `entitlements.windows`. This moves time-of-day logic from price to the
  client's quota (ADR 0008).
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
  `MeteringEvent` entity, `User` aggregate.
- Application layer (`internal/metering/application`): use cases `Meter`,
  `GetBalance`, `GetUsage`, `TopUp`, `SetPlan`, `GetAdminUsage`, `ListUsers`,
  driven through `ports.go` interfaces (`UserRepository`, `EventRepository`,
  `Meterer`) with typed application errors.
- Service config loader (`config_loader.go`): local JSON file, remote HTTP
  source with TTL refresh, and environment overrides.
- HTTP infrastructure (`internal/metering/infrastructure/http_handler.go`,
  `router.go`): stdlib `net/http` handlers for all endpoints with RFC 7807
  problem+json errors and a `{ "data": ... }` envelope; Go 1.22+ ServeMux
  routing with auth, logging, recovery and request-size middleware.
- PostgreSQL persistence (`internal/metering/infrastructure/postgres_store.go`):
  `pgx/v5`-backed `UserRepository` and `EventRepository`, including atomic
  meter-and-deduct, with schema migration `migrations/001_initial_schema.sql`.
- Server entry point (`cmd/server/main.go`): loads configuration, wires the
  PostgreSQL store and use cases into the HTTP router with structured `slog`
  JSON logging, and runs the server with graceful shutdown.

### Removed
- `PROMPT.md` (the temporary development guide), now that the implementation
  is complete and documented under `docs/`.

### Fixed
- Credit mutations are now atomic. Top-up, set-plan and auto-provisioning did
  read-modify-write through a full-row `Upsert` outside any lock, so they could
  lose updates against a concurrent charge (which locks the row) — a top-up
  could clobber a deduction, and two concurrent first requests could reset a
  just-charged balance. They now use single atomic statements
  (`AddCredits` = `UPDATE ... balance = balance + $`, `SetPlan` = one `UPDATE`,
  provisioning = `INSERT ... ON CONFLICT DO NOTHING`). Covered by a concurrent
  top-up integration test.
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
