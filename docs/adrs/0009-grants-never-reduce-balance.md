# 0009 — Periodic grants never reduce a balance

- Status: Accepted
- Date: 2026-06-05
- Refines: the grant model added for entitlements (ADR 0008 lineage).

## Context

The first grant design had `reset` (and `rolling`) set the balance to the
grant amount at each period. That is fine for pure courtesy accounts, but it
silently **destroys purchased credits**: a user who tops up 5000 credits and
also carries a `reset` grant of 1000 would be wiped down to 1000 at the next
month boundary. For a billing ledger, silently destroying paid balance is
unacceptable.

## Decision

A periodic grant must **never reduce** a balance:

- `reset` and `rolling` now apply a **floor**: `balance = max(balance, amount)`.
  They top a low balance up to the periodic entitlement but leave a higher
  (e.g. topped-up) balance untouched.
- `rollover` is unchanged: `balance = min(balance + amount, cap)` (additive,
  also non-destructive).

Grants are applied lazily and at most once per period (unchanged).

## Consequences

- Purchased credits are never destroyed by a grant; top-ups and grants compose
  safely.
- True "use it or lose it" expiry (a separate, expiring courtesy pool spent
  before purchased credits) is **not** modelled here. If that is ever required,
  it should be a second balance pool (granted vs purchased), not a destructive
  reset — that is deliberate future work, kept out of the 1.0 ledger to avoid a
  schema and accounting change with money-loss risk.
