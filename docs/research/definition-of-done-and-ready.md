# Definitions of done and ready

A definition of done says what must be true before work closes. A definition of ready says
what must be true before work starts. Most systems write the first one down and never
enforce it, and never write the second one down at all.

This document collects a worked proposal for both, derived from a predecessor project's
hygiene track, and reads it against what Cromwell already has. It is research, not a
design: the check sets, the policy-file question, and the roll-out sequence are all
proposals to accept, reject, or reshape.

The evidence behind the enforcement argument is in
[agent-research-evidence-base.md §2.1](agent-research-evidence-base.md#21-enforceable-constraints-beat-advisory-instructions);
the review-loop machinery around verification is in
[orchestration-patterns.md §3.6](orchestration-patterns.md#36-maker-checker-with-two-termination-controls).

---

## Table of contents

1. [Why this matters](#1-why-this-matters)
2. [What Cromwell already enforces](#2-what-cromwell-already-enforces)
3. [A definition of done](#3-a-definition-of-done)
4. [A definition of ready](#4-a-definition-of-ready)
5. [Declarative policy or Go?](#5-declarative-policy-or-go)
6. [Two layers: deterministic and judged](#6-two-layers-deterministic-and-judged)
7. [Making the rules inspectable](#7-making-the-rules-inspectable)
8. [Surfacing the health of the work](#8-surfacing-the-health-of-the-work)
9. [Rolling it out safely](#9-rolling-it-out-safely)
10. [Risks](#10-risks)

---

## 1. Why this matters

The failures this addresses are all the same shape: work that *looks* finished but isn't.

- A feature closes with its branch never merged.
- A worktree is left behind, so the next agent picks up stale state.
- Tests are failing and nobody notices, because nothing checks.
- A bug closes with no review record, because the bug path runs different code from the
  feature path.
- An override closes something without a recorded reason.

Each of these is cheap to detect mechanically and expensive to discover later. And each of
them recurs, because the definition of done lives in a table inside a design document,
where nothing can enforce it.

The research position is unambiguous. A rule enforced by instruction is skipped when the
task seems simple or the context grows long. A rule enforced by a tool that refuses to
operate is not skipped. That is what makes the difference between a definition of done and
a wish.

The symmetric case is less obvious but just as real. Nothing normally stops an agent
claiming work and starting to edit while its worktree is dirty, while its branch has
diverged, while a dependency is unfinished, or while the contract it is working from has
gone stale. A definition of ready is the gate that catches those at claim time, when
they're cheap, rather than at merge time, when they aren't.

---

## 2. What Cromwell already enforces

Cromwell is unusual in having got the enforcement mechanism right before the check
inventory. The gates in `internal/lifecycle/gates.go` are pure functions over current
state, evaluated by a code orchestrator with no model attached, and every evaluation is
audited with its reason.

| Gate | Guards | Expression |
|---|---|---|
| **G0** | A feature may be specified | Its own design is approved, or its immediate parent initiative's is |
| **G1** | Feature `idea → ready` | Current spec approved, and dev-plan approved where required |
| **G2** | Feature `active → review` | Every task terminal, at least one done, contract not stale |
| **G3** | Feature `review → done` | Verification passed |
| **G4** | Milestone `open → locked` | — |
| **G5** | Initiative archivable | — |

Read against the proposal below, several of the classic definition-of-done checks are
already covered:

| Proposed check | Cromwell status |
|---|---|
| All tasks terminal | **Enforced** by G2 |
| Contract documents present and approved | **Enforced** by G1 (spec, and dev-plan where required) |
| Design approved before specification | **Enforced** by G0 |
| Review record exists | **Enforced** — the review loop runs per task diff, and its verdict is derived from finding severity |
| Verification performed | **Enforced** by G3, and verification is always dispatched and cannot be skipped |
| Contract not stale mid-flight | **Enforced** by G2's `spec_stale` brake |
| No silent overrides | **Enforced** — no force flags; an override happens only through an answered checkpoint |

So the question isn't whether to build enforcement. It's which checks are missing, and
whether the ruleset should be a governable artefact rather than Go.

What's genuinely absent:

- **The worktree and repository checks.** Nothing asserts that the working tree is clean,
  that the branch merged, that the worktree was removed, or that the tests pass, as a
  condition of closing.
- **A definition of ready.** G0 and G1 are readiness gates for a *feature*, and they're
  good ones. There is no equivalent at task claim time — no check that the worktree exists
  and is clean, that dependencies are done, or that the branch isn't hopelessly behind.
- **A dry run.** There's no way to ask "would this pass?" without attempting the transition.
- **A health surface** over the lagging indicators: stale worktrees, failing tests,
  unmerged completed work.

---

## 3. A definition of done

The proposal expresses the definition of done as a set of named checks per entity type,
each declaring whether it's required or advisory, with parameters where a check needs them.

### The feature check set

| Check | Required | What it means |
|---|---|---|
| `git_status_clean` | Yes | `git status --porcelain` is empty in the feature's worktree |
| `branch_merged` | Yes | The feature branch is an ancestor of the target branch |
| `tests_pass` | Yes | The project's test command exits zero |
| `contract_documents_approved` | Yes | Spec and dev-plan exist and are approved — not merely registered |
| `worktree_removed` | Yes | The worktree record is merged or removed, *and* the directory is gone from disk |
| `all_tasks_terminal` | Yes | Every child task is done or abandoned |
| `reviews_present` | Yes | At least one review record exists for the work |
| `verification_passed` | Yes | The dispatched verification returned an approve verdict with per-criterion evidence |

Two details in that list carry more weight than they look:

**Approved, not merely present.** A specification sitting in draft does not satisfy
`contract_documents_approved`. The whole point of the check is that a human or a reviewer
made a decision.

**Both halves of `worktree_removed`.** A record saying the worktree was removed while the
directory still sits on disk is exactly the state that poisons the next agent. Check the
record *and* the filesystem.

### Smaller entity types

The reason to write the check sets down as data rather than as code is what it prevents.
A recurring failure in the predecessor system was that bugs closed without following the
definition of done, because the bug pipeline had its own code path. Expressing every entity
type's checks as entries in one place, evaluated by one function, makes a separate path
impossible rather than merely discouraged.

A smaller entity — a bug, or a follow-up fix — would carry a subset:

| Entity | Checks |
|---|---|
| Bug | `git_status_clean`, `branch_merged`, `tests_pass`, `worktree_removed`, `reviews_present` |
| Retro-fix | `git_status_clean`, `branch_merged`, `tests_pass`, `worktree_removed` |

Cromwell doesn't have bug or fix entities yet — they're planned. Whenever they arrive, the
lesson to carry forward is that they must run the **same** check function against entries in
the **same** ruleset. Not a parallel implementation that drifts.

### Advisory checks

Not every check should block. An advisory check runs, reports, and doesn't fail the gate.
It's the right level for something that's usually right but sometimes legitimately not:
linked incidents still open, an index gone stale, a running binary out of date.

Advisory status should be a deliberate decision recorded with the check, not a default that
accumulates. A ruleset where most checks are advisory has stopped being a definition of
done.

### What each check must return

Every check returns three things: pass or fail, a short human-readable **evidence** string
explaining what it saw, and any error.

The evidence string is the part that's easy to skip and expensive to omit. "Failed" tells
whoever reads the audit log nothing. "3 uncommitted files: internal/rules/rules.go,
internal/store/doc.go, docs/notes/scratch.md" tells them what to do next.

---

## 4. A definition of ready

The symmetric gate, evaluated before work starts rather than after it finishes. Two natural
places for it: when a task is claimed, and when an editing tool is invoked.

### At claim time

| Check | Required | What it catches |
|---|---|---|
| `worktree_exists` | Yes | Work starting outside an isolated worktree |
| `worktree_clean` | Yes | Uncommitted state from an earlier run, hidden from parallel agents |
| `branch_current` | Advisory | A branch far enough behind the target that the merge will be a fight |
| `dependencies_done` | Yes | A task starting before what it depends on has finished |
| `role_resolvable` | Yes | A dispatch whose role or skill doesn't exist |
| `contract_approved` | Yes | Work starting against an unapproved or stale spec or dev-plan |

Each of these closes a specific failure that has actually happened somewhere:

- Agents that assume they're the only one working, and so don't use a worktree →
  `worktree_exists`.
- Stashing that hides state from parallel agents → `worktree_clean` forces commit or
  discard before the claim.
- Forced merges that overwrite good work → `branch_current` makes the divergence visible at
  claim time rather than at merge time.

`branch_current` is advisory deliberately. A long-running feature will legitimately drift,
and a hard gate here blocks honest work. Make the threshold configurable and let it warn.

### At edit time

A lighter, faster check on every editing tool call:

| Check | Required | What it catches |
|---|---|---|
| `work_context_present` | Yes | An edit with no task or feature attached — untracked work |
| `not_on_default_branch` | Yes | A commit landing directly on the main branch during feature work |

Both need an exemption path for genuine infrastructure work, or they'll block the very
person trying to fix them. An explicit exempt list is better than a force flag, because it's
declared in advance rather than reached for under pressure.

The edit-time gate is also where Cromwell's newer flexibility needs care. If work can be
executed by a dispatched agent, by a chat agent, or by a human, then a chat agent's edits
can't be constrained by the same tool profile a dispatch enforces. The practical net is a
watcher rather than a gate: flag commits touching an active feature's paths that have no
claim behind them.

---

## 5. Declarative policy or Go?

The proposal makes the ruleset a version-controlled YAML file, with Go functions registered
against the names declared in it. Its argument:

- The ruleset becomes a reviewable artefact. Changing it is a pull request like any other
  change, with a diff and a discussion, rather than an edit buried in a handler.
- Adding a check needs one entry and one registered function — no changes to the code that
  evaluates them.
- One file means one code path, which is the structural fix for the divergent bug pipeline.
- A human can read the current rules without reading Go.

The counter-argument, specific to Cromwell:

- Cromwell's gates are already small pure functions with human-readable reasons attached,
  which is most of the readability benefit without a second format.
- The config compartment already carries roles, skills, and templates. Another file type
  there is more surface to validate, version, and keep in step.
- A YAML ruleset with Go functions behind it can drift: a name declared with no function
  registered, or a function nobody calls. That needs its own check — a startup assertion
  that names and registrations correspond one to one.

**A middle position is available and probably right.** Keep the gate expressions in Go,
where they are. Add a declarative layer only for the checks that are genuinely
project-specific and that a project would reasonably want to vary — the test command,
whether a dirty worktree blocks, how far behind is too far behind. That's a small
configuration surface rather than a policy engine, and it keeps the load-bearing logic where
it can be tested without a database.

Whatever is chosen, one rule holds: **no evaluation logic outside the registered checks.**
The moment a merge handler grows its own idea of what "done" means, there are two
definitions of done and one of them is invisible.

---

## 6. Two layers: deterministic and judged

Some checks are mechanical — a command exits zero, a directory is absent, a status field is
terminal. Others need judgement — does this implementation actually satisfy the acceptance
criteria?

The proposal runs the deterministic layer first and lets it block, then runs the judgement
layer. And it makes a strong recommendation: **let the judging layer observe before it
blocks.** Ship it recording verdicts to the audit log with no power to fail the gate,
gather evidence that its judgement is calibrated, and only then promote it to blocking.

The reasoning: a system's track record on enforcement by model judgement is usually poor,
and the failure mode is silent. "The reviewer was supposed to catch this and passed it" is
much worse than "the check wasn't there", because it looks like coverage.

**Cromwell has already made the opposite call, and deliberately.** The dispatched
`verify-feature` verdict blocks G3 today. Two defences make that defensible where it
otherwise wouldn't be:

- The **evidence contract is enforced at parse time**, not merely requested in the prompt. An
  approve with an empty criteria array does not merge the feature. This is the structural
  answer to rubber-stamp approval, which is the most frequently observed quality failure in
  multi-agent systems.
- The **verdict is derived from finding severity** rather than chosen, so a reviewer holding
  a major finding cannot approve.

The order still matters, though. **Run the mechanical checks before spending tokens on
judgement.** There's no value in paying a verification agent to notice that the build is
broken, and a failing test discovered by a model is a failing test discovered expensively.

---

## 7. Making the rules inspectable

A ruleset nobody can read is a ruleset nobody trusts. The proposal adds a read-only surface
with three operations:

- **Get** — return the full parsed ruleset as structured data, not raw file text.
- **List** — return the checks for one entity type, or all of them, each with its name,
  required flag, and parameters.
- **Evaluate** — run every applicable check against a specific entity right now and return
  per-check pass, fail, and evidence, plus an overall result.

**Evaluate is the valuable one.** It's a dry run: no state change, no transition, no audit
row, no effect on any later attempt. It answers "would this pass, and if not, what
exactly?" — which is the question anyone asks before trying to close something, and which
today can only be answered by attempting the transition and reading the refusal.

Three constraints keep it honest:

- **No side effects at all.** A passing dry run is advisory. It is not a gate pass, and it
  must not be cached as one.
- **No audit row.** Dry runs would pollute the trail that a drift detector reads.
- **Unknown entity types return an empty check list and a warning,** not an error. A type
  with no checks registered isn't a failure; it's a gap worth reporting.

For Cromwell there are two natural homes. The command centre is the obvious one — a
feature's page showing each check with its evidence turns "why won't this close?" from a log
dig into a glance. The MCP facet is the other, with one important boundary: the facet
exposes *inspection*, never *verdict*. Reading the rules is safe. Deciding they're satisfied
is not, and verdict tools stay out of the facet.

A read-only surface also needs a real error path. If the ruleset is missing or malformed,
every operation returns a structured error naming the file, and the server does not panic.
If a check name is declared with no function registered behind it, that check reports a
failure with evidence saying so, and the other checks still run.

---

## 8. Surfacing the health of the work

The gates catch problems at transitions. Some problems never reach a transition — they just
sit there. The proposal extends the routine status view with the lagging indicators:

**Tests.** When they last ran, how long ago, whether they pass, and which packages fail.
Read from stored results; don't re-run tests to answer a status query. Flag if failing, or
if the last run is more than a day old.

**Worktrees.** How many are active; how many are stale with no recent commits; how many
belong to completed work that never merged; how many have diverged badly; how many records
point at directories that no longer exist. Every non-zero count names the entities involved.

**Unmerged work.** Features and bugs marked done whose branches never landed. Work closed
with no review record. These are the lagging indicators that catch "the code was developed
but never merged" — the failure that's invisible until someone goes looking.

**Build currency.** Whether the running server matches what's installed. This one deserves
the top of the list, because when it's wrong it silently corrupts the meaning of everything
else on the page.

An ordering for attention items, worst first:

1. Running build out of date — it will make every other signal lie
2. Tests failing
3. Completed work unmerged
4. Worktrees for completed work still present
5. Work closed without review
6. Stale worktrees
7. Everything else

For Cromwell this belongs in the command centre rather than in a status tool. Most of the
data already exists — the audit log, the dispatch records, the worktree records, the token
ledger. It isn't gathered anywhere a human sees it at a glance.

---

## 9. Rolling it out safely

The sequence matters more than the content, because a badly-sequenced roll-out blocks
legitimate work on the first day and gets switched off.

1. **Surface first, enforce later.** Ship the health view before any new check blocks
   anything. It's pure read-side, it costs nothing, and it shows the real state of the
   world before anyone argues about thresholds.
2. **Write the checks, run them in observe-only mode.** Evaluate on every relevant
   transition, record the result and its evidence in the audit log, block nothing. Run them
   alongside current behaviour and compare.
3. **Make sure the audit trail is real before flipping.** Observe-only mode is worthless if
   the results aren't durably recorded. If the audit write fails, blocking must not be
   enabled — and the failure must be visible, not swallowed.
4. **Read the would-have-blocked events and triage them.** Two weeks is a reasonable
   observation period. Every one of these is either a real problem the check caught or an
   edge case the check gets wrong. Both need fixing before the flip.
5. **Flip to blocking.** One entity type at a time if the evidence is mixed.
6. **Add the definition of ready at claim time**, then the edit-time checks that depend on
   it.
7. **Add the inspection surface** once the rules are stable enough to be worth reading.

Two things to record while doing this:

- **Every evaluation writes exactly one audit row**, in observe-only mode and in blocking
  mode alike, carrying the entity, each check's result and evidence, and the overall
  outcome. The row is written before the response returns.
- **A check that fails required in blocking mode short-circuits the gate but not the
  evaluation loop.** Report every check's result, so whoever reads it fixes everything at
  once rather than discovering the next failure on the next attempt.

An honest exit condition for the work: several features and at least one smaller entity have
closed under the new gates, and either nothing closed by override, or every override has a
recorded reason and a matching audit entry.

---

## 10. Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Checks block legitimate work over an edge case — an unrelated dirty file, a generated artefact | Medium | Medium | The observe-only period exists precisely to find these. Triage every would-have-blocked event before flipping |
| A currency check blocks long-running work too aggressively | Medium | Low | Advisory by default, with a configurable threshold |
| Edit-time checks break genuine infrastructure work | Medium | Medium | An explicit exemption list, declared in advance — never a force flag reached for under pressure |
| A check depends on a tool that isn't available | Medium | Low | Advisory until the dependency is reliable; a missing dependency is a warning, not a failure |
| A declarative ruleset drifts from its registered functions | Medium | High | Assert one-to-one correspondence at startup. A declared name with no function is a startup error, not a silent pass |
| Evaluation logic leaks outside the registered checks | Low | High | Enforce by review and by test: check-name literals should not appear as evaluation logic anywhere else |
| A judged verdict silently passes bad work | Low | High | The evidence contract, enforced at parse time; the verdict derived from severity; mechanical checks run first |
| The health view becomes noise nobody reads | Medium | Medium | Strict ordering by severity, and every item names the entities involved so it's actionable |
