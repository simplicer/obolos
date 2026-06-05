# 0001 — DDD with vertical slicing and Screaming Architecture

- Status: Accepted
- Date: 2025-06-04

## Context

The metering service is small today but is the billing core for a platform of
10+ APIs, so it must stay comprehensible and changeable as it grows. We had to
choose how to organize the code: by technical layer at the top level
(handlers/, models/, repositories/) or by business capability.

## Decision

Organize the code as a single bounded context, `metering`, living in
`internal/metering/`, split into three layers with a strict inward dependency
direction:

```
infrastructure → application → domain
```

- `domain/` — pure business logic (entities, value objects), no imports of the
  other layers, no frameworks.
- `application/` — use cases only, one concern per file, depending on `domain`
  and on ports it declares.
- `infrastructure/` — adapters (HTTP, PostgreSQL, config) implementing those
  ports.

The top-level layout (`cmd/`, `internal/metering/`, `etc/`, `infra/`,
`migrations/`) screams "metering service", not "Go web app".

## Consequences

- The domain is testable in isolation with no infrastructure.
- Swapping an adapter (e.g. a different store) touches only `infrastructure`.
- Go's convention of colocating code and tests is kept; there is no separate
  `tests/` tree. The single-context project does not need per-feature
  sub-slices yet; if a second context appears it gets its own slice under
  `internal/`.
- Developers must respect the dependency direction; a domain import of
  infrastructure is a review/CI failure.
