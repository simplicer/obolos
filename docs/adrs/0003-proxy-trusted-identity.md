# 0003 — Proxy-trusted identity, no token introspection

- Status: Accepted
- Date: 2025-06-04

## Context

Every metered request needs a caller identity. The platform already terminates
TLS and authenticates callers at the reverse proxy (Traefik/Caddy/KrakenD)
against the IdP before forwarding. Re-validating tokens here would duplicate
that work on the hot path and couple metering to the IdP.

## Decision

The metering service does **not** manage identity and does **not** introspect
tokens. It trusts the `X-User-Id` header that the proxy injects after
authentication, and treats it as an opaque owner identifier. Administrative
routes are gated by a separate shared `X-Admin-Key`.

## Consequences

- The service stays simple and fast; no IdP round-trips.
- Security depends on the proxy: clients must never be able to reach the
  service directly and set `X-User-Id` themselves. This is a deployment
  invariant (network policy / the proxy strips inbound `X-User-Id`).
- Admin authorization is a coarse shared key, adequate for internal/admin use;
  a finer scheme is a future decision if admin access widens.
