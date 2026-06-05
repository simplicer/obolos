# Metering

Credit-based billing engine for API platforms. Called by your reverse proxy (Traefik/Caddy/KrakenD) via `forwardAuth` on every request.

## What It Does

Receives `X-User-Id`, `X-Api`, `X-Endpoint` headers from the proxy and answers: **"Does this user have enough credits for this request?"**

## Features

- **Three plan types**: Prepaid (block at 0), Unlimited (no limit), Hybrid (prepaid → postpaid overage)
- **Per-endpoint pricing**: JSON config with time-based modifiers (peak/off-peak)
- **Hot-reloadable config**: Local JSON file or remote HTTP endpoint
- **Auto-provisioning**: New users get default plan on first request
- **PostgreSQL**: Durable storage for balances and metering events
- **Zero HTTP frameworks**: Go stdlib only (net/http + ServeMux)
- **AGPL v3**: Strong copyleft license

## Architecture

```
Client → Traefik (SSL + routing)
       → forwardAuth → IdP (validates token, injects X-User-Id)
       → forwardAuth → metering (checks credits, deducts balance)
       → proxy → API (vatguard, api-2, ...)

```

DDD with vertical slicing. Single bounded context: `metering`.

```
internal/metering/
├── domain/          # Balance, Plan, PricingRule, MeteringEvent
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

### Docker

```bash
make docker-build
make docker-up
```

### Configuration

Copy example configs and edit:

```bash
cp etc/config.example.json etc/config.json
cp etc/pricing.example.json etc/pricing.json
```

## API

| Endpoint | Purpose |
|----------|---------|
| `POST /v1/meter` | Meter a request (proxy calls this) |
| `GET /v1/balance` | Get user balance |
| `GET /v1/usage` | Get usage history |
| `GET /health` | Health check |
| `GET /version` | Version info |
| `POST /v1/admin/users/{id}/topup` | Add credits (admin) |
| `POST /v1/admin/users/{id}/plan` | Set plan (admin) |
| `GET /v1/admin/users/{id}/usage` | Get usage (admin) |

## License

GNU AGPL v3 — see [LICENSE](LICENSE).
