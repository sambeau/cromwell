# Retrieval and document intelligence

Should Cromwell have a retrieval-augmented generation system for its own documents?

The short answer is that **it already has most of one, and has had since the first migration.** Cromwell parses every submitted document into sections, stores them in Postgres with a generated full-text vector and a GIN index, and ships a working section-level search function with snippet generation. That function is wired to one thing: the search box in the command centre. No agent, anywhere, can reach it.

So the real question isn't whether to build retrieval. It's what to connect the retrieval that exists to, and whether to add anything on top.

This document covers the current state of the field, what Claude ships built-in, an audit of what Cromwell already has, a re-examination of why the predecessor's document intelligence system failed, and a recommendation.

The lens throughout is **document intelligence** — designs, specifications, dev-plans, decisions, reviews, notes. Codebase knowledge is a separate problem with a separate planned answer (`codebase-memory-mcp`) and is out of scope here.

---

## Table of contents

1. [The short answer](#1-the-short-answer)
2. [Where retrieval has got to](#2-where-retrieval-has-got-to)
3. [The agentic-search counter-argument](#3-the-agentic-search-counter-argument)
4. [What Claude ships built-in](#4-what-claude-ships-built-in)
5. [What Cromwell already has](#5-what-cromwell-already-has)
6. [Was the predecessor's failure the orchestrator's fault?](#6-was-the-predecessors-failure-the-orchestrators-fault)
7. [Two phases, two architectures](#7-two-phases-two-architectures)
8. [Recommendation](#8-recommendation)
9. [What this does not replace](#9-what-this-does-not-replace)
10. [Sources](#10-sources)

---

## 1. The short answer

**Yes, but not the kind that failed before.**

The thing that failed twice was an *opt-in knowledge subsystem* — a store agents had to choose to query, with its own contribution lifecycle and confidence scoring. Agents ignored it and searched the files instead. That verdict stands and nothing in the current research overturns it.

What's being proposed here is different in three ways that matter:

- **The retrieval already exists.** Section indexing, full-text vectors, ranked search with snippets — built, tested, running. This is activation, not construction.
- **The orchestrator does the retrieving, not the agent.** Cromwell's orchestrator is code. Code doesn't forget to search. That removes the failure mode entirely, and it's a capability the predecessor never had.
- **Results are pushed into the prompt, not offered behind a tool.** The one rule the predecessor's failure established.

The recommendation, in one line: **wire the existing section search into dispatch prompt assembly for the execution path, expose it as a narrow tool to the chat agent, and make retrieval quality a per-project configuration — because the corpus that matters is the target project's, and those span three orders of magnitude.**

### Two constraints that shape everything below

**The corpus is the target project's, not Cromwell's.** Cromwell exists to build software larger than itself. A real project of the kind it's aimed at runs to 619 Markdown files and 992,000 words — about **1.3 million tokens**, of which 781,000 words are workflow documents. That is 7.5× Cromwell's own corpus, 6.6× over the threshold below which Anthropic says skip retrieval, and larger than a full 1M context window. Retrieval at that size isn't a cost optimisation; it's the only way the content is reachable at all.

**The model won't always be Claude.** As cheaper models become good enough, Cromwell will run both phases on them. That has one consequence more important than any other in this document: **agentic search degrades sharply with model capability, and pre-computed retrieval barely degrades at all.** The cheaper the model, the more retrieval must be done *for* it rather than *by* it.

---

## 2. Where retrieval has got to

### 2.1 The reframing: from RAG to context engineering

The field has stopped treating retrieval as a question-answering pattern and started treating it as infrastructure for assembling an agent's context. The phrase that's replaced "RAG" is **context engineering**, and the reframing identifies three retrieval problems that used to be considered separate:

1. **Domain knowledge retrieval** — the classic case; find the relevant documents.
2. **Tool retrieval** — with hundreds of available tools, which schemas belong in this request's context?
3. **Memory retrieval** — conversation history, prior decisions, accumulated state.

The argument is that these are one problem wearing three hats, and that a system which coordinates all three beats three isolated systems. Anthropic's own product surface bears this out: tool search and the memory tool arrived alongside document retrieval, all solving "get the right thing into the window at the right moment".

### 2.2 The pipeline that still wins

Despite the reframing, the measured retrieval stack has been stable for over a year and the numbers are consistent across benchmarks.

**Hybrid beats either half alone.** Keyword search (BM25 or Postgres `ts_rank`) and dense embedding search fail on different queries. Keyword search wins on exact terminology — identifiers, error codes, requirement labels, anything where the precise token matters. Dense retrieval wins on paraphrase and synonymy. Fusing them with reciprocal rank fusion consistently outperforms either, by 15–20% recall on technical corpora.

**Anthropic's contextual retrieval measurements** are the clearest published numbers on what each stage buys:

| Stage | Reduction in top-20 retrieval failures |
|---|---|
| Contextual embeddings alone | 35% |
| Contextual embeddings + contextual BM25 | 49% |
| The above plus reranking | 67% |

The technique behind "contextual" is worth understanding because it's cheap and it addresses the single biggest weakness of chunked retrieval. A chunk reading *"revenue grew 3% over the previous quarter"* is nearly unretrievable — no company, no date, no context. Contextual retrieval prepends a 50–100 token model-generated description situating each chunk in its document before embedding it. The chunk becomes *"This chunk is from an SEC filing on ACME Corp's Q2 2023 performance; the previous quarter's revenue was $314 million. Revenue grew 3%…"*.

**Reranking is the highest-return single addition.** Retrieve broadly — 150 candidates — then use a reranking model to score and keep the top 20. It improves accuracy *and* reduces cost, because fewer irrelevant chunks reach the generating model.

**Chunking follows document structure, not token counts.** The 2026 default is 512–1024 tokens with 50–100 overlap, but for Markdown with real heading structure, heading-based chunking beats fixed-size on every measure. It preserves authorial intent, keeps requirements intact instead of splitting them across boundaries, and produces chunks that align with how people cite documents. Cromwell already chunks this way.

### 2.3 Adaptive routing is the 2026 pattern

The current best practice isn't one pipeline but a classifier that routes each query by complexity. Simple lookups — which are most real queries — take a fast, cheap path. Genuinely multi-hop questions get the full agentic treatment with iterative retrieval and validation. The motivation is cost: running every query through a multi-step agentic pipeline is enormously wasteful when most of them are one hop.

### 2.4 The long-context threshold, and which corpus to measure

Anthropic's guidance is unambiguous and worth quoting as a rule:

> For knowledge bases smaller than 200,000 tokens (roughly 500 pages), include everything in the prompt with prompt caching. It's significantly faster and more cost-effective than retrieval.

The tempting mistake is to measure Cromwell's own documentation — 133,000 words, about 180,000 tokens — conclude it sits just under the line, and decide retrieval isn't worth much. **That measures the wrong corpus.** Cromwell is a tool for building software larger than itself, and the corpus that matters belongs to the project being built.

A representative target project:

| Measure | Cromwell itself | A real target project |
|---|---|---|
| Markdown files | 64 | 619 |
| Words | 133,000 | 992,000 (781,000 of them workflow documents) |
| Approximate tokens | 180,000 | **1,320,000** |
| Against the 200k threshold | Just under | **6.6× over** |
| Against a 1M context window | 18% | **132% — it does not fit** |

At that size the threshold argument doesn't merely fail, it inverts. The corpus exceeds the entire context window of a current frontier model, so there is no "just put it in the prompt" option at any price. Retrieval is how the content is reachable at all.

Two further consequences follow from scale, and they change the engineering.

**The corpus is past the small-corpus regime.** The literature's "keyword search is competitive below roughly a thousand documents" finding is usually stated in retrievable units, not files. At 619 files averaging 1,600 words, and sections averaging somewhere between 250 and 600 words, the index holds roughly **2,500 to 4,000 sections**. That is squarely in the range where hybrid retrieval and reranking start earning their keep, and where lexical-only search begins to show its characteristic failure: vocabulary mismatch. In a 64-file corpus written over a few months, the person querying and the person who wrote the document use the same words. Across 619 files, written over a long project by several agents and a human, the same concept acquires several names — and that is precisely the gap dense retrieval closes and keyword search cannot.

**Precision at the top of the ranking becomes load-bearing.** Returning twenty hits from sixty files is fine; a reader scans them. Returning twenty hits from four thousand sections, of which three are right, is not — especially when the consumer is a dispatched agent that will read the top three and nothing else. This is the argument that moves reranking from a nice-to-have to a requirement, and it is a scale effect rather than a quality preference.

### 2.4.1 A caveat on Postgres ranking

Worth stating plainly because it affects the staging below: **`ts_rank` is not BM25.** Postgres full-text ranking uses term frequency with document-length normalisation options, but it lacks BM25's inverse-document-frequency weighting and term-frequency saturation. At small scale the difference is invisible. At several thousand sections it shows up as common terms dominating the ranking and rare, discriminating terms being under-weighted — exactly the wrong behaviour when searching a technical corpus where the distinguishing term is usually the rare one.

Three ways out, in ascending order of cost: `ts_rank_cd` (cover density, which at least rewards proximity), a real BM25 implementation via a Postgres extension such as `pg_search`, or hybrid retrieval with embeddings. The point for now is that "keep it lexical" is a defensible position and "keep it exactly as ranked today" is a weaker one.

### 2.5 Hierarchical retrieval earns its place at scale

One structural technique does become worth having as the corpus grows, and it's cheap.

The problem it solves is that a retrieved section can be meaningless on its own. In a large specification, a section reading *"the same constraint applies to the bulk path"* is a perfect keyword match for "constraint" and useless without the section above it. Retrieval systems address this by decoupling **search** from **reading**: match at fine granularity, then expand the match to a coherent unit before handing it over — the neighbouring sections, or the parent heading and its children.

Cromwell can do this with a `WHERE` clause. `document_sections` already carries `position` and `level`, so expanding a hit to its siblings or its parent's span is an ordinary indexed query against a table that's already there. No new storage, no new dependency.

At sixty files this would be a refinement. At four thousand sections, where the matched fragment is more often a fragment, it is most of the difference between retrieval that reads well and retrieval that reads like grep output.

### 2.6 GraphRAG: still not worth it, and the reason doesn't change with size

Entity-relation graphs over documents discover non-adjacent connections that chunk retrieval misses. The reported practical problems are consistent: massive token consumption during ingestion, a persistent gap between expected and actual entity-extraction quality, and knowledge fragmentation.

For Cromwell this is unnecessary, and — unlike most of the conclusions in this section — **the reason is scale-independent.** The relationships a document graph would infer are already **first-class foreign keys in Postgres**: this spec belongs to this feature, this feature sits under this initiative, this design was approved before that spec, this document supersedes that one. A larger project has more of those relationships, not vaguer ones. Cromwell never needs to infer its document graph, at any size, because it has one and it's correct by construction.

This is the one place where scale strengthens the existing recommendation rather than weakening it.

---

## 3. The agentic-search counter-argument

The most important development for a coding-agent system is that Anthropic removed retrieval from Claude Code.

Early versions used RAG with a local vector database. In May 2025 the embedding pipeline, the vector store, and the chunking heuristics were replaced with `grep`, `glob`, and `find`. The stated reasoning: agentic search proved more accurate, simpler to operate, and free of the security, privacy, staleness, and reliability problems of maintaining an index. Cursor, Windsurf, Cline, Devin, and Sourcegraph Amp followed.

Anthropic's context engineering guidance generalises this into **just-in-time retrieval**: agents hold lightweight identifiers — file paths, queries, links — and load data at runtime through tools, rather than pre-loading everything. The analogy offered is human cognition: we don't memorise a corpus, we know where to look.

The same document recommends a **hybrid**, and describes Claude Code itself as one: it loads instruction files up front and uses `grep` and `glob` at runtime.

### What this does and doesn't establish

It establishes that **for code, in an interactive agent with a filesystem, search tools beat a vector index.** That's a strong result and Cromwell should honour it.

It does not establish that retrieval is dead generally. The counter-arguments are substantive:

- Index-free search works well for well-organised, consistently-named codebases and degrades on unstructured or semi-structured prose, where the query terms and the document's terms diverge.
- Grep returns line matches without section context or relevance ranking. A search for "error handling" across a design corpus returns two hundred hits across fifty files with no way to prioritise. Section-level ranked retrieval returns the six sections that are actually about it.
- Agentic search requires an agent that can iterate — search, read, refine, search again. That's exactly what a chat agent can do and exactly what a single-shot dispatched agent with a fixed turn cap cannot.

That last point is the one that matters most here, and it's the hinge of §7.

---

## 4. What Claude ships built-in

A survey, because several of these are newer than the last time this was looked at, and one of them is directly relevant.

### Search result content blocks — the RAG-native primitive

This is the piece most worth knowing about. A `search_result` content block lets Claude cite your own content the way it cites web search results, with the source and title you supplied attached to each citation.

```json
{
  "type": "search_result",
  "source": "docs/design/DESIGN-003-document-lifecycle-and-gates.md",
  "title": "Document lifecycle and gates",
  "content": [{ "type": "text", "text": "…the section body…" }],
  "citations": { "enabled": true }
}
```

Two delivery routes: returned from your own tool call, for dynamic retrieval, or placed directly in a user message, for content you retrieved ahead of time. **The second route is the one that fits Cromwell** — the orchestrator retrieves, then pushes the results in as blocks.

It's generally available on all current Claude models with no beta header, and citations appear automatically without special prompting.

The value is specific: an agent that cites which document section justified a decision produces exactly the evidence trail the review and verification paths already demand, as structured data rather than a prose promise.

**But it is an Anthropic-specific content block, and Cromwell must not depend on it.** DeepSeek's Anthropic-compatible endpoint documents a list of supported fields, and this is not among them; an unrecognised content block is at best ignored and at worst a 400. Building the evidence trail on a provider-specific primitive would make Cromwell's most important quality mechanism break silently the moment a project switches models.

The portable equivalent is already in Cromwell and is arguably better: **carry citations in the outcome tool schema.** `submit_review` and `submit_verification` already take structured evidence, and the evidence contract is already enforced at parse time. A `source` field on each finding — document path plus section heading — gives the same audit trail, works on any provider that supports tool use at all, and is checkable by code rather than by trusting the model to emit the right block type.

Use search result blocks if they're available and free. Don't design around them.

### Memory: two different things with the same name

**The memory tool** (`memory_20250818`) is client-side. Claude reads and writes files in a `/memories` directory; you implement the storage. Commands are `view`, `create`, `str_replace`, `insert`, `delete`, `rename`. It persists across sessions, and you own everything about how.

**Memory stores** are a Managed Agents feature and a much larger thing: workspace-scoped collections of small text documents (≤100KB each), mounted into the agent's container as a filesystem, with host-side CRUD, optimistic concurrency via content hashes, immutable per-mutation versions, and redaction for leaked content. Access can be read-only or read-write per session.

Neither is a retrieval system. Both are *storage* the model reads with ordinary file tools. That's a meaningful design signal in itself: Anthropic's answer to persistent agent knowledge is a mounted filesystem the model greps, not a queryable store.

### Context management

**Context editing** (`clear_tool_uses_20250919`, `clear_thinking_20251015`) removes stale tool results and thinking blocks from a transcript. It prunes; it doesn't summarise.

**Compaction** (`compact_20260112`) summarises earlier history server-side when approaching the window limit.

Neither applies to Cromwell today — dispatches are short and stateless between runs — but both matter if long-running or resumable sessions ever arrive.

### Files, documents, and citations

The Files API uploads content once and references it by `file_id` across requests. Document content blocks accept PDFs and text, and setting `citations: {enabled: true}` splits the response into cited and uncited text blocks, each citation carrying `cited_text` and a character or page location.

### Progressive disclosure by other means

**Agent Skills** load a description always and the full body only when relevant. **Tool search** (regex and BM25 variants) lets Claude discover tool schemas from a large library instead of carrying all of them — and notably, discovered schemas are *appended* rather than swapped, preserving the prompt cache.

That BM25 tool search is a quiet endorsement worth noting: when Anthropic needed retrieval over their own tool catalogue, they shipped lexical search, not embeddings.

---

## 5. What Cromwell already has

This is the section that changes the shape of the decision. I audited the schema and the store layer.

### The index exists

Migration `0001_init.sql` creates:

```sql
CREATE TABLE document_sections (
  id          uuid PRIMARY KEY,
  document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  position    integer NOT NULL,
  heading     text NOT NULL,
  level       integer NOT NULL,
  content     text NOT NULL,
  role_class  text,
  fts         tsvector GENERATED ALWAYS AS (to_tsvector('english', heading || ' ' || content)) STORED
  -- embedding vector(...) arrives with its phase-2 migration (DESIGN-001 §12)
);
CREATE INDEX document_sections_fts ON document_sections USING gin (fts);
```

Every element of a competent lexical retrieval layer is there. Heading-based chunking — the structure the literature prefers. A generated full-text vector that can never go stale, because Postgres maintains it. A GIN index. A `role_class` column for section classification. And an explicit comment reserving the place where embeddings will go.

`DESIGN-001` goes further and already specifies the vector layer:

```sql
embedding    vector(1536)   -- pgvector; NULL until embedded
CREATE INDEX ON document_sections USING hnsw (embedding vector_cosine_ops);
```

with dimensionality flagged as provisional pending a model choice. This was designed, deliberately deferred, and documented as deferred.

### The population path exists

`internal/store/documents.go` deletes and re-inserts a document's sections on every version, so the index tracks the live document by construction. There's no staleness problem to solve because there's no separate index to drift.

### The query path exists

`SearchSections` is complete:

```go
SELECT d.id, d.path, d.title, sec.heading,
       ts_headline('english', sec.content, websearch_to_tsquery('english', $1),
                   'MaxWords=20, MinWords=5') AS snippet
FROM document_sections sec
JOIN documents d ON d.id = sec.document_id
WHERE sec.fts @@ websearch_to_tsquery('english', $1)
  AND d.state <> 'superseded'
ORDER BY ts_rank(sec.fts, websearch_to_tsquery('english', $1)) DESC
```

Section-level ranked retrieval, with generated snippets, filtering superseded documents, using `websearch_to_tsquery` so the query syntax is the one people already know. This is a good implementation.

### And a second retrieval path exists, in production

`RetrieveCorpus` does nearest-neighbour retrieval over completed work for the estimator, rewriting `plainto_tsquery`'s AND into an OR so neighbours match on term overlap rather than containment, then ranking by `ts_rank`. It has been running and tested since phase 3.

So Cromwell doesn't merely have retrieval machinery. **It has a shipped, working, tested retrieval feature that dispatched agents already benefit from** — the estimator receives real prior estimate-versus-actual pairs, retrieved lexically, pushed into its prompt by the orchestrator without the agent asking for anything.

That is a working retrieval-augmented dispatch. It just isn't called one.

### What's missing

Exactly one thing, and it's small:

| Piece | Status |
|---|---|
| Section chunking | Built |
| Full-text index | Built, GIN-indexed, self-maintaining |
| Ranked section search with snippets | Built (`SearchSections`) |
| Retrieval pushed into a dispatch prompt | **Built, but only for the estimation corpus** |
| Retrieval exposed to the chat agent | **Missing** |
| Retrieved sections pushed into execution-path prompts | **Missing** |
| Embeddings / hybrid / reranking | Designed, deliberately deferred |

`SearchSections` is called from exactly one place: `internal/server/http.go:464`, the command centre's search endpoint. **A human can search the document corpus. No agent can.**

### How well it holds up at target scale

The parts that are built are the parts that scale best. Postgres full-text search over a GIN index handles millions of rows without complaint; four thousand sections is not a performance question. Section population is incremental — a document's sections are replaced when that document is written — so a large corpus costs no more to maintain than a small one. And the index cannot go stale, at any size, because the tsvector is a generated column.

What degrades with scale is **ranking quality, not throughput**: `ts_rank`'s weak term weighting (§2.4.1), vocabulary mismatch across a corpus written over a long project, and matched fragments that don't stand alone (§2.5). All three are addressed by additions on top of what exists, none of which require changing the schema that's already there.

That's a good position to be in. The foundation was designed well enough that scaling it is additive.

---

## 6. Was the predecessor's failure the orchestrator's fault?

This is the sharpest question in the brief, and the answer is a qualified yes — with an important part that isn't.

### What actually failed

Two distinct failures get conflated:

**Failure one: the knowledge subsystem went unused.** Eighty-six entries contributed, twelve retrieved. Agents preferred to search documents directly.

**Failure two: the document intelligence system was never activated.** Layer 3 classification never ran across the 280 indexed documents. The concept registry was empty. Concept search had never returned a result. There was no full-text search at all, so agents fell back to grep for anything not addressable by entity ID.

These have different causes, and only the first is about agent behaviour.

### The part that *was* the orchestrator's fault

A non-deterministic orchestrator makes retrieval a **choice**, and every choice is a chance to not choose it.

The parallel evidence is the sub-agent context propagation failure. Several dispatched agents had a codebase knowledge graph available — indexed, ready, faster than the alternative. None used it, because the dispatching agent never told them it existed. The prose fix worked and was fragile for exactly the reason all prose fixes are: it depended on an agent remembering, in a context window where the instruction sat in the low-attention middle.

**With a code orchestrator, that entire class of failure disappears.** Code cannot forget to run a query, cannot fail to propagate a tool's existence, and cannot decide it would rather grep. So yes — a meaningful part of why document intelligence never delivered was that nothing deterministic was ever going to make it deliver.

Cromwell's estimation corpus proves the point empirically. It's the same retrieval technique that went unused before, and it works here, for one reason: **the orchestrator runs the query and puts the results in the prompt.** The agent was never asked.

### The part that wasn't

Two of the failures were the system's own.

**Layer 3 classification never ran** because it required ongoing agent effort to maintain — every document needed classifying, and re-classifying as it changed. That's a maintenance burden no orchestrator fixes; a deterministic orchestrator would simply have failed to run it reliably too, or run it expensively forever.

**There was no full-text search.** Concept search and entity search were built; the thing agents actually needed — find sections mentioning this phrase — didn't exist. Agents grepped because grep was the only tool that answered their question. That's not a discipline failure. It's the tool interface finding, arriving from the direction that hurts: **agents fall back to what they know when the purpose-built path doesn't do the job**, and here it genuinely didn't.

### The synthesis

> The predecessor built the sophisticated half of retrieval and never built the simple half; then it left the whole thing behind an opt-in tool and hoped an agent would choose it.

Cromwell has inverted both. It has the simple half (full-text over sections) and not the sophisticated half (concepts, classification, graph). And it has a code orchestrator that can push rather than offer.

That inversion is why the answer this time is different. **The failure is not repeatable in Cromwell's architecture** — not because the lesson was wrong, but because both of its causes have been removed.

One caveat worth keeping. The lesson still binds anywhere Cromwell *does* offer retrieval as a choice, which means the chat agent. There, the mitigation isn't discipline, it's making the tool good enough to beat grep and letting the agent search files when it prefers.

---

## 7. Two phases, two architectures

Cromwell's two-phase shape — chat-based design and planning, then deterministic orchestration of dispatched agents — is unusual, and it's the reason a single retrieval answer won't fit. The phases want opposite architectures, and Cromwell can give them both.

### Phase one: chat-based design and planning

**Characteristics.** A human and a strong model in a long conversation. Broad, exploratory, unpredictable queries. The model can iterate — search, read, refine, search again. The context window is large and the conversation is long-lived.

**What the field says.** This is the case where agentic search wins. It's Claude Code's own use case, and the reason Anthropic deleted their vector index. An agent that can iterate outperforms pre-computed retrieval, because it refines its query based on what the first search returned.

**The caveat that matters if the model isn't a frontier one.** That result was measured on a strong model, and iterative search is exactly the capability that degrades first as models get cheaper. The orchestration research documents the shape of it: weaker models fall into a reactive pattern — more status-checking, more messaging, far fewer structural actions — while stronger models decompose and refine. Query reformulation is a structural action. A model that isn't good at it will run one mediocre search, accept whatever comes back, and proceed.

So the phase-one recommendation has a slope to it:

| Chat model | Retrieval posture |
|---|---|
| Frontier | Give it search tools and let it drive. Pre-retrieval adds little. |
| Mid-tier | Give it search tools, but pre-load the high-signal set — decisions, current designs, the feature's ancestors. |
| Cheap | Pre-retrieve and push. Treat it more like the dispatch phase than like Claude Code. |

**Pre-computed retrieval degrades far less than agentic search does**, because the quality comes from the index and the ranker rather than from the model's search strategy. That asymmetry is the single most important consequence of not standing on Claude.

**What Cromwell should do.** Expose `SearchSections` through the MCP facet as a ranked section search, alongside the file access the chat agent already has. The two are complementary: ranked section search for "what has been decided about X", grep for "where does this exact string appear".

Keep the tool schema **narrow** — a query string and a limit, not a rich object with filters, scopes, and modes. Beyond the general principle that one tool should do one thing, there's a concrete provider hazard: DeepSeek has been observed to silently omit arguments from schemas deeper than about two levels or wider than about ten leaves. A retrieval tool that silently loses its scope filter returns plausible, wrong results with no error.

And note what scale has done to the fallback: at 1.3 million tokens, "push a curated subset of everything important" is no longer available. Even the decisions and current designs of a project that size may not fit comfortably inside the useful attention band. Selection has to be query-driven, which means it has to be retrieval.

### Phase two: deterministic dispatch

**Characteristics.** A single-shot agent with a fixed turn cap and an enforced tool profile. It cannot iterate its way to the right context — it has a budget and a job. Its prompt is assembled entirely by code.

**What the field says.** This is *not* the agentic search case. The whole premise of just-in-time retrieval is an agent with time and tools to explore. A dispatched agent has a hard budget instead — the starter pack gives the implementer 40 turns and the code reviewer 15 — and turns spent rediscovering conventions are turns not spent on the work. The code reviewer's budget is the tighter constraint, and it's the role most likely to need a design decision it wasn't given.

**What Cromwell should do.** The orchestrator retrieves and pushes. This is the estimation corpus pattern, generalised.

Concretely, for an implementer or code-reviewer dispatch, the orchestrator would query section search using the task description and the feature name, take the top few sections from approved documents, and include them in the assembled prompt with their provenance. Deterministic, auditable, identical for identical inputs, and cacheable.

Two things make this materially better than the same idea in the predecessor:

- **It closes a gap that's already documented as open.** Implementer and code-reviewer dispatches currently receive no ancestor documents at all — only the spec and dev-plan. Every implementer rediscovers the project's conventions by reading code, paying tokens each time and landing somewhere slightly different each time.
- **It has a natural output format.** Each pushed section carries its document path and heading, and the outcome tool's evidence field carries them back — so "which section justified this" is structured data rather than a prose promise. On Anthropic, `search_result` blocks do this more elegantly; the outcome-tool version is the one that works everywhere (§4).

### The rules that fall out

> Retrieval that an agent must choose to invoke belongs in the chat phase, where the agent can iterate and the human is watching. Retrieval in the dispatch phase is the orchestrator's job, and its results are pushed, not offered.

> The weaker the model, the further the boundary moves toward pushing. Agentic search is a capability, and capabilities are what you lose first when you trade down.

The second rule has a pleasant property: the work it implies is the same work either way. An orchestrator-side retrieval path built for the dispatch phase is exactly what a weaker chat model needs pre-loaded. Building for the cheap-model case costs nothing extra and buys provider independence.

---

## 8. Recommendation

Build in stages, and stop as soon as it's good enough *for the project in front of you*. Each stage is independently useful and each one's evidence informs the next.

### The principle that governs the staging

**Retrieval quality has to be a per-project configuration, because Cromwell's projects span three orders of magnitude of corpus size.** A sixty-file project is well served by ranked lexical search over sections and nothing else. A 619-file, 1.3-million-token project needs reranking and probably embeddings. Cromwell cannot know which it's pointed at until it's pointed there.

This is not a new mechanism — the config compartment already carries roles, skills, templates, and assignments per project. Retrieval settings belong beside them: which ranker, whether to rerank, whether to embed, how many sections to push. The default should be the cheap end, and scaling up should be a config change rather than a migration.

The same principle answers the provider question. Every stage below is provider-neutral except where noted, and where a stage needs a model — contextual chunking, reranking, embeddings — that model is a per-project configuration too, and needn't be the one running the workflow.

### Stage one: connect what exists

**Push retrieved sections into execution-path dispatch prompts.** The orchestrator queries `SearchSections` scoped to the feature's approved ancestor documents, takes the top three to five sections, and includes them with provenance. This closes the documented conventions gap using machinery that already works.

**Expose section search to the chat agent** through the MCP facet, as a read-only tool returning ranked sections with snippets and paths.

**Keep the pushed block small.** Three to five sections, not thirty. The n=19 cliff and the attention budget both bite, and the whole point is to spend fewer tokens than rediscovery costs, not more.

**Measure it.** Cromwell has the token ledger to answer whether pushing conventions costs less than agents rediscovering them. That measurement should decide whether stage two happens.

Cost: small. Both halves are wiring, not construction. This is enough on its own for a small project.

### Stage one and a half: expand hits before pushing them

**Return a matched section together with its neighbours or its parent's span**, rather than the fragment alone. `position` and `level` are already on the table; this is a `WHERE` clause and a few lines of assembly.

It's listed separately because it's disproportionately cheap for what it fixes, and because the fix only becomes visible at scale — the larger the corpus, the more often the best keyword match is a fragment that doesn't stand alone.

### Stage two: expected at target scale, not conditional

The original framing of this stage was "if stage one shows it needs to be". At sixty files that's right. At 2,500–4,000 sections it isn't — the published numbers and the vocabulary-mismatch problem both say these are needed, and waiting for a project to visibly suffer wastes the project it suffers on.

In order of return per unit of effort:

1. **Reranking.** The single highest-return addition in the published numbers (49% → 67% failure reduction), and it does double duty here by compensating for `ts_rank`'s weak ranking without replacing it. Retrieve broadly, rerank, keep the top few. Provider-neutral: any small model can rerank, including a cheap one, and it needn't be the workflow model.
2. **Better lexical ranking.** Cheaper than embeddings and addresses a real defect (§2.4.1). `ts_rank_cd` is a one-line change; a proper BM25 via a Postgres extension is a larger but still contained one. Worth trying before reaching for vectors.
3. **Contextual chunking.** Prepend a short generated description situating each section in its document before indexing. Addresses the orphaned-chunk problem at its root. One caveat that scale introduces: this is cheap **incrementally** — one small model call per section at document submission — and expensive as a **backfill**, which is the mode Cromwell will be in when pointed at an existing 619-file project. Budget the backfill deliberately, or run it lazily against the documents that actually get retrieved.
4. **Embeddings and hybrid fusion.** The pgvector column and its HNSW index are already designed in DESIGN-001. At target scale the vocabulary-mismatch argument now favours adding them, where at Cromwell's own scale it didn't.

   Two notes on the provider question, since embeddings are the one stage that adds a genuinely new external dependency. First, this was never a Claude decision: **Anthropic doesn't sell an embedding model** — their own contextual retrieval guidance points at third parties — so adding embeddings introduces the same dependency regardless of which model runs the workflow. Second, that makes a **local embedding model a serious option**, which would keep the whole retrieval path in-house and provider-independent. Whatever is chosen, treat DESIGN-001's provisional `vector(1536)` as genuinely provisional; the dimensionality locks you to a model family and is awkward to change after indexing.

### Stage three: probably never

Concept registries, entity-relation graphs, semantic role classification, community detection. Every one of these was built or designed in the predecessor, and none of them paid for itself. Cromwell's document relationships are foreign keys, not inferences. Leave this alone.

### What to avoid

**Don't build a knowledge subsystem.** No contribution lifecycle, no confidence scoring, no staleness detection over agent-authored entries. That experiment has run twice.

**Don't add a separate retrieval service.** The index is in the same Postgres as the documents, maintained by a generated column. That's the property that makes it impossible to go stale. A standalone server reintroduces exactly the sync problem this design avoids.

**Don't make classification a prerequisite.** The `role_class` column can stay NULL forever. The predecessor's Layer 3 never ran; a design that depends on it will fail the same way. This warning gets sharper with scale, not softer — whatever the per-document cost of classification, a 619-file corpus pays it 619 times and then again on every revision.

**Don't design around provider-specific primitives.** Search result content blocks, the memory tool, and prompt-cache mechanics all differ by provider or don't exist outside Anthropic. Use them where available as an optimisation; keep the load-bearing path — retrieved text in the prompt, citations in the outcome tool schema — on primitives every provider supports.

**Don't assume the workflow model is the retrieval model.** Reranking, contextual chunking, and embedding are separable jobs that can run on cheap or local models regardless of what's running the chat and the dispatches. Coupling them would mean a project's retrieval quality silently changes when someone switches the workflow model for cost reasons.

---

## 9. What this does not replace

The brief asks directly whether a retrieval system would replace Claude's memory and planned decision records. It would not, and it's worth being precise about why.

### Not decision records

A decision record is an **authored artefact with a lifecycle** — proposed, approved, superseded, cited. Retrieval indexes it. The two operate at different layers, and conflating them loses the thing decision records exist for.

The distinction that matters: retrieval answers *"what have we written about X?"* A decision record answers *"what did we decide about X, why, what did it cost, and is that still current?"* No amount of ranked section search produces the second from a corpus that doesn't contain it.

There's also a supersession problem retrieval makes *worse* rather than better. A decision partly overtaken by a later one is more dangerous than one fully replaced, because it still reads as current — and a retrieval system will happily surface it, ranked highly, with no signal that it's stale. `SearchSections` already filters superseded documents, which is the right instinct, and it only works because supersession is modelled explicitly. Retrieval depends on the decision lifecycle; it doesn't substitute for it.

**Decision records become more valuable with retrieval, not less** — they're the highest-signal documents in the corpus, and today nothing surfaces them into any prompt.

### Not Claude's memory

Claude's memory tool and memory stores solve **cross-session persistence for a conversational agent**. Cromwell's dispatches are stateless by design — that's a feature, and the reason the orchestrator can't drift. There's no session to persist.

They're also Anthropic-only, which settles the question from the other direction: a Cromwell that may run on any provider cannot have durable project knowledge living in a Claude memory store, because on half its deployments the store wouldn't exist. Durable knowledge belongs in documents, in Postgres, which every deployment has.

The chat phase is different, and there the two are complementary rather than competing. Chat memory holds *how this human works and what we're currently doing*. The document corpus holds *what the project has decided and specified*. Retrieval over approved documents doesn't tell a chat agent that Sam prefers plain prose in human-facing text; memory does. Memory doesn't tell it what gate G2 checks; the corpus does.

One caution worth carrying, from the memory-store documentation and from the predecessor's experience alike: memories are replayed verbatim into future contexts, so a wrong one is wrong forever until someone notices. That's an argument for keeping durable project knowledge in **approved documents with a review path** rather than in an agent-authored memory store — which is the architecture Cromwell already has.

### Not the codebase graph

`codebase-memory-mcp` answers structural questions about code. This answers content questions about documents. They overlap nowhere. Both should exist.

---

## 10. Sources

### Anthropic — primary

- [Contextual retrieval](https://www.anthropic.com/news/contextual-retrieval) — contextual embeddings and contextual BM25, the 35/49/67% failure-reduction measurements, reranking, and the 200,000-token "don't bother" threshold.
- [Effective context engineering for AI agents](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents) — the attention budget, just-in-time retrieval, the hybrid recommendation, compaction and structured note-taking.
- [Search result content blocks](https://platform.claude.com/docs/en/build-with-claude/search-results.md) — the RAG-native primitive: schema, citation behaviour, tool-call versus top-level delivery.
- Memory tool, memory stores, context editing, compaction, Files API, citations, Agent Skills, and tool search — surveyed via the bundled Claude API reference.

### The agentic-search turn

- [Why Claude Code abandoned RAG for agentic search](https://zenn.dev/karamage/articles/2514cf04e0d1ac?locale=en) — the decision and its stated reasoning.
- [Anthropic replaced their RAG pipeline with agentic search](https://robertheubanks.substack.com/p/anthropic-replaced-their-rag-pipeline) — independent testing of both approaches.
- [Agentic search over vector embeddings](https://github.com/nibzard/awesome-agentic-patterns/blob/main/patterns/agentic-search-over-vector-embeddings.md) — the pattern, written up.

### The counter-position and current practice

- [From RAG to context — a 2025 year-end review](https://ragflow.io/blog/rag-review-2025-from-rag-to-context) — the context-engine reframing, the three retrieval needs, TreeRAG and GraphRAG, and the direct rebuttals to both long-context-replaces-RAG and grep-replaces-RAG.
- [Hybrid search: BM25, vector, and reranking reference 2026](https://www.digitalapplied.com/blog/hybrid-search-bm25-vector-reranking-reference-2026) — the current consensus stack and fusion numbers.
- [RAG chunking strategies: a 2026 retrieval playbook](https://www.digitalapplied.com/blog/rag-chunking-strategies-2026-retrieval-quality-playbook) — chunk sizing defaults, contextual retrieval, late chunking.
- [From BM25 to Corrective RAG: benchmarking retrieval strategies](https://arxiv.org/html/2604.01733v1) — two-stage hybrid plus neural reranking as the measured best configuration.
- [RAG techniques compared: a practical guide for 2026](https://blog.starmorph.com/blog/rag-techniques-compared-best-practices-guide) — adaptive RAG and query-complexity routing as the emerging default.
- [A-MEM: agentic memory for LLM agents](https://arxiv.org/pdf/2502.12110) and [AgenticRAG: agentic retrieval for enterprise knowledge bases](https://arxiv.org/pdf/2605.05538) — the current academic framing of agent-side retrieval and memory.

### Cromwell code and design referenced

- [internal/store/migrations/0001_init.sql:92](internal/store/migrations/0001_init.sql:92) — the `document_sections` table, generated `fts` column, GIN index, and the reserved embedding comment.
- [internal/store/documents.go:163](internal/store/documents.go:163) — `SearchSections`, the existing ranked section search.
- [internal/store/documents.go:142](internal/store/documents.go:142) — section population on document write.
- [internal/store/corpus.go:78](internal/store/corpus.go:78) — `RetrieveCorpus`, the working retrieval-augmented dispatch already in production.
- [internal/server/http.go:464](internal/server/http.go:464) — the only caller of `SearchSections`.
- `docs/design/DESIGN-001-data-model-and-schema.md` §12 — the deferred pgvector and HNSW design, with dimensionality flagged provisional.

### Related research in this folder

- [agent-research-evidence-base.md](agent-research-evidence-base.md) §4 — the push-not-pull finding and the token-cost measurements behind progressive disclosure.
- [context-for-dispatched-agents.md](context-for-dispatched-agents.md) — the missing project-conventions layer this proposal would fill.
- [prefix-cache-discipline.md](prefix-cache-discipline.md) — why retrieved content's position in the prompt matters for cost.
