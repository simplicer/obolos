# 0008 — Cost is supplied by the caller, not priced here

- Status: Accepted
- Date: 2026-06-05
- Supersedes: ADR 0005 (pricing config) and the pricing portions of the
  original SPECS; complements ADR 0003 (proxy-trusted identity).

## Context

The service originally owned a pricing catalog (`pricing.json`: per-API,
per-endpoint `base_cost` with time modifiers) and computed the cost of each
request. That forced this bounded context to know the endpoints, infra
details and price policy of APIs it does not control, and created drift
between each API and metering's price table. It also caused real defects
(a missing `default_cost` silently made endpoints free; a negative
`base_cost` would *credit* the user).

The party that performs the metering call — the reverse proxy / middleware
in front of each API — already knows what the call should cost. That is the
same trusted component that injects `X-User-Id` (ADR 0003).

## Decision

Metering does **not** price requests. The trusted caller reports the cost in
credits via the `X-Cost` header on `POST /v1/charges`. This service is a
**credit ledger and quota authority**: it owns the client's balance, plan
rules (prepaid / unlimited / hybrid) and entitlement rules (time windows,
grants/expiry, rate limits), and it enforces them against the reported cost.

- `X-Cost` is a required, non-negative integer; missing/invalid → `400`.
- `X-Api` / `X-Endpoint` become **optional metadata** stored on the event
  for analytics; they no longer affect cost.
- The whole pricing subsystem is removed: `pricing_engine`, `pricing_loader`,
  `domain.PricingRule`, time modifiers, `etc/pricing.example.json`, the
  `PricingConfig` port and the remote pricing refresh.

Trust boundary is unchanged: the cost is set by the proxy, not the end
client, exactly like `X-User-Id`. The deployment invariant (the proxy must
strip inbound `X-Cost`/`X-User-Id` from clients) is the same one ADR 0003
already requires. Metering still validates `cost ≥ 0` as a safety net.

## Consequences

- The bounded context shrinks to its real domain (credit accounting), is
  simpler, and sheds a class of pricing-misconfiguration bugs.
- Price policy can change per API without redeploying or reconfiguring
  metering; pricing lives where the knowledge is.
- The trade-off: there is no central price catalog, so each caller is
  trusted to report cost correctly. If central price control is ever needed,
  reintroduce it as a separate pricing context in front of metering — not
  inside it.
- Time-of-day logic moves from *price* (old time modifiers) to the client's
  *entitlement* (free windows on the plan), which is its natural home.
