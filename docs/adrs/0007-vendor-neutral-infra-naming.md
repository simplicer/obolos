# 0007 — Vendor-neutral naming for container and deploy artifacts

- Status: Accepted
- Date: 2026-06-05

## Context

Deployment artifacts originally carried the tool's brand in their paths:
`infra/docker/Dockerfile` and `infra/docker-compose.yml`, with Makefile
targets `docker-build` / `docker-up` / `docker-down`. Screaming Architecture
says the structure should reveal *what* the system is and does — a deployable
service — not *which tool* happens to run it. Both the OCI ecosystem and the
Compose Specification now provide vendor-neutral canonical names.

The `ralph-loop-ddd` skill's literal example prescribes `infra/docker/` and
`infra/docker-compose.yml`; this ADR deliberately overrides that example on
Screaming-Architecture grounds (the skill's own precedence rules put
ecosystem standards and project conventions above a literal example).

## Decision

Name our deploy/container artifacts vendor-neutrally under `infra/`:

- `infra/image/Containerfile` — the image build recipe (OCI standard name,
  honoured by Docker/BuildKit via `-f` and natively by Podman/Buildah),
  not `Dockerfile`.
- `infra/compose.yaml` — the runtime stack, the Compose Specification's
  canonical filename, not `docker-compose.yml`.
- `infra/compose.test.yaml` — the full test suite run inside containers.

Makefile targets are intent-revealing: `image-build`, `stack-up`,
`stack-down`, `test-compose`.

Exempt (not ours to rename): the `docker compose` CLI verb, the Compose
schema's `dockerfile:` key, and the Postgres image's
`/docker-entrypoint-initdb.d` convention. These are tool/spec/upstream
tokens, not project structure.

## Consequences

- The repository screams "deployable service + its tests", not "Docker".
- The artifacts are portable across OCI runtimes (Podman can build the
  Containerfile and run the compose files unchanged).
- Anyone used to `Dockerfile` / `docker-compose.yml` must learn the new
  names; the README, AGENTS.md and Makefile `help` document them.
- This is a documented, intentional divergence from the ralph-loop-ddd
  example layout.
