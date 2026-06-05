# 0004 — Signed-integer balance for hybrid overage

- Status: Accepted
- Date: 2025-06-04

## Context

Three plan types must be supported: prepaid (block at zero), unlimited (never
deducted) and hybrid (prepaid credits, then postpaid overage). Hybrid must keep
serving requests after credits are exhausted while tracking the debt. Money-like
quantities must avoid floating-point rounding error.

## Decision

Represent the balance as a **signed 64-bit integer** count of credits
(`int64`, stored as `BIGINT`). Costs are integers; time modifiers multiply the
base cost and the result is rounded to `int64`. Plan rules interpret the sign:

- Prepaid: deduction is rejected when it would take the balance below zero
  (HTTP 402); the balance never goes negative.
- Unlimited: never deducted; cost is logged as zero.
- Hybrid: always deducted; the balance may go **negative**, and that negative
  magnitude is the overage debt.

## Consequences

- One representation serves all three plans; "overage" is simply "balance < 0".
- No floating-point credit arithmetic, so no cent-drift; the only float is the
  pricing multiplier, applied once and rounded immediately.
- Reporting overage is a matter of summing negative movements, surfaced as
  `overage_cost` in usage responses and an `overage` flag per event.
