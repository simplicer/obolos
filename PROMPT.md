# PROMPT.md — Temporary Development Guide

> **DELETE THIS FILE** once development is complete. This is a reference for agents working on this project.

## What Is This Project?

**metering** is a Go microservice that acts as a credit-based billing engine for a platform of 10+ APIs. It sits behind a reverse proxy (Traefik/Caddy) and is called via `forwardAuth` on every API request. Its job is simple: "User X called endpoint Y on API Z — what does it cost, and do they have enough credits?"

## Architecture

- **DDD** with vertical slicing, Screaming Architecture, strict layer separation.
- **Single bounded context**: `metering`
- **Layers**: `domain/` → `application/` → `infrastructure/` (strict dependency direction)
- **Zero HTTP frameworks** — Go stdlib `net/http` with `ServeMux` (Go 1.22+ method routing)
- **PostgreSQL** via `jackc/pgx/v5` for persistence
- **JSON config** with local file + remote HTTP source support

## Project Structure

```
api-metering/
├── cmd/server/main.go                 # Entry point
├── internal/metering/                  # Bounded context
│   ├── domain/                         # Pure business logic
│   │   ├── balance.go                  # Balance value object
│   │   ├── plan.go                     # Plan types (prepaid, unlimited, hybrid)
│   │   ├── pricing_rule.go             # Pricing rule value object
│   │   ├── metering_event.go           # Metering event entity
│   │   └── *_test.go
│   ├── application/                    # Use cases
│   │   ├── meter.go                    # Meter use case (main flow)
│   │   ├── get_balance.go              # Get balance use case
│   │   ├── get_usage.go                # Get usage history
│   │   ├── topup.go                    # Admin top-up
│   │   ├── set_plan.go                 # Admin set plan
│   │   ├── ports.go                    # Interfaces (stores, config)
│   │   └── *_test.go
│   └── infrastructure/                 # Adapters
│       ├── http_handler.go             # HTTP handlers + Problem JSON
│       ├── router.go                   # Route definitions
│       ├── postgres_store.go           # PostgreSQL adapter
│       ├── config_loader.go            # JSON config (local + remote)
│       ├── pricing_engine.go           # Pricing calculation
│       └── *_test.go
├── docs/
│   ├── SPECS.md                        # Business specification
│   ├── openapi.yaml                    # OpenAPI 3.1
│   └── adrs/                           # Architecture Decision Records
├── etc/
│   ├── pricing.example.json            # Example pricing config
│   └── config.example.json             # Example service config
├── migrations/
│   └── 001_initial_schema.sql          # Database schema
├── infra/
│   └── docker/
│       ├── Dockerfile
│       └── docker-compose.yml
├── var/log/                            # Runtime logs
├── bin/                                # Compiled binary
├── README.md
├── AGENTS.md                           # Architecture rules
├── VERSION                             # 0.1.0
├── Makefile
├── go.mod                              # codeberg.org/simplicer/metering
├── CHANGELOG.md
├── LICENSE                             # GNU AGPL v3
└── PROMPT.md                           # THIS FILE — delete when done
```

## Implementation Order (Tasks)

### Task 1: Scaffold
- `go.mod`, `VERSION`, `Makefile`, `LICENSE`, `README.md`, `CHANGELOG.md`
- Directory structure
- `cmd/server/main.go` (minimal, just health + version)
- Copy `LICENSE` from vatguard (AGPL v3)

### Task 2: Domain Layer
- `balance.go` — Balance value object (signed int, CanDebit, Debit, Credit)
- `plan.go` — PlanType enum (prepaid, unlimited, hybrid), Plan entity
- `pricing_rule.go` — PricingRule with base_cost + time_modifiers, CostFor(endpoint, time) method
- `metering_event.go` — MeteringEvent entity
- 100% unit test coverage on domain

### Task 3: Application Layer
- `ports.go` — UserRepository, EventRepository, PricingConfig interfaces
- `meter.go` — Meter use case: lookup user → check plan → calculate cost → deduct balance → record event
- `get_balance.go` — GetBalance use case
- `get_usage.go` — GetUsage use case
- `topup.go` — TopUp use case (admin)
- `set_plan.go` — SetPlan use case (admin)
- Unit tests with mocked repositories

### Task 4: Pricing Engine
- `pricing_engine.go` — Implements domain pricing logic: match endpoint to rule, apply time modifiers
- Config loading from JSON file
- Remote config fetching with TTL cache
- Tests for all pricing scenarios

### Task 5: Infrastructure — HTTP
- `http_handler.go` — Handlers for all endpoints with Problem JSON errors
- `router.go` — Route definitions with logging middleware
- Same patterns as vatguard

### Task 6: Infrastructure — PostgreSQL
- `postgres_store.go` — Implement UserRepository and EventRepository
- Migration `001_initial_schema.sql`
- Integration tests with test DB

### Task 7: Integration — E2E
- `cmd/server/main.go` — Full wiring
- E2E tests: full metering flow via HTTP
- Docker support

## Key Domain Rules

1. **Prepaid plan**: `balance >= cost` required. If not, return error (HTTP 402). Balance never goes below 0.
2. **Unlimited plan**: No deduction. Always pass. Cost is logged as 0.
3. **Hybrid plan**: Deduct from balance. If balance goes below 0, mark event as `overage: true`. Never block.
4. **Auto-provision**: If a user_id is seen for the first time, create the user with the default plan and default credits.
5. **Time modifiers**: First matching modifier wins. Multiplier is applied to base_cost. Result is rounded to int64.
6. **Cost = 0**: Pass through, no balance check, no deduction. Just log the event.

## Consistency with vatguard

This project follows the same patterns as vatguard (`codeberg.org/simplicer/vatguard`):
- Same LICENSE (AGPL v3)
- Same error format (RFC 7807 Problem JSON)
- Same response envelope (`{ "data": {...} }`)
- Same logging (slog JSON, Loki-compatible)
- Same routing style (Go 1.22+ ServeMux)
- Same testing philosophy (colocated *_test.go files)
