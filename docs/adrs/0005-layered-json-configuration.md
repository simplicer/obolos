# 0005 — Layered JSON configuration sources

- Status: Accepted (pricing config portion superseded by ADR 0008)
- Date: 2025-06-04

## Context

Pricing and service configuration change at different cadences and in different
environments. Pricing may need to change without a redeploy; per-environment
values (port, log level, database URL) come from the platform; secrets come
from the environment. We want hot reload without a config framework.

## Decision

Configuration is JSON, loaded from three sources in priority order:

1. **Remote HTTP endpoint** (`config_remote_url`), polled on an interval, for
   dynamic/centralised values; falls back to cached/local on fetch failure.
2. **Local JSON file** (`etc/config.json`, `etc/pricing.json`), reloadable on
   `SIGHUP`.
3. **Environment variables**, which override specific values (PORT, LOG_LEVEL,
   DATABASE_URL, …).

Parsing uses `encoding/json` only. Pricing changes take effect on the next
request after a reload (TTL-bounded), with no per-request caching beyond that.

## Consequences

- Pricing can be updated centrally without restarting the service.
- Configuration is hot-reloadable (`SIGHUP` and the refresh loop) with no extra
  dependency.
- Three sources means precedence must be documented and tested; the loader owns
  that precedence so call sites read one resolved config.
- Only `*.example.json` is committed; real `etc/config.json` / `etc/pricing.json`
  are git-ignored.
