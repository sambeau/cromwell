# Reviewing the codebase-memory-mcp plan

Cromwell has carried a plan to integrate [codebase-memory-mcp](https://github.com/DeusData/codebase-memory-mcp) since the vision document. The plan was written against research done for the predecessor project, and both the tool and Cromwell have moved since. This report re-examines it.

The short answer: **keep it on the roadmap, but turn the integration around.** The tool is a better fit than it was — more capable, provider-neutral, MIT-licensed, and it demonstrably finds things a grep-based search misses. The plan's *mechanism*, though, is the one that failed in the predecessor: expose a graph tool to sub-agents and hope they use it. Cromwell can now do something the predecessor could not, which is have the orchestrator query the graph and push the answer into the prompt.

There's also a finding that arrived by accident. While testing the tool on Cromwell itself, it surfaced a prompt-assembly site that my earlier prefix-cache audit missed — which means that report's defect list is incomplete, and it's a concrete demonstration of the tool's value in the same breath.

**Section 5 now reports the spike this report called for**, run against Tickly — a real TypeScript monorepo rather than this Go repository — first on version 0.6.0 and then again on 0.9.0. It changes two of the conclusions below. The cross-language HTTP boundary, which §4 calls the strongest single argument for the tool, **does not work on TypeScript at all**; and `detect_changes` does not compute a blast radius, so the code-review push has to be composed from two tools rather than taken from one. Both defects survived the upgrade, so they are properties of the tool rather than of an old build. Sections 3 and 4 have been corrected in place, with pointers to the evidence.

The case for keeping the tool now rests on **cross-package** structure rather than **cross-language** structure — a smaller claim than §4 makes, and the one the measurements support.

---

## Table of contents

1. [What the current plan says](#1-what-the-current-plan-says)
2. [What codebase-memory-mcp is now](#2-what-codebase-memory-mcp-is-now)
3. [Testing it on Cromwell](#3-testing-it-on-cromwell)
4. [What changes for mixed-language projects](#4-what-changes-for-mixed-language-projects)
5. [Running the spike on Tickly](#5-running-the-spike-on-tickly)
6. [Does Claude make it unnecessary?](#6-does-claude-make-it-unnecessary)
7. [Alternatives](#7-alternatives)
8. [The integration is backwards](#8-the-integration-is-backwards)
9. [Recommendation](#9-recommendation)
10. [Sources](#10-sources)

---

## 1. What the current plan says

Three documents carry it.

**Vision §8** describes it as an inherited investment: per-worktree graph projects, where the orchestrator derives a project name from the worktree path, indexes on worktree creation, and tears the index down on garbage collection. It also sets the architectural posture — *"the Cromwell server is an MCP client for external services like codebase-memory-mcp, re-exposing their capabilities through curated tools"* — and describes implicit context injection, where an agent calls `search_graph(query=…)` without knowing which project to target because the server injects it from the worktree record.

**DESIGN-006 §8** defers the whole thing past phase 2, with reasoning worth preserving: it's an optimisation on a working tool host, not load-bearing for proving the implementation loop. The `worktrees.graph_project` column and the ToolContext field exist from the start so that adding the integration later changes an implementation rather than a contract.

**SPEC-002 SD-1** records the deferral, and `internal/config/compartment.go` enforces it — `search_graph` is a named-but-unimplemented tool, and declaring it in a role profile is a config error.

So the current state is: a column that is always NULL, a tool name that is deliberately un-declarable, and a documented intention.

The deferral was the right call and the "true subset" reasoning still holds. What's worth re-examining is the shape the integration takes when it does land.

---

## 2. What codebase-memory-mcp is now

The tool has changed considerably since the predecessor's research. What it is today:

**A single static binary** for macOS, Linux, and Windows. No Docker, no runtime dependencies, no API keys. Installable by shell script or through npm, PyPI, Homebrew, Scoop, Winget, Chocolatey, AUR, or `go install`.

**Local and private by construction.** All processing happens on the machine; nothing leaves it. Zero telemetry, no background version checks. Storage is SQLite in WAL mode under `~/.cache/codebase-memory-mcp/`.

**158 languages** via vendored tree-sitter grammars, plus a "Hybrid LSP" type-resolution layer that resolves imports, generics, inheritance, type inference, and stdlib calls without running external language servers. Its coverage list names **TypeScript, JavaScript, JSX, and TSX explicitly**, alongside Python, PHP, C#, Go, C, C++, Java, Kotlin, Rust, and Perl.

Resolution quality is published in three bands, and the distribution is the part worth reading. The "excellent" tier (≥90%) holds Lua, Kotlin, C++, Perl, Objective-C, Bash, Zig, and Swift. **Every mainstream language a Cromwell project is likely to be written in sits one tier down, in the "good" band at 75–89%: TypeScript, JavaScript, Python, Go, Java, C#, Rust, PHP.**

The implication is not that any particular language is penalised — it is that 75–89% is simply where the common case lands, and no language choice available to a target project escapes it. Plan for a call graph that is mostly right rather than complete, whatever the project is written in.

**Fifteen MCP tools**, and — this is the part that matters most for Cromwell — **every one of them can be invoked as a one-shot CLI command** without starting the daemon.

**Three search modes in one tool.** `search_graph` now takes a BM25 full-text `query` with camelCase splitting and structural label boosting, a regex `name_pattern`, and a `semantic_query` array doing vector cosine search. The semantic mode bridges vocabulary — finding "publish" when you searched "send" — and it runs locally with no API key.

**A research foundation.** An arXiv preprint (2603.27277) evaluates it across 31 real-world repositories, reporting 83% answer quality, 10× fewer tokens, and 2.1× fewer tool calls against file-by-file exploration.

**MIT licensed**, which matters more than it looks — see §7.

Capabilities the predecessor's research predates: `manage_adr` for architecture decision records, `ingest_traces` for validating HTTP call edges against runtime traces, a committed team-shared graph artefact (`.codebase-memory/graph.db.zst`), Cypher queries, a 3D graph UI, and installer support for 37 coding agents.


### The daemon is not the cache

This distinction caused confusion in an earlier draft of this report, so it's worth stating precisely.

**The index is a plain SQLite file on disk**, one per project, at `~/.cache/codebase-memory-mcp/<project>.db`. On this machine those files are 10 MB for Cromwell and 138 MB for the predecessor. They persist whether or not any process is running.

**The daemon is a coordinator, not a store.** It exists to share resources across *concurrent, mutually unaware* agent sessions — managing file watchers, sequencing indexing jobs, and hosting the optional 3D graph UI. Sessions register work with it and the last one out shuts it down.

**A CLI invocation reads the same database.** Verified directly: `cli list_projects` returned the full cached node and edge counts for every indexed project in **0.036 seconds**, with no daemon involved and no standing process left behind.

So using the CLI does **not** mean giving up the index or re-parsing anything. It means not speaking JSON-RPC to a long-lived server process. The cache is identical either way. What you give up is the background file watcher — which, as §8 argues, is a thing Cromwell actively does not want.

---

## 3. Testing it on Cromwell

Rather than trust the README, I indexed Cromwell and ran the tools that the integration would actually depend on. Worth noting up front: **only the predecessor was indexed** — 79,611 nodes and 147,847 edges sitting in the cache from a year ago. Cromwell had never been indexed at all.

Indexing Cromwell took seconds and produced 2,794 nodes and 7,926 edges.

**Read everything below with one caveat.** Cromwell is a small, single-language Go repository, and it is *not* representative of what Cromwell will be pointed at. Target projects are large and mixed-language, most often TypeScript and JavaScript. Testing against this repo is the same category error as sizing a document corpus against Cromwell's own docs — convenient, and the wrong sample. The qualitative findings below are real, but §4 covers what changes when the project looks like a real one.

The performance findings, however, were re-run against a genuinely representative project — see immediately below.

### Indexing cost, measured on a real project

The obvious worry about a graph is that building it is slow, and that the cost falls on every worktree. Measured against a 204,463-line Go project (`basil`, 434 source files), it isn't.

| Subject | Scale | Full index | Result |
|---|---|---|---|
| Cromwell | 176 Go files, 48k lines | seconds | 2,859 nodes / 8,015 edges — queryable |
| basil, source only | 436 files, **204,463 lines** | **1.7 s** | 7,710 nodes / 47,840 edges — queryable |
| basil, whole repo as checked out | the above plus 1.7 GB of media and binaries | 5.8 s | 67,521 edges written — **not queryable, see below** |
| Query against a warm cache | — | **0.036 s** | Full result, no daemon |

**1.7 seconds for 204,000 lines.** The pipeline is RAM-first and parallel — 382% CPU observed across cores — and the published Linux-kernel figure (28M lines in 3 minutes) implies roughly 155,000 lines per second, which is consistent with what I measured.

This settles the performance question rather than deferring it to a spike. **A full re-index at this scale costs less than a `git worktree add`.** Any design contorted to avoid re-indexing is optimising something that costs under two seconds.

### The real risk is silent failure, not slowness

Indexing `basil` as it actually sits on disk — 1.7 GB, including 1,255 images, a 211 MB `.MOV` in test data, and a 41 MB committed binary — behaved like this:

- It reported `status: indexed` and exited 0.
- It wrote a 35 MB database and logged 67,521 edges.
- `list_projects` showed it with **`nodes: 0` and an empty `root_path`**.
- Every query returned `project not found or not indexed` — while the project simultaneously appeared in that error's own `available_projects` list.

The cause is the repository's non-source content, not its code. The identical 204,463 lines, copied to a clean directory without the media and binaries, indexed in 1.7 seconds and queried correctly. Orphaned `-shm` and `-wal` files were already sitting in the cache from an earlier attempt, so this had happened before and nothing surfaced it.

Two consequences, and they matter more than anything else in this report:

**Exclusion configuration is not housekeeping, it is a precondition.** The `.cbmignore` and `.gitignore` handling has to actually exclude media, build output, and committed binaries, and that has to be verified per project. The caution I wrote in §4 about `node_modules` for JavaScript projects turns out to understate it — this is a Go project, and the failure was total rather than merely noisy.

**Cromwell must verify the index, never trust the status.** An automated orchestrator that indexes on worktree creation and believes `status: indexed` would dispatch agents against a graph that answers nothing, and the only symptom would be tools returning errors mid-dispatch. The integration needs a post-index assertion — node count above zero, `root_path` populated, one real query returning results — before anything depends on it.

Caveat on version: the installed binary here is **0.6.0** and the README documents 0.7.0. Some of this may already be fixed upstream. That is a reason to re-test on the current release, not a reason to assume it away — and the verification step is worth having regardless.

**Re-tested since, on 0.9.0.** The silent-failure mode was not reproduced, and `index_repository` now returns the list of excluded directories plus `expected_nodes` and `expected_edges`, which makes the assertion easier to write. Keep the assertion anyway — 0.9.0 was not tested against a repository carrying the media and committed binaries that broke `basil`. See [§5.7](#57-re-tested-on-090-all-three-defects-survive).

### What worked well

**`trace_path` found something my grep did not.** Tracing inbound callers of `roleSystemPrompt` returned six call sites. Five were in `internal/server/planner.go` and I already knew about them. The sixth was `buildReview` at `internal/server/documents.go:347` — the document-review prompt assembly path, which I had missed entirely when auditing dispatch prompts for the prefix-cache report, because I only grepped `planner.go`.

That is a real, checkable demonstration of the value proposition, on this codebase, found while writing this report. **It also means [prefix-cache-discipline.md](prefix-cache-discipline.md) §6 has an incomplete defect list** — it names `planImplement`, `planCodeReview`, and `planDocument`, and should also consider `buildReview`. That correction is owed regardless of what happens to this plan.

**`search_graph` BM25 mode is good.** A natural-language query — "gate blocks feature transition to review" — returned `FeatureTransition`, `TransitionFeature`, the lifecycle transition functions, and the relevant tests, correctly ranked, with file paths and line ranges. This is meaningfully better than what a keyword grep would produce for the same question.

### What didn't

**`get_architecture` under-delivers.** The README promises "languages, packages, routes, hotspots, clusters, ADRs". On Cromwell, with `aspects: ["all"]` and again with the aspects named individually, it returned only node-label counts and edge-type counts. Nothing about packages, no hotspots, no clusters.

This matters more than it sounds, because an architecture summary is exactly the content the missing project-conventions layer wants (see [context-for-dispatched-agents.md](context-for-dispatched-agents.md) §3). **The capability I would most want to push into implementer prompts is the one that performed worst.**

This is also the finding most likely to be an artefact of the sample. "Packages" is a weakly-defined notion in a flat Go module and a strongly-defined one in a TypeScript workspace with a `package.json` per package; "clusters" and "hotspots" need enough graph to cluster. A 2,794-node repository may simply be below the threshold where any of it is meaningful. Re-test on a real target project before concluding anything.

**`detect_changes` returned nothing, three times.** Once against a docs-only commit range — correct, since no code symbols changed. But also against a range touching 13 Go files, which is not.

The likely explanation is benign: the tool takes a `base_branch` defaulting to `main`, and I was testing *on* `main`, so it was diffing main against itself. Its designed shape is branch-versus-base — which is precisely Cromwell's worktree shape, so the designed configuration is the one I couldn't test from here. But the `since` parameter did not behave as its description implies, and **this tool is untested in the configuration Cromwell would use it in.** That's a specific thing for a spike to settle, and it's the second-most-valuable tool for Cromwell after `trace_path`.

**It indexes Markdown, shallowly.** The largest single node label on Cromwell was `Section`, at 1,104 — Cromwell's own design documents, chopped into headings. The nodes carry heading text but empty file fields, and there's no content retrieval or document-lifecycle awareness.

This overlaps confusingly with Cromwell's own `document_sections` table, which has content, a full-text index, and supersession filtering (see [retrieval-and-document-intelligence.md](retrieval-and-document-intelligence.md) §5). **Cromwell's own document index is much better for documents.** Nobody should reach for the graph to answer a document question, and the overlap is worth stating so nobody tries.

---

## 4. What changes for mixed-language projects

Cromwell's target is not a Go repository. It is a large, mixed-language project — most often TypeScript and JavaScript, usually with other languages alongside. That changes the value case, and mostly in the tool's favour.

### Cross-language is where a graph earns the most

This is the biggest shift, and it inverts the usual scepticism about code graphs.

In a single-language repository, a graph competes against decent native tooling. Go has `go` tooling and a compiler that will tell you what breaks; a well-organised Go repo is also unusually greppable, because the naming is consistent and the imports are explicit. That's the case where "just use grep" is most defensible — and, not coincidentally, it's the case Anthropic optimised Claude Code's agentic search for.

A mixed TypeScript, JavaScript, Python and shell repository has **no single tool that spans it.** `tsc` knows nothing about the Python service. The Python language server knows nothing about the TypeScript client. Grep spans everything and understands none of it. A cross-language graph is the only artefact in the picture that covers the whole repository with structure rather than string matching.

So the argument for a graph is *stronger* in Cromwell's target world than in the repository I tested it on.

### The HTTP boundary is the killer feature, and I under-sold it

> **Corrected by the spike. This section's central claim is wrong — see [§5.3](#53-the-http-boundary-does-not-resolve-on-typescript).** On a real TypeScript monorepo the tool produced **zero** `HTTP_CALLS` edges, on version 0.6.0 and again on 0.9.0 ([§5.7](#57-re-tested-on-090-all-three-defects-survive)). The reasoning below still describes what the capability *would* be worth; the measurement says it is not there for TypeScript. Read the two together.

Indexing Cromwell produced 48 `Route` nodes and 137 `HTTP_CALLS` edges, which I noted in passing and should not have. In a mixed frontend-and-backend project, the call from the TypeScript client to the backend handler **is** the architecture, and it is exactly the edge that every text-based tool loses. Grep cannot follow it. Neither can any single language server.

Three capabilities line up on this, and together they are the strongest reason to keep the plan:

- **`HTTP_CALLS` and `Route` nodes** connect a caller in one language to a handler in another.
- **`trace_path(mode="cross_service")`** follows those edges, through Routes, across the boundary.
- **`ingest_traces`** validates and enriches those edges against real runtime traces, which matters because statically inferring "this fetch call hits that handler" is the least reliable part of the pipeline.

For a code reviewer looking at a task that changed a shared type or an endpoint contract, "what on the other side of the wire depends on this" is the single most valuable question, and the hardest one to answer any other way.

The trap in the paragraph above is that I inferred a cross-language capability from a **single-language** sample. Cromwell's 48 routes are real because Go registers routes as literal strings in a mux, which is exactly the shape a static extractor can read. Nothing about that result predicted TypeScript, and §5.3 shows it did not transfer.

### Dynamic languages resolve worse, so describe the output honestly

TypeScript and JavaScript are harder to resolve statically than Go, and the published 75–89% band understates the variance. Dynamic dispatch, duck typing, runtime `require`, dynamic `import()`, monkey-patching, dependency injection, re-export barrels, and bundler path aliases all defeat static resolution to some degree. JSX adds a component-call convention that isn't a function call at all.

The practical consequence is not "don't use it" — a 75–89% call graph is enormously more useful than no call graph. It is that **the miss rate has to be reflected in how pushed content is described to the agent.** Graph output framed as "the callers of this function" invites an agent to treat absence as proof. Framed as "known callers — the graph resolves most but not all call sites in this language, so verify before relying on completeness", it invites the right scepticism. That distinction costs one sentence in a prompt template and prevents a class of confident-wrong reasoning.

### Monorepos strengthen the one-project recommendation

The TypeScript world is heavily monorepo — pnpm workspaces, Turborepo, Nx. Packages within a monorepo cross-reference constantly, and the interesting questions are precisely the cross-package ones.

That makes the recommendation in §8 — one graph project per repository rather than per worktree — more clearly right, not less. A per-package or per-worktree index would cut exactly the edges worth having.

### Two operational cautions specific to this ecosystem

**Dependency and build directories.** A JavaScript project carries `node_modules`, `dist`, `build`, `.next`, coverage output, and often committed bundles. Indexing those would swamp the graph with code nobody wrote. The tool respects `.gitignore` and `.cbmignore`, which handles the common case, but **verifying what actually got indexed should be part of the spike** — a node count wildly higher than the source file count is the signal.

**Generated and transpiled code.** Generated clients, protobuf output, and `.d.ts` declaration files are real code that resolves fine and is noise in every answer. Worth an ignore rule.

### What this does not change

The integration argument in §8 is language-independent. Push-versus-pull, CLI mode versus MCP client, and the observation that no Cromwell role has a search tool all hold regardless of what the target project is written in.

---

## 5. Running the spike on Tickly

This section reports the spike §9 calls for, run on 2026-08-01 against **Tickly** — a pnpm and Turborepo workspace with a Next.js web app, an Expo mobile app, Cloudflare Workers, and Supabase. It is a project intended to be managed by Cromwell, so it is the right sample in the sense that matters.

Two findings change the plan. The HTTP boundary does not resolve on TypeScript, which removes the strongest argument in §4. And `detect_changes` does not compute a blast radius, which means the highest-value push has to be built from two tools instead of one. Everything else came out better than expected, including call-graph accuracy, which is the thing the published 75–89% band made me most nervous about.

### 5.1 What Tickly is, and what it cannot tell us

Read every number below against this profile.

| Property | Value |
|---|---|
| Source | 15,250 lines across about 50 TypeScript, TSX, SQL, and JavaScript files |
| Documentation | 8,304 lines of Markdown |
| Workspaces | Eight, under `apps/`, `packages/`, `backend/workers/`, and `tools/` |
| On disk | 2.0 GB, of which 1.3 GB is `node_modules` and 498 MB is `.git` |
| Languages | TypeScript, TSX, SQL, JavaScript, TOML, and JSON |

**Tickly has the right shape and the wrong scale.** It is a genuine mixed-language monorepo with a real client-to-worker HTTP boundary, so it tests idioms, extraction, and tool behaviour properly. It is nowhere near the 1.3M-token corpus recorded in [retrieval-and-document-intelligence.md](retrieval-and-document-intelligence.md), so **it says nothing about behaviour at target scale.** Where a finding depends on size rather than shape, I have said so.

Every measurement below was taken on version **0.6.0** and then repeated on **0.9.0**, which shipped on 8 July 2026. Figures are quoted from 0.6.0 unless stated otherwise; [§5.7](#57-re-tested-on-090-all-three-defects-survive) reports what the upgrade changed, which is less than its release notes suggest.

### 5.2 Indexing worked, and the silent-failure risk is still real

Indexing succeeded and produced a queryable graph, in **0.32 seconds**.

```
{"project":"Users-samphillips-Dev-tickly","status":"indexed","nodes":1764,"edges":2268}
```

`list_projects` confirmed a populated `root_path` and 1,764 nodes, `index_status` reported `ready`, and real queries returned real results. The 1.3 GB of `node_modules` was excluded correctly by `.gitignore` alone, with no `.cbmignore` present. Two committed Sketch files over 5 MB and 32 committed PNGs caused no trouble.

**The `basil` failure this report recorded has not healed on its own.** It still sits in the cache as `nodes: 0` with an empty `root_path`, and it still appears in `list_projects`. The post-index assertion §9 asks for remains necessary — the point is not that indexing usually fails, but that when it fails it reports success.

**What did get indexed is noisier than the file count suggests.** Of 98 `Function` nodes, roughly 35 are not project functions:

- **Ambient declarations** from `apps/web/cloudflare-env.d.ts` — `addEventListener`, `btoa`, `atob`, `setTimeout`, `structuredClone`, and three separate nodes named `fetch`.
- **Makefile targets parsed as functions** — `install`, `verify`, `typecheck`, `seed-10k`, `seed-100k`, and two nodes named `.PHONY`.

The `.d.ts` noise is exactly the caution §4 raised about generated and declaration files, and it is worth an ignore rule. As on Cromwell, the largest single label is `Section` at 448 — Markdown headings — which reinforces that the graph is not the tool for documents.

### 5.3 The HTTP boundary does not resolve on TypeScript

This is the finding that changes the case. **Tickly produced zero `HTTP_CALLS` edges**, and its three `Route` nodes were not routes at all:

```
http://127.0.0.1        http://127.0.0.1:3000        https://127.0.0.1:3000
```

All three are strings scraped from `backend/supabase/config.toml` — `api_url`, `site_url`, and `additional_redirect_urls`. They are configuration values for a local development stack.

Tickly has exactly one real cross-service boundary, which makes it a clean test:

- **The caller** is `packages/api-client/src/media.ts:78`, which calls `` fetch(`${mediaSignUrl.replace(/\/$/, "")}/media/sign`, …) ``.
- **The handler** is `backend/workers/media-sign/src/index.ts:45`, a Cloudflare Worker that dispatches on `if (url.pathname !== "/media/sign")`.

Neither side was recognised. The route was not extracted, the call was not extracted, and no edge joined them. Tickly's other real route — a Next.js App Router file at `apps/web/src/app/.well-known/apple-app-site-association/route.ts` exporting `GET` — produced a `Function` node and no `Route` node.

Both idioms are ordinary modern TypeScript. A template-literal URL built from a configured base defeats static extraction of the call; an imperative `pathname` comparison and file-based routing defeat extraction of the route.

**Checking the other indexed projects shows what the extractor actually keys on.** All three figures below are from 0.9.0, after the re-test in §5.7.

| Project | Language | Has an HTTP server | Route nodes | Real routes | `HTTP_CALLS` |
|---|---|---|---|---|---|
| Cromwell | Go | Yes, `mux` with literal paths | 60 | 45 of 60 | 149 |
| kanbanzai | Go | No, it is a command-line tool | 33 | 0 of 33 | 36 |
| Tickly | TypeScript | Yes, Worker and App Router | 0 | 0 of 0 | 0 |

The `Route` extractor **scrapes path-shaped string literals.** Where a project registers routes as literal strings, as Cromwell's Go `mux` does, the real ones dominate and the output is genuinely good — `/api/features`, `/ui/work`, and so on. Where a project has no HTTP server at all, as kanbanzai does not, every `Route` node is a false positive harvested from test fixtures, such as `/nonexistent/path/role.yaml`. And where routes exist but are not literal-string registrations, as in Tickly, **nothing is extracted at all.**

So the failure is not that TypeScript resolves badly. It is that **the extractor recognises one idiom — the literal route string — and TypeScript's common idioms are not it.** A Worker comparing `url.pathname`, and a file-based App Router where the path is the directory name, both fall outside it.

**The cross-language edge is therefore weakest exactly where §4 argued it was strongest**, and §5.7 confirms this is still true on 0.9.0.

Two consequences follow. `trace_path(mode="cross_service")` has nothing to traverse, and returns results identical to `mode="calls"`. And `ASYNC_CALLS` and `DATA_FLOWS` are zero on all three projects, so `mode="data_flow"` also returns the same thing and never shows the argument expressions its description promises.

### 5.4 `detect_changes` needs a per-worktree index, and is not a blast radius

Two separate defects, both load-bearing for the code-review push.

**It ignores `repo_path` and anchors to the indexed project's stored `root_path`.** Running it from inside a feature worktree, against a repository-level project, returned the branch diff of the *main checkout* — a different branch entirely. It reported 13 changed files belonging to `feat/tickly-org-front-door` when the worktree was on `spike/passkey-mobile`. Passing `repo_path` explicitly changed nothing. There is no error; the answer is simply wrong.

For Cromwell this is severe. A reviewer dispatched into worktree X would receive the blast radius of whatever branch the main checkout happens to sit on, and nothing in the output would reveal it.

Indexing the worktree as its own project fixes it completely, and costs **0.22 seconds**. The correct five changed code files then appeared. **This reverses §8's recommendation of one graph project per repository, at least for `detect_changes`.**

**`impacted_symbols` is not a blast radius.** Of 53 impacted symbols reported, **all 53 were declared inside the changed files and none were outside them.** The `depth` parameter is inert — 1, 2, 4, and 6 all return the same 53.

The graph *does* hold the answer. The change touched `packages/api-client/src/auth.ts`, which defines `createAuth`. Asking `trace_path` who calls `createAuth` returns `createTicklyClient` and `Home` — and `Home` lives in `apps/web/src/app/page.tsx`, which the change does not touch. That is precisely the downstream dependent a reviewer needs, the graph knows about it, and `detect_changes` omits it.

So `detect_changes` is `git diff` plus a parser for the symbols it declares. Useful, but not what §8 assumed. **The blast-radius push must be composed:** take the changed files from `detect_changes`, then run `trace_path` inbound on each declared symbol.

### 5.5 Call-graph accuracy is good, but the default hides half of it

This came out better than the published 75–89% band led me to expect, once the tools are called correctly.

Comparing `trace_path` against the raw `CALLS` edges in the graph, for every function where ground truth was checkable:

| Function | `CALLS` in-edges | Default call | With `include_tests` | Missed |
|---|---|---|---|---|
| `createFeed` | 3 | 1 | 3 | None |
| `createAuth` | 2 | 1 | 2 | None |
| `encodeCursor` | 3 | 1 | 3 | None |
| `decodeCursor` | 3 | 1 | 3 | None |
| `handleMediaSign` | 2 | 1 | 2 | None |
| `createMediaResolver` | 2 | 1 | 2 | None |
| `mapRpcError` | 2 | 1 | 2 | None |

**With `direction: "inbound"` and `include_tests: true`, `trace_path` recovered every edge.** Nothing was missed.

**The defect is the default.** `include_tests` defaults to `false`, which cut every caller set to a single entry. The output carries no marker saying results were filtered, so the omission is invisible unless you have read the tool schema. For a reviewer asking what depends on a changed function, tests are among the most important dependents — they are the code most likely to need updating.

Cross-package tracing works properly. `createMediaResolver` resolves back through `createTicklyClient` to `Home`, which crosses two workspace packages and a `.ts` to `.tsx` boundary, and matches the source exactly.

Two granularity warts are worth knowing before building on this:

- **Calls from module scope or anonymous callbacks are attributed to a file pseudo-node.** A caller list can contain `packages/api-client/src/cursor.test.ts` where a function name is expected, because the call sits inside an `it(...)` block rather than a named function.
- **Duplicate function names are ambiguous.** Tickly has two functions called `makeStub`, and `trace_path` takes a bare name.

An honest summary for a prompt template: the call graph is accurate for what it captures, its granularity degrades to file level inside callbacks, and it is only complete if you ask for tests.

### 5.6 Search, architecture, and cost

**BM25 search is good, and this now holds for TypeScript as well as Go.** The query `"sign media url for playback"` returned the media-signing cluster correctly ranked — `mediaProviderFor`, `expiryForKind`, `signStreamJwt`, `handleMediaSign`, `createCloudflareMediaProvider`. Regex `name_pattern` also works, returning file paths and in-degree and out-degree.

**Semantic search is not usable.** Results appear in a separate `semantic_results` field rather than `results`, which is easy to miss. Once read correctly, the quality is poor: `["rate limiting"]` returned mobile layout components and missed `backend/workers/media-sign/src/rate-limit.ts` entirely, and scores sat around 0.006. Only 102 functions had vectors stored, so this may be a small-corpus effect — but on this project it returns noise.

**`get_architecture` really is that thin.** §3 hoped its weakness was a Go artefact and asked for a re-test where packages are well defined. On an eight-package pnpm workspace, with `aspects: ["all"]`, it returned **only node-label counts and edge-type counts** — no packages, no routes, no hotspots, no clusters. That question is now settled, and the project-conventions push must be built from `search_graph` and `trace_path` output instead.

**The token economics are strongly favourable.** A composed blast-radius push covering four changed symbols came to 602 characters, roughly 150 tokens. Reading the five files an agent would otherwise open costs about 6,029 tokens. That is **40 times cheaper**, and it is the concrete evidence §9 asked for.

### 5.7 Re-tested on 0.9.0: all three defects survive

The spike first ran on 0.6.0. Because the 0.9.0 release notes named fixes for two of the failures above, the binary was upgraded and the three tests that could change the recommendation were re-run. **None of them changed.**

| Test | 0.6.0 | 0.9.0 | Verdict |
|---|---|---|---|
| HTTP boundary resolves | 0 `HTTP_CALLS`, 3 junk routes | 0 `HTTP_CALLS`, **0** routes | Noise removed, signal still absent |
| `detect_changes` honours the worktree | Ignored, wrong branch | Ignored, wrong branch | Unchanged |
| `impacted_symbols` traverses | 0 outside, `depth` inert | 0 outside, `depth` inert | Unchanged |

**The route fix removed noise rather than adding signal.** Tickly's three junk `Route` nodes are gone, which is what "reject filesystem paths" promised. But no real route replaced them, and no node anywhere in the graph now mentions `media/sign` or `apple-app-site-association`. The Worker route and the App Router route are simply not seen. `HTTP_CALLS` remains zero, so `mode="cross_service"` still returns a plain `calls` trace.

**The filesystem-path rejection is also incomplete, and Cromwell regressed.** kanbanzai, re-indexed on 0.9.0, still carries all 33 junk filesystem-path routes. Cromwell's junk rose from 2 to 15, because JavaScript regular-expression literals in its htmx handling are now parsed as routes:

```
/HX-Push:/i   /HX-Trigger:/i   /HX-Retarget:/i   /HX-Trigger-After-Settle:/i
```

**`detect_changes` behaves identically.** Run from inside the `spike/passkey-mobile` worktree against a repository-level project, with and without an explicit `repo_path`, it returned 16 changed files of which 9 belong to `apps/site` — the main checkout's branch — and none to the worktree's branch. Per-worktree indexing is still the only configuration that gives correct answers, and it still works.

**`impacted_symbols` behaves identically.** With a per-worktree index the correct branch is found, but all 62 impacted symbols were declared inside the changed files, none outside, at every `depth` tested.

**What did improve.** Extraction is richer and indexing is faster. Tickly went from 1,764 to 1,886 nodes and from 2,268 to 2,885 edges, with `CALLS` up from 84 to 131 and `IMPORTS` from 31 to 89. kanbanzai re-indexed in **5.5 seconds** for 78,884 nodes. Three things are genuinely better for Cromwell:

- **`index_repository` now reports what it excluded**, listing the directories skipped and returning `expected_nodes` and `expected_edges` alongside the actual counts. That is a built-in version of the verification step §9 asks for, and it answers the "what actually got indexed" question directly.
- **BM25 search improved**, now also finding `signImage` and `signVideo` for the media query.
- **The CLI no longer wraps results in an MCP envelope**, returning raw JSON. Anything parsing 0.6.0 output will need updating.

The positive findings in §5.5 all hold: `trace_path` still recovers 100% of `CALLS` in-edges, and `include_tests` still defaults to `false` and still halves every caller set.

**So the three defects are properties of the tool, not of an old build.** The design should be made on that basis.

### 5.8 What the spike answers

| Question from §9 | Answer | Still true on 0.9.0 |
|---|---|---|
| Does it index into something queryable? | Yes, in 0.32 s. But `basil`'s silent failure sat in the cache for months, so keep the assertion | Yes, and 0.9.0 now reports exclusions and expected counts itself |
| Does `detect_changes` work in a worktree against base? | Only with a per-worktree index. It ignores `repo_path` and returns the wrong branch otherwise | Yes, unchanged |
| Does the HTTP boundary resolve? | No. Zero `HTTP_CALLS`, and the route idioms are not recognised | Yes. Junk routes removed, but still no real ones |
| Is `get_architecture` genuinely this thin? | Yes. Confirmed on a monorepo where packages are well defined | Yes, unchanged |
| How good is TypeScript resolution? | Good — 100% of `CALLS` edges recovered, but only with `include_tests: true` | Yes, and extraction is richer |
| What actually got indexed? | `node_modules` excluded correctly. About 35 of 98 function nodes are `.d.ts` and Makefile noise | Yes, but 0.9.0 now lists what it excluded |

---

## 6. Does Claude make it unnecessary?

No — and the answer is close to the opposite.

**Anthropic ships no codebase index, deliberately.** Claude Code uses agentic search: Glob, Grep, Read. The stated reasoning is the same set of arguments recorded in the retrieval report — precision over fuzzy embedding matches, no index to build or maintain, no drift between index and code during active editing, and no data leaving the machine. Anthropic removed a vector index from Claude Code and has not replaced it.

So there is no Anthropic feature that overlaps here. What Anthropic has is a *bet against* this category, and the ecosystem's response has been to add code intelligence back as MCP servers rather than wait.

Three things could be behind the impression that Claude now has something similar, and they're worth separating:

**The `codebase-memory-*` skills in Claude Code are codebase-memory-mcp's own.** Its installer configures 37 coding agents and installs skills and hooks that nudge the agent toward graph queries instead of grep. If those skills are visible in a Claude Code session, that is this tool already installed — not an Anthropic feature. Worth checking before concluding anything is redundant.

**Claude Code's Explore subagent and agentic search** are genuinely good at codebase questions, and for a chat-phase agent they may be sufficient. That's a real overlap, but only for the chat phase, and only when the model is strong (see §8).

**Anthropic's tool search and memory features** solve adjacent problems — finding the right *tool schema*, and persisting notes across sessions. Neither indexes code.

One consequence follows directly from Cromwell's provider constraint. Because Anthropic ships nothing here, **any capability Cromwell gets from Claude Code's agentic search evaporates the moment a project runs on a cheaper model.** A local, provider-neutral index does not.

---

## 7. Alternatives

The category has become crowded. The landscape divides into four kinds:

| Kind | Examples | What it gives | What it lacks |
|---|---|---|---|
| **Search servers** | Claude Context, Local Code Search | BM25 and embedding search over code | No structural analysis — can't answer "what calls this?" |
| **Knowledge-graph servers** | codebase-memory-mcp, GitNexus, CodeGraphContext | Call graphs, transitive queries, impact analysis | Index to build and keep fresh |
| **LSP-backed servers** | Serena | Precise definition and reference lookup via language servers | Requires running LSPs; no persistent graph or memory |
| **Hybrid** | Sverklo, code-review-graph | Search plus structure plus ranking plus memory | More moving parts |

Three considerations decide it for Cromwell.

**Licence is a hard filter.** Cromwell ships a starter pack into other people's projects, so anything with a non-commercial licence is disqualified. **GitNexus is PolyForm Noncommercial** — out. codebase-memory-mcp, Serena, and CodeGraphContext are MIT.

**Deployment friction is a hard filter too.** Cromwell's orchestrator would be invoking this from Go on a user's machine as part of an automated loop. Anything needing Neo4j, Docker, a language-server fleet, or an API key adds an install step to every Cromwell project. codebase-memory-mcp's single static binary with no runtime dependencies is close to the ideal shape here; **Serena's LSP dependency is its main cost** in this context, since it means the relevant language servers must be present and running on whatever machine the orchestrator lives on.

**Structural queries are the point.** A pure search server doesn't answer the question Cromwell most wants answered — what does this change touch? That rules out the search-only tier for the code-review use case, though such a tool would serve the chat phase fine.

On those three filters, codebase-memory-mcp remains the best fit, and the reasoning that put it in the vision has strengthened rather than weakened.

**Serena is the credible alternative, and a mixed TypeScript world pulls it in both directions at once.** Its precision advantage grows: `tsserver` is an outstanding language server, and for TypeScript specifically it will resolve types, generics, and re-exports far more accurately than tree-sitter plus a lightweight type layer. If resolution quality turns out to be the binding constraint, that is the escape hatch.

But its install cost grows faster. A mixed project needs **one language server per language**, all present and running on whatever machine the orchestrator lives on — `tsserver` plus Node for the TypeScript, a Python server, and whatever else the repo contains. For an automated loop running across many user projects, that is a per-project, per-language provisioning problem rather than a one-time install. Against that, a single vendored binary covering 158 languages is doing something Serena structurally cannot.

There's also a coverage asymmetry worth naming: **an LSP-based tool is precise within a language and blind between them.** The HTTP boundary from §4 — the TypeScript client calling the backend handler — is invisible to every language server involved, because no single server sees both sides. That is the query Cromwell's reviewers most need, and it is the one only a cross-language graph answers.

Honest caveat: comparison articles in this space read as substantially promotional, several appear to be written by the tools' own authors, and the benchmark numbers across them are not comparable. I've weighted the licence, dependency, and capability facts, which are checkable, over the performance claims, which mostly are not.

---

## 8. The integration is backwards

This is the substantive recommendation.

### What the plan proposes

Vision §8 has Cromwell act as an MCP client, re-exposing `search_graph` to dispatched agents as a curated tool with the project name injected from the worktree record. The agent decides when to search.

**That is precisely the mechanism that failed in the predecessor.** The clearest evidence in the whole research corpus is the sub-agent propagation failure recorded in [context-for-dispatched-agents.md](context-for-dispatched-agents.md) §1: several agents were dispatched in parallel, all of them had a codebase knowledge graph available — indexed, ready, and faster for the structural questions they were asking — and *none of them used it*. Every one fell back to grep and reading files.

The tool being planned is the same tool. The failure is documented, first-hand, and specific to it.

### Three things have changed since

**Cromwell's orchestrator is code.** It cannot decline to run a query. This is the same argument that resolved the retrieval question, and it applies identically here.

**CLI mode removes the client requirement — without giving up the index.** Every tool runs as a one-shot command against the same on-disk SQLite cache, in tens of milliseconds warm. Vision §8's "Cromwell server is an MCP client for external services" is more architecture than the job needs: the orchestrator can shell out, parse JSON, and put the result in a prompt.

The positive form of the argument is stronger than the negative one. **Cromwell is already a coordinator.** The daemon exists to arbitrate between interactive agent sessions that don't know about each other — deciding who runs the watcher, sequencing overlapping indexing jobs, owning the UI. Cromwell has none of that problem: it knows precisely when a worktree is created, when a feature merges, and when the graph should be refreshed, because it is the thing causing those events. Running a second coordinator underneath it — one that requires every participating process to agree on version, build, coordination ABI, and cache root — adds a synchronisation surface to replace one Cromwell already owns.

The one capability genuinely lost is the background file watcher that auto-reindexes on git changes. Cromwell should not want that: an index that mutates underneath a running dispatch is a source of non-determinism, and deciding *when* the graph is refreshed is exactly the kind of thing a deterministic orchestrator should hold.

**No Cromwell role has any search tool at all.** This is the finding that most changes the calculus, and it cuts the other way. Every starter role's profile is some subset of `read_file`, `list_files`, `edit_file`, `write_file`, `run_command`:

| Role | Tools |
|---|---|
| implementer | read_file, list_files, edit_file, write_file, run_command |
| code-reviewer, design-reviewer, spec-author, dev-plan-author | read_file, list_files |
| verifier | read_file, list_files, run_command |
| estimator, spec-reviewer, dev-plan-reviewer | none |

There is no grep. So in Cromwell, `search_graph` would not be *competing* with grep — it would be the only search tool an agent has. The predecessor's failure mode was agents preferring a familiar tool over an unfamiliar one; that specific failure cannot occur where the familiar tool is absent.

What happens instead is worse: an agent that needs to find something can only `list_files` and then `read_file` repeatedly, which is the most expensive strategy available and the one the token measurements are worst for.

### So do both, in the right order

**Push first, because it always works.** The orchestrator queries the graph and puts the answer in the dispatch prompt. Deterministic, auditable, identical for identical inputs, and it costs the agent no turns. The two highest-value pushes:

- **For code review**: the blast radius of the task's diff. `detect_changes` from inside the feature worktree against the base branch is designed for exactly this, and the reviewer's 15-turn budget is the tightest in the system.
- **For implementation**: callers and callees of the symbols the task names, so the implementer starts knowing what its change touches instead of discovering it.

**Amended by the spike.** `detect_changes` alone does not produce a blast radius — it returns only the symbols declared inside the changed files, and its `depth` parameter does nothing ([§5.4](#54-detect_changes-needs-a-per-worktree-index-and-is-not-a-blast-radius)). The code-review push is therefore a **composition of two calls**: take the changed files from `detect_changes`, then run `trace_path` inbound on each declared symbol, with `include_tests: true`. Measured on Tickly, the composed result costs about 150 tokens against roughly 6,029 to read the same files — so the push is worth building, it is just two steps rather than one.

**Then pull, because the roles currently have nothing.** Add a narrow search tool to the read-only roles. Keep the schema small — a query string, a limit — both because one tool should do one thing and because DeepSeek silently drops arguments from wide or deep schemas.

Note this reverses the deferral logic slightly. DESIGN-006 argued `search_graph` "makes it cheaper and faster, which matters at scale but proves nothing new". That was right for phase 2. At the target-project scale recorded in [retrieval-and-document-intelligence.md](retrieval-and-document-intelligence.md) §2.4 — hundreds of files, where an implementer's only alternative is reading them one at a time — cheaper and faster stops being an optimisation and becomes the difference between a task fitting inside its turn budget and not.

### Per-worktree graph projects: a free choice, not a constraint

This was the operational sticking point when the predecessor integrated the tool, so it deserves the measured answer rather than an architectural one.

**The cost objection has evaporated.** A full index of a 204,000-line project takes 1.7 seconds. Indexing on worktree creation would add less time than the `git worktree add` it follows. Any design bent out of shape to avoid re-indexing is avoiding a cost that no longer exists at this scale.

What remains is not a performance argument but a correctness one, and it points the same way:

**You usually don't want the worktree's graph — you want the repository's.** The questions an agent asks are about the code that exists: what calls this, what does this touch, how is this package arranged. The worktree's delta is the thing being *written*, and the authoritative account of that is the git diff, which `detect_changes` derives from git rather than from the index. Indexing the worktree to learn about the worktree's own changes is the long way round.

**Duplication is now a storage question rather than a time one.** N worktrees means N near-identical databases — 35 MB each on the project measured — plus N teardown paths to get wrong.

So the recommended shape is unchanged, but for better reasons: **one graph project per repository**, refreshed when Cromwell decides, with per-worktree deltas answered by `detect_changes(base_branch=…)`. The `worktrees.graph_project` column survives holding a repository-level name, and the teardown path disappears.

**And if a per-worktree index is wanted anyway** — so an implementer can trace into code it has just written — it is now affordable enough to be a per-project configuration rather than an architectural decision. That is a much more comfortable position than the predecessor was in, and it is worth revisiting only once the base integration is proving its value.

> **Reversed by the spike — index per worktree.** The paragraphs above rest on `detect_changes(base_branch=…)` answering the per-worktree delta against a repository-level project. It does not. **`detect_changes` ignores `repo_path` and anchors to the indexed project's stored `root_path`**, so called from a worktree against a repository-level project it silently returns the *main checkout's* branch diff ([§5.4](#54-detect_changes-needs-a-per-worktree-index-and-is-not-a-blast-radius)). Indexing the worktree as its own project fixes it, costs 0.22 seconds on Tickly, and is the only configuration measured to give correct answers. The cost argument for per-repository indexing was never strong; the correctness argument has now flipped to the opposite side. Keep `worktrees.graph_project` as a genuinely per-worktree name, and keep the teardown path.

The remaining unknown is whether the committed shared artefact (`.codebase-memory/graph.db.zst`, 8–13:1 compression) plus incremental indexing works cleanly across differing root paths. Given a 1.7-second cold index, that mechanism has become an optimisation rather than a necessity — worth knowing about, not worth blocking on.

---

## 9. Recommendation

**Keep it on the roadmap, on narrower grounds. Reshape the integration. The spike has been run, on 0.6.0 and again on 0.9.0, and the answers are in.**

### Keep, because the case still holds — on narrower grounds

The tool is more capable than when the plan was written, it is MIT-licensed, it is a single binary with no API keys and no network calls, and it is **provider-neutral** — which matters specifically because Cromwell will not always run on Claude, and because Anthropic ships nothing in this category. It demonstrably found a call site this project's own maintainer missed.

**The spike narrowed the case without breaking it.** The cross-language HTTP boundary — the argument §4 rated strongest — does not work on TypeScript, on either version tested ([§5.3](#53-the-http-boundary-does-not-resolve-on-typescript), [§5.7](#57-re-tested-on-090-all-three-defects-survive)). What survives is still worth having, and is now measured rather than assumed: an accurate within-language call graph that crosses file, package, and workspace boundaries; good BM25 search; and a blast-radius push that costs about 40 times less than the reading it replaces ([§5.5](#55-call-graph-accuracy-is-good-but-the-default-hides-half-of-it), [§5.6](#56-search-architecture-and-cost)).

The case now rests on **cross-package** rather than **cross-language** structure. That is a smaller claim, and the one the evidence supports.

### What is settled, and what would change it

Re-tested on 0.9.0 and unchanged, so treat these as properties of the tool:

- **The HTTP boundary does not resolve on TypeScript.** Build nothing that depends on it.
- **`detect_changes` ignores the worktree.** Index per worktree.
- **`impacted_symbols` does not traverse.** Compose the push from two calls.
- **`get_architecture` returns only counts**, and semantic search returns noise.

**What would reopen the HTTP question** is route extraction that recognises a non-literal registration — a Worker dispatching on `url.pathname`, or file-based routing where the path is the directory name. That is a different feature from the filesystem-path rejection 0.9.0 shipped, so watch for it rather than assuming the next release brings it.

One thing genuinely improved and is worth using: **`index_repository` on 0.9.0 reports the directories it excluded and returns `expected_nodes` and `expected_edges`**, which is most of the verification step below.

### Then build it in this order

1. **Orchestrator-side, CLI mode, pushed into prompts.** Start with the code-review blast radius, composed as `detect_changes` for the file list, then `trace_path` inbound per declared symbol. Pass `include_tests: true` and set `direction` explicitly — the defaults silently drop most of the answer.
2. **A narrow search tool for the read-only roles.** They currently have nothing, and something beats nothing. Use the BM25 `query` mode, which works well; do not expose `semantic_query`.
3. **One graph project per worktree**, indexed on worktree creation. It costs a fraction of a second, and it is the only configuration in which `detect_changes` gives correct answers.

### Describe the output honestly in the prompt

§4 asked for one sentence of framing. The spike says what it has to contain: the call graph is accurate for what it captures, **it omits test callers unless asked for them**, its granularity falls back to the file when a call sits inside a callback, and **it does not cross HTTP boundaries at all in TypeScript**. An agent told "these are the callers" will read absence as proof, and on the HTTP boundary that inference would be wrong every time.

### Don't

- **Don't make Cromwell an MCP client for it.** CLI mode is enough and is far less machinery.
- **Don't use it for documents.** Cromwell's own `document_sections` is better on every axis that matters. The graph's Markdown sections are structural only.
- **Don't use `manage_adr`.** Cromwell has decision records as first-class entities with a lifecycle. A second, parallel decision store in a cache directory is the kind of divergence that produces two answers to the same question.
- **Don't make it a hard dependency.** Every role must remain able to do its job without it, as they do today. It should degrade to absent, not to broken.
- **Don't rely on `semantic_query`, `mode="cross_service"`, or `mode="data_flow"`.** On both 0.6.0 and 0.9.0 the first returns noise and the other two return the same thing as a plain `calls` trace, because `HTTP_CALLS`, `ASYNC_CALLS`, and `DATA_FLOWS` are empty.
- **Don't trust `status: indexed`.** Assert node count, `root_path`, and one real query. On 0.9.0, compare the returned `expected_nodes` and `expected_edges` against the actuals as well.
- **Don't parse the CLI output format loosely.** 0.9.0 dropped the MCP content envelope and returns raw JSON, so anything written against 0.6.0 breaks silently on upgrade. Pin the version Cromwell expects and handle both shapes.
- **Don't run the tool's own installer, and never ship it in the starter pack.** Invoke the binary directly. See below.

### The installer is opinionated, and Cromwell must not inherit that

Upgrading surfaced something the earlier testing had not. `codebase-memory-mcp install` and `update` reconfigure **every coding agent they detect** — on this machine, Claude Code, Codex CLI, Gemini CLI, Zed, and VS Code — writing MCP entries, instruction files, skills, and hooks into each.

In Claude Code specifically it leaves a `PreToolUse` hook matching `Grep|Glob|Read|Search`. It blocks the **first** such call in a session, writes a sentinel keyed on the process ID, and allows everything afterwards — so the cost is one refused call per session, not a tax on every read. That single block landed during this spike on a plain read of a Markdown file, which is the real complaint: the matcher covers `Read`, so the one nudge you get can be spent pushing you toward a graph that holds no document content at all.

It also adds a `SessionStart` reminder on four matchers — `startup`, `resume`, `clear`, and `compact` — which injects about fifteen lines instructing the agent to prefer graph tools and to read source with `get_code_snippet` rather than `Read`. Those are four distinct events, not duplicate registrations, so the reminder fires once per event as any hook would.

On this machine the vendor gate was replaced with a user-owned equivalent scoped to `Grep|Glob`, keeping the once-per-session nudge for code search while leaving `Read` alone. The reminder was kept as installed.

**For Cromwell this is a firm constraint.** Cromwell ships a starter pack into other people's projects, and a dependency whose installer rewrites the user's agent configuration — across Claude Code, Codex, Gemini, Zed, and VS Code — is not something to bundle. Call the binary, and leave the installer alone. Note too that an `update` may restore what you remove.

This also cuts against the pull half of §8's recommendation in a small way. The tool's own answer to "agents ignore the graph" is a hook that blocks the alternatives — which is the blunt version of the problem [context-for-dispatched-agents.md](context-for-dispatched-agents.md) §1 records. Cromwell's answer is better: push the content, and let the tool stay optional.

### Corrections owed elsewhere

[prefix-cache-discipline.md](prefix-cache-discipline.md) §6 lists three prompt-assembly sites with the volatile-before-shared defect. `buildReview` in `internal/server/documents.go:347` is a fourth and should be checked.

---

## 10. Sources

### Primary

- [github.com/DeusData/codebase-memory-mcp](https://github.com/DeusData/codebase-memory-mcp) — README: architecture, tool surface, language support, performance, security posture, installation.
- [LICENSE](https://github.com/DeusData/codebase-memory-mcp/blob/main/LICENSE) — MIT.
- *Codebase-Memory: Tree-Sitter-Based Knowledge Graphs for LLM Code Exploration via MCP* — arXiv:2603.27277. 31 repositories, 83% answer quality, 10× fewer tokens, 2.1× fewer tool calls.
- **Direct testing, 2026-08-01, binary version 0.6.0.** Indexed Cromwell (2,859 nodes / 8,015 edges) and a 204,463-line Go project both as checked out (1.7 GB, failed silently) and source-only (6.2 MB, 1.7 s, 7,710 nodes, queryable). Exercised `list_projects`, `index_repository`, `get_architecture`, `search_graph`, `trace_path`, `query_graph`, `get_graph_schema`, and `detect_changes`, over both the MCP surface and the one-shot CLI. All findings in §3 are first-hand; note the README documents 0.7.0.
- **The Tickly spike, 2026-08-01, binary version 0.6.0.** Indexed Tickly (1,764 nodes / 2,268 edges in 0.32 s) and a `spike/passkey-mobile` worktree of it as a separate project (1,273 nodes in 0.22 s). Ground truth for every call-graph claim was read from the source. Route and `HTTP_CALLS` counts were cross-checked against the cached kanbanzai and Cromwell graphs. All findings in §5 are first-hand. The worktree and its scratch graph project were removed afterwards; Tickly's repository was left unchanged.
- **The 0.9.0 re-test, 2026-08-01.** Upgraded 0.6.0 to 0.9.0 and re-ran the three tests that could change the recommendation. Re-indexed Tickly (1,886 nodes, 0.44 s), kanbanzai (78,884 nodes, 5.5 s), and Cromwell (3,038 nodes, 0.76 s) on the new binary, then repeated the HTTP-boundary, `detect_changes`, and `impacted_symbols` tests against a fresh `spike/passkey-mobile` worktree. All three defects reproduced. First-hand; see §5.7.
- [Release v0.9.0](https://github.com/DeusData/codebase-memory-mcp/releases/latest) — published 8 July 2026. Names route and HTTP extraction fixes ("reject filesystem paths so client routes join server routes", parameterised path canonicalisation) and `detect_changes` honouring `since`. **Tested: the route change removed junk nodes but added no real TypeScript routes, and the `detect_changes` behaviour Cromwell depends on is unchanged.** A useful reminder that a changelog entry naming a defect is not evidence that the defect a downstream consumer cares about is gone.

### On Claude's position

- [Claude Code doesn't index your codebase — here's what it does instead](https://vadim.blog/claude-code-no-indexing/) — the agentic-search rationale: precision, simplicity, freshness, privacy.
- [zilliztech/claude-context](https://github.com/zilliztech/claude-context) — the ecosystem's response: semantic code search added back as an MCP server.

### Alternatives

- [Best MCP servers for code intelligence: honest comparison of 12 options](https://sverklo.com/blog/practical-guide-mcp-code-intelligence/) — the four-category framing, licence differences, and stated trade-offs. Note the author ships one of the compared tools.
- [Code graph MCP tools compared](https://www.saurabhsharma.dev/blogs/code-graph-mcp-tools-comparison/) and [the DEV write-up](https://dev.to/coder11/code-review-graph-vs-graphify-vs-codebase-memory-mcp-the-best-code-intelligence-mcp-tools-for-ai-3ea) — comparisons of code-review-graph, Graphify, and codebase-memory-mcp. Both read as promotional; treat the numbers as unverified.

### Cromwell documents and code

- `docs/vision/vision-v1.md` §8 — per-worktree graph projects, implicit context injection, the MCP-client posture.
- `docs/design/DESIGN-006-tool-host-and-worktrees.md` §8 — the deferral and its reasoning.
- `docs/specs/SPEC-002-phase-2-implementation-loop.md` SD-1 — the scope decision.
- `internal/config/compartment.go:22` — `search_graph` as a named-but-unimplemented tool.
- `internal/store/migrations/0002_tasks_worktrees.sql:36` — the always-NULL `graph_project` column.
- `internal/starter/pack/roles/*.yaml` — the tool profiles showing no role has any search tool.
- [context-for-dispatched-agents.md](context-for-dispatched-agents.md) §1 — the predecessor's first-hand failure with this exact tool.
- [retrieval-and-document-intelligence.md](retrieval-and-document-intelligence.md) §5 — Cromwell's own document index, which the graph should not duplicate.
