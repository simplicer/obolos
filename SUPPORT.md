# Support & Lifecycle

Obolos follows the Simplicer SemVer support policy. Each major version is
supported from its first stable release.

| Version | Class  | Released   | Active support until | Deprecated until | Sunset (gone) |
|---------|--------|------------|----------------------|------------------|---------------|
| 1.0.0   | Normal | 2026-06-05 | 2028-06-05 (24 mo)   | 2028-12-05 (6 mo)| 2029-06-05    |

- **Active** — full support: fixes, security and refactors.
- **Deprecated** — served, **security/vulnerability fixes only**; responses
  advertise the deprecation (`Deprecation`, `Sunset`, `Link` headers).
- **Sunset** — no longer accessible; requests get `410 Gone`
  (`application/problem+json`) pointing at the successor.

Production deploys only frozen release branches (`release/X.Y.Z`). The current
production line is **`release/1.0.0`** (tag `v1.0.0`).
