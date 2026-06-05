# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Major version 0 is pre-first-release: every build carries the `-unstable`
suffix and is never frozen or deployed to production. The first stable
release will be `1.0.0`.

## [Unreleased]

### Added
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

[Unreleased]: https://codeberg.org/simplicer/metering/compare/main
