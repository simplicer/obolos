# Architecture Decision Records

Each ADR captures one significant decision: its context, the choice made and
the consequences. ADRs are immutable once accepted; a later ADR supersedes an
earlier one rather than editing it. They record *why*; `AGENTS.md` records the
resulting *rules*.

| ADR | Title | Status |
|-----|-------|--------|
| [0001](0001-ddd-vertical-slicing.md) | DDD with vertical slicing and Screaming Architecture | Accepted |
| [0002](0002-zero-http-frameworks.md) | Zero HTTP frameworks — Go stdlib only | Accepted |
| [0003](0003-proxy-trusted-identity.md) | Proxy-trusted identity, no token introspection | Accepted |
| [0004](0004-signed-integer-balance.md) | Signed-integer balance for hybrid overage | Accepted |
| [0005](0005-layered-json-configuration.md) | Layered JSON configuration sources | Accepted |
| [0006](0006-restful-resource-api.md) | RESTful resource API over RPC endpoints | Accepted |
| [0007](0007-vendor-neutral-infra-naming.md) | Vendor-neutral naming for container and deploy artifacts | Accepted |
| [0008](0008-cost-supplied-by-caller.md) | Cost is supplied by the caller, not priced here | Accepted |
| [0009](0009-grants-never-reduce-balance.md) | Periodic grants never reduce a balance | Accepted |
| [0010](0010-fail-closed-on-db-unavailable.md) | Fail closed when the database is unavailable | Accepted |
