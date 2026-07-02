# DEC-002: State Store — Supabase-hosted Postgres, plain-Postgres compatible

**Status:** Accepted
**Date:** 2026-07-02
**Decided by:** Sam
**Resolves:** Vision v1, Section 14, "Embedded vs hosted Postgres"

## Decision

Cromwell's canonical state store is **Postgres hosted on Supabase**. The codebase
targets **plain Postgres** — every feature Cromwell depends on must work against a
vanilla Postgres 16+ instance. Supabase is the recommended and default *host*, not
a dependency.

## Context

The vision left the default open between embedded Postgres, SQLite-with-sync, and
hosted. Sam has chosen Supabase hosting for this project.

## Rationale

- **Managed operations.** Backups, upgrades, connection pooling (Supavisor), and
  monitoring come free. For a small team, zero database operations work.
- **Realtime push.** Supabase Realtime will power the live web UI in a later phase
  without Cromwell building a push layer.
- **pgvector included.** Document/section embeddings (vision §6, §10) need pgvector;
  Supabase ships it enabled.
- **Auth, later.** When multi-user arrives, Supabase Auth is available at the web
  UI boundary without touching the server core.

## Compatibility boundary

To keep Supabase a host rather than a lock-in:

| Concern | Mechanism | Supabase-specific? |
|---|---|---|
| State reads/writes | `pgx` over the Postgres wire protocol | No |
| Server-internal eventing | In-process event bus; `LISTEN/NOTIFY` as the cross-process channel | No |
| Embeddings | `pgvector` extension | No (standard extension) |
| Full-text search | Postgres `tsvector` | No |
| Web UI live updates (later) | Supabase Realtime, behind a `Notifier` interface | Yes — isolated at the UI boundary |
| Auth (later) | Supabase Auth, at the web UI boundary only | Yes — isolated |

The server core (orchestrator, lifecycle engine, ledger) must never import a
Supabase SDK.

## Migrations

Cromwell owns its schema. Migrations are forward-only, numbered SQL files embedded
in the binary and applied by `cromwell init` / `cromwell upgrade` (vision §12).
Supabase's own migration tooling is **not** used — Supabase is treated purely as a
Postgres host. The migration runner records applied versions in a
`schema_migrations` table.

## Local development

Developers run either:

- `supabase start` (the Supabase CLI local stack), or
- a plain `postgres:16` container with `pgvector` installed.

Both must pass the full test suite; CI runs against plain Postgres to enforce the
compatibility boundary.

## Alternatives considered

- **Embedded Postgres / SQLite.** Simpler single-developer start, but forfeits
  realtime, pgvector maturity, and managed backups; and a dual SQLite/Postgres
  target doubles the query-compatibility surface. Rejected for v1; the plain-
  Postgres compatibility rule keeps a self-hosted path open.
- **Supabase-native (Realtime, Auth, RLS as core dependencies).** Faster web UI
  later, but couples the state store to one vendor and violates the vision's
  "Supabase recommended, not required" stance. Rejected.

## Consequences

- Connection config lives in `.cromwell/config.yaml` (connection string or
  Supabase project ref + key); secrets via environment variables, never committed.
- Server holds a small `pgx` pool; the CLI does not talk to Postgres directly —
  it talks to the server (see DESIGN-002, single-writer rule).
- Historical model prices are snapshotted into the dispatch ledger at dispatch
  time (partially resolving Section 14's price-table question — see DESIGN-001).
