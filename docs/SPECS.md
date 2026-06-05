# SPECS.md — Metering Service Specification

## 1. Purpose

Metering is a lightweight Go service that counts API requests per user and deducts credits based on configurable pricing rules. It is designed to be called by a reverse proxy (Traefik, Caddy, KrakenD) via `forwardAuth` middleware.

**This service does NOT manage identity.** It trusts the `X-User-Id` header injected by the reverse proxy after IdP authentication.

## 2. Core Concepts

### 2.1 User (Owner)

A user is identified by an opaque string (the `sub` claim from the IdP). Users are auto-provisioned on first metered request with a default plan.

### 2.2 Balance

Every user has a credit balance. The balance is a signed integer:
- Positive: prepaid credits remaining
- Zero: no credits left (behavior depends on plan)
- Negative: overage consumed (only possible with hybrid plans)

### 2.3 Plans

| Plan Type | Balance Behavior | When Balance = 0 | When Balance < 0 |
|-----------|-----------------|-------------------|-------------------|
| **Prepaid** | Starts positive, decreases | Request blocked (HTTP 402) | Impossible |
| **Unlimited** | Always 0, never changes | Requests pass, no deduction | Impossible |
| **Hybrid** | Starts positive, can go negative | Switches to overage rate | Requests pass, balance tracks debt |

### 2.4 Pricing

Pricing is defined per API and per endpoint in a JSON configuration file. Each endpoint has:
- `base_cost`: credits consumed per request
- `time_modifiers` (optional): multiplier applied during specific hours/days

Time modifiers are evaluated in order. First match wins. If no modifier matches, `base_cost` is used as-is.

### 2.5 Metering Event

Every metered request generates a MeteringEvent stored in PostgreSQL:

```
MeteringEvent {
    id: UUID
    user_id: string
    api: string              // e.g. "vatguard"
    endpoint: string         // e.g. "/v1/vies/{number}"
    base_cost: int64
    applied_multiplier: float64
    final_cost: int64
    balance_before: int64
    balance_after: int64
    plan_type: string
    overage: bool
    timestamp: time
}
```

## 3. Configuration

### 3.1 Sources

Configuration can come from three sources, in priority order:

1. **Remote HTTP endpoint**: `CONFIG_REMOTE_URL` env var. The service fetches JSON from this URL on a configurable interval (default: 60s). If the fetch fails, it falls back to cached/local config.
2. **Local JSON file**: `etc/pricing.json` for pricing, `etc/config.json` for service config. Reloaded on SIGHUP.
3. **Environment variables**: Override specific values (PORT, LOG_LEVEL, DATABASE_URL, etc.).

### 3.2 Pricing Config Schema

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["apis"],
  "properties": {
    "apis": {
      "type": "object",
      "additionalProperties": {
        "type": "object",
        "required": ["endpoints"],
        "properties": {
          "endpoints": {
            "type": "object",
            "additionalProperties": {
              "type": "object",
              "required": ["base_cost"],
              "properties": {
                "base_cost": { "type": "integer", "minimum": 0 },
                "time_modifiers": {
                  "type": "array",
                  "items": {
                    "type": "object",
                    "required": ["hours", "multiplier"],
                    "properties": {
                      "hours": { "type": "string", "description": "e.g. '09:00-17:00'" },
                      "days": {
                        "type": "array",
                        "items": { "type": "string", "enum": ["mon","tue","wed","thu","fri","sat","sun"] }
                      },
                      "multiplier": { "type": "number", "minimum": 0 }
                    }
                  }
                }
              }
            }
          },
          "default_cost": { "type": "integer", "minimum": 0 }
        }
      }
    },
    "default_cost": { "type": "integer", "minimum": 0, "default": 1 }
  }
}
```

### 3.3 Service Config Schema

```json
{
  "port": 8080,
  "log_level": "info",
  "database_url": "postgres://...",
  "default_plan_type": "prepaid",
  "default_plan_credits": 1000,
  "config_remote_url": "",
  "config_refresh_interval_seconds": 60
}
```

## 4. API Endpoints

The surface is modelled RESTfully around two resources: **charges** and
**users**. The full machine-readable contract is in
[`docs/openapi.yaml`](./openapi.yaml). Every success uses a `{ "data": ... }`
envelope; every error uses `application/problem+json` (RFC 7807 / RFC 9457).

Authorization:
- Public: `GET /health`, `GET /version`.
- Proxy identity (`X-User-Id`): `POST /v1/charges`.
- Owner (`X-User-Id` == `{user_id}`) or admin (`X-Admin-Key`):
  `GET /v1/users/{user_id}`, `GET /v1/users/{user_id}/charges`.
- Admin only (`X-Admin-Key`): `GET /v1/users`,
  `POST /v1/users/{user_id}/credits`, `PUT /v1/users/{user_id}/plan`.

### 4.1 Create Charge (called by proxy)

```
POST /v1/charges
Headers:
  X-User-Id: string (required, injected by proxy after IdP auth)
  X-Api: string (required, e.g. "vatguard")
  X-Endpoint: string (required, e.g. "/v1/vies/{number}")

Response 201:
  { "data": { "cost": 5, "balance": 495, "plan_type": "prepaid" } }

Response 402 (application/problem+json):
  { "data": { "cost": 5, "balance": 0, "plan_type": "prepaid" },
    "error": { "title": "insufficient_credits", "status": 402 } }
```

The user is auto-provisioned with the default plan on its first charge.

### 4.2 Get User Balance

```
GET /v1/users/{user_id}
Headers: X-User-Id (owner) or X-Admin-Key (admin)

Response 200:
  { "data": { "user_id": "...", "balance": 495, "plan_type": "prepaid", "total_usage_this_month": 505 } }
```

### 4.3 Get User Charges

```
GET /v1/users/{user_id}/charges?from=2025-06-01&to=2025-06-30
Headers: X-User-Id (owner) or X-Admin-Key (admin)

Response 200:
  { "data": { "events": [...], "total_cost": 505, "overage_cost": 0 } }
```

### 4.4 List Users (admin)

```
GET /v1/users
Headers: X-Admin-Key

Response 200:
  { "data": { "users": [ { "user_id": "...", "plan_type": "prepaid", "balance": 495, "created_at": "..." } ], "total": 1 } }
```

### 4.5 Add Credits (admin)

```
POST /v1/users/{user_id}/credits
Headers: X-Admin-Key
Body: { "amount": 5000 }

Response 201:
  { "data": { "user_id": "...", "balance": 5495 } }
```

### 4.6 Set Plan (admin)

```
PUT /v1/users/{user_id}/plan
Headers: X-Admin-Key
Body: { "plan_type": "hybrid", "credits": 10000, "overage_rate": 0.01 }

Response 200:
  { "data": { "user_id": "...", "plan_type": "hybrid", "credits": 10000 } }
```

### 4.7 Health & Version

```
GET /health → { "data": { "status": "ok" } }
GET /version → { "data": { "version": "0.6.3-unstable" } }
```

## 5. Database Schema

### users table

```sql
CREATE TABLE users (
    id VARCHAR(255) PRIMARY KEY,
    plan_type VARCHAR(50) NOT NULL DEFAULT 'prepaid',
    balance BIGINT NOT NULL DEFAULT 0,
    overage_rate DECIMAL(10,4) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### metering_events table

```sql
CREATE TABLE metering_events (
    id UUID PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL REFERENCES users(id),
    api VARCHAR(255) NOT NULL,
    endpoint VARCHAR(255) NOT NULL,
    base_cost BIGINT NOT NULL,
    applied_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0,
    final_cost BIGINT NOT NULL,
    balance_before BIGINT NOT NULL,
    balance_after BIGINT NOT NULL,
    plan_type VARCHAR(50) NOT NULL,
    overage BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_events_user_id ON metering_events(user_id);
CREATE INDEX idx_events_created_at ON metering_events(created_at);
CREATE INDEX idx_events_api ON metering_events(api);
```

## 6. Error Handling

All errors follow RFC 7807 Problem JSON format, consistent with vatguard.

| HTTP Status | Error | Meaning |
|-------------|-------|---------|
| 400 | `missing_user_id` | X-User-Id header not present |
| 400 | `missing_api` | X-Api header not present |
| 400 | `missing_endpoint` | X-Endpoint header not present |
| 402 | `insufficient_credits` | Prepaid plan, balance is 0 |
| 429 | `rate_limit_exceeded` | Too many requests |
| 500 | `internal_error` | Database or config error |

## 7. Non-Goals

- Token management / identity / authentication (done by IdP + proxy)
- SSL termination (done by Traefik/Caddy)
- Reverse proxying (done by Traefik/Caddy)
- Invoice generation (future: separate service or manual from metering_events)
- Payment processing (future: Stripe integration)
