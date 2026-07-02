# DEC-001: Server Language — Go

**Status:** Accepted
**Date:** 2026-07-02
**Decided by:** Sam
**Resolves:** Vision v1, Section 14, "Server language and runtime"

## Decision

The Cromwell server, CLI, and all tooling are written in **Go**.

## Context

The vision deferred Go vs Node vs other. The server is a long-running process that
hosts the orchestrator, serves HTTP (web UI, later), serves the MCP facet, watches
a git repository, listens to Postgres, and dispatches concurrent agent calls to
provider APIs. The deployment target is a single machine per project.

## Rationale

- **Team fit.** Kanbanzai is Go; the team's engineering muscle and existing patterns
  (worktree management, MCP serving, YAML/Markdown tooling) transfer directly.
- **Deployment story.** A single static binary. `cromwell init` and the server ship
  as one artifact with migrations embedded (`embed.FS`). No runtime to install on
  the host.
- **Concurrency model.** The orchestrator is fundamentally a concurrent event loop
  with worker pools, per-feature serialisation, and rate governors. Goroutines and
  channels are the natural shape for this; Go's `context` propagation maps cleanly
  onto dispatch cancellation and budget cutoffs.
- **Postgres ecosystem.** `pgx` is mature, supports LISTEN/NOTIFY natively, and has
  first-class pgvector support via `pgvector-go`.
- **Provider SDKs.** The official Anthropic Go SDK covers tool use, streaming, and
  prompt caching; OpenAI similarly.

## Alternatives considered

- **Node/TypeScript.** Stronger web-UI co-location (shared types with a React
  front end) and a larger AI-tooling ecosystem. Rejected: the server is the
  load-bearing component and the UI is a later phase; Go's deployment and
  concurrency advantages matter more, and the team is not a Node team. The web UI
  will talk to the server over a typed HTTP API regardless of server language, so
  UI-language freedom is preserved.
- **Rust.** Performance is not a constraint at this scale; iteration speed matters
  more. Rejected.

## Consequences

- The web UI (later phase) is a separate front-end project consuming the server's
  HTTP API; types are shared via OpenAPI or similar generated contracts, not a
  shared language.
- MCP facet uses the Go MCP SDK.
- All design documents may assume Go idioms (interfaces, `context.Context`,
  `embed.FS`) when sketching component boundaries.
