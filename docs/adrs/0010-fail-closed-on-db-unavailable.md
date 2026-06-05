# 0010 — Fail closed when the database is unavailable

- Status: Accepted
- Date: 2026-06-05

## Context

Metering is called by the reverse proxy via `forwardAuth` on every request:
a `2xx` allows the upstream call, a non-`2xx` denies it. If PostgreSQL is
unreachable, a charge cannot be recorded or a balance checked. We must decide
what happens then: **fail open** (allow the upstream call un-metered) or **fail
closed** (deny it).

## Decision

**Fail closed.** When the database is unavailable a charge returns
`500 internal_error`, which `forwardAuth` treats as a denial, so the upstream
API is not served. We never serve un-metered, un-billed traffic.

Operational support for this choice:

- `GET /ready` returns `503` when the database is unreachable (ADR R1), so the
  orchestrator and load balancer route away from a degraded instance and the
  blast radius is limited to instances that genuinely cannot serve.
- The PostgreSQL connection uses a pool with health checks; transient blips are
  absorbed by retries at the pool level.

## Consequences

- No revenue leak: a database outage never results in free, un-metered API
  usage.
- Availability of the *fronted* APIs is coupled to metering's database. This is
  accepted: the platform treats metering as part of the request-critical path,
  and the readiness probe plus connection pooling keep transient failures from
  becoming outages.
- If a future product wants a specific API to stay available during a metering
  outage, that is a deliberate per-API **fail-open allowlist** at the proxy
  (not in this service) — out of scope for 1.0.
