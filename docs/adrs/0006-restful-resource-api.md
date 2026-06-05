# 0006 — RESTful resource API over RPC endpoints

- Status: Accepted
- Date: 2025-06-04
- Supersedes: the RPC-style draft surface in early `SPECS.md`

## Context

The first draft (`SPECS.md`) described RPC-style endpoints: `POST /v1/meter`,
`GET /v1/balance`, `GET /v1/usage`, and `POST /v1/admin/users/{id}/topup`,
`/plan`, `/usage`. As the handlers were implemented, the surface was reworked
around resources to be consistent, cacheable and to use HTTP methods
meaningfully.

## Decision

Model the API around two resources with standard HTTP semantics:

- `POST /v1/charges` — create a charge (meter a request); `201 Created`.
- `GET /v1/users/{id}` — a user's balance.
- `GET /v1/users/{id}/charges` — a user's charges (sub-collection).
- `GET /v1/users` — list users (admin).
- `POST /v1/users/{id}/credits` — add credits (admin); `201 Created`.
- `PUT /v1/users/{id}/plan` — set the plan (admin); idempotent replace.

Authorization is by role: proxy identity for creating charges, owner-or-admin
for reading a user, admin-only for the collection and mutations.

## Consequences

- Methods carry meaning (`PUT` plan is idempotent; `POST` credits/charges
  create); responses use the correct `2xx` codes.
- The naming is uniform (`/v1/users/{id}/...`) instead of an `/admin/` prefix;
  authorization is enforced per route, not encoded in the path.
- The earlier RPC paths are **not** served. `SPECS.md`, `AGENTS.md`,
  `README.md` and `docs/openapi.yaml` were updated to this surface. Future
  breaking changes ship additively as new endpoint versions (see
  simplicer-semver §6).
