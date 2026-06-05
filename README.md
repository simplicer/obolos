# Obolos

> **Obolos** (Greek *ὀβολός*) — the smallest unit of ancient Greek currency,
> the coin you reckon value in. This service counts the obols: the credit
> ledger and quota authority for an API platform.

A credit ledger and quota gate for API platforms. Called by your reverse proxy
(Traefik/Caddy/KrakenD) via `forwardAuth` on every request: it accounts the
caller-reported cost and enforces each user's balance and entitlement rules.

## What It Does

Receives `X-User-Id` and `X-Cost` (plus optional `X-Api`/`X-Endpoint`) from the
proxy and answers, per request: **"Does this user have the credits — and is this
request within their plan, windows, grants and rate limits?"**

## Features

- **Three plan types**: Prepaid (block at 0), Unlimited (no limit), Hybrid (prepaid → postpaid overage)
- **Cost reported by the caller**: the trusted proxy sends the per-request cost in `X-Cost`; Obolos does not own a price catalog
- **Entitlements**: per-user time windows (free / multiply / block), periodic grants (reset / rollover / rolling, never destructive), and rate limits
- **Idempotent charges**: optional `X-Request-Id` makes a `forwardAuth` retry safe
- **Hot-reloadable config**: Local JSON file or remote HTTP endpoint
- **Auto-provisioning**: New users get default plan on first request
- **PostgreSQL**: Durable storage for balances and metering events
- **Zero HTTP frameworks**: Go stdlib only (net/http + ServeMux)
- **AGPL v3**: Strong copyleft license

## Architecture

```
Client → Traefik (SSL + routing)
       → forwardAuth → IdP (validates token, injects X-User-Id)
       → forwardAuth → obolos (checks credits, deducts balance)
       → proxy → API (vatguard, api-2, ...)

```

DDD with vertical slicing. The product is **Obolos**; its single bounded
context (the domain capability) is `metering`, under `internal/metering/`.

```
internal/metering/
├── domain/          # Balance, Plan, MeteringEvent
├── application/     # Meter, GetBalance, GetUsage, TopUp, SetPlan
└── infrastructure/  # HTTP handlers, PostgreSQL, config loader
```

## Setup

### Prerequisites

- Go 1.24+
- PostgreSQL 15+

### Build & Run

```bash
make build
make run
```

### Containers

```bash
make image-build   # build the container image (infra/image/Containerfile)
make stack-up      # start the service + database stack (infra/compose.yaml)
make stack-down    # stop the stack
```

Run the full test suite inside containers:

```bash
make test-compose  # unit + integration tests via infra/compose.test.yaml
```

### Configuration

Copy the example config and edit:

```bash
cp etc/config.example.json etc/config.json
```

There is no pricing file: the per-request cost is sent by the caller in the
`X-Cost` header (see [`docs/adrs/0008-cost-supplied-by-caller.md`](docs/adrs/0008-cost-supplied-by-caller.md)).

## API

Full contract: [`docs/openapi.yaml`](docs/openapi.yaml) · spec: [`docs/SPECS.md`](docs/SPECS.md)

| Endpoint | Auth | Purpose |
|----------|------|---------|
| `POST /v1/charges` | proxy `X-User-Id` | Meter a request (proxy calls this) |
| `GET /v1/users/{id}` | owner or admin | Get user balance |
| `GET /v1/users/{id}/charges` | owner or admin | Get usage history |
| `GET /v1/users` | admin | List users |
| `POST /v1/users/{id}/credits` | admin | Add credits |
| `PUT /v1/users/{id}/plan` | admin | Set plan |
| `GET /health` | public | Health check |
| `GET /version` | public | Version info |

## License

GNU AGPL v3 — see [LICENSE](LICENSE).
