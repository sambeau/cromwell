# How M13 and M14 are built

**Date:** 2026-10-02
**For:** Sam, the M13 and M14 lead sessions, and whoever builds the next
milestones the same way.
**Asked for by Sam:** build M13 and M14 as logically and in as parallel a way
as possible, using suitably classed agents. Include at least one review cycle
that checks code quality as well as bugs and spec mismatch. Use Subutai's own
definition of done, so that code is submitted and `main` stays up to date and
clean.

## The shape

M13 (executors) and M14 (spikes) are built at the same time, each by a
**lead session**. Each lead orchestrates its own agents through the stages
below. I integrate the finished work into `main`.

| Milestone | Builds | Depends on |
|---|---|---|
| **M13, executors** | `claim_task` and `submit_task`, the executor on every task, expiry of stale claims, the check for unclaimed commits, and *unmeasured* tokens (DEC-007) | DEC-007, M10 |
| **M14, spikes**, stage 1 | Spikes run by a dispatched agent. A person starts them in the UI. Each has a token budget that stops it hard, a findings document, a worktree that is thrown away, and no merge path (DESIGN-010 §10) | M8 |
| **M14, spikes**, stage 2 | Spikes run by the chat AI or a person, held to a time box through M13's claims | **M13 merged** |

M14's stage 2 is the only real dependency between the two, so it waits for M13
to merge. Everything else runs in parallel.

**Migrations:** M13 owns `0014` and M14 owns `0015`. **M13 merges first.**

## Who does what

Each class of agent does the work that suits it. The spec gets the strongest
model, as DESIGN-010 says; implementation gets the fast, capable one.

| Role | Model | Why |
|---|---|---|
| **Lead**: writes the spec, plans the tasks, orchestrates, integrates, writes the handoff | Opus 5.5 (the session's own model) | The spec is the document everything else rests on |
| **Spec reviewer**: an independent consistency review of the spec | Opus subagent | Judging a spec against the decisions needs depth |
| **Implementers**: one per independent task, run in parallel in isolated worktrees | Sonnet subagents | Well-specified implementation work |
| **Bug and spec-conformance reviewer**: correctness, races, security, and every FR's acceptance criterion against the code | Opus subagent | Finding real defects is judgement work |
| **Code-quality reviewer**: simplicity, reuse, idiom consistency, naming, comment density, dead code, test quality | Sonnet subagent | A careful, mechanical read |
| **Verifier**: checks every acceptance criterion with specific evidence | Opus subagent | The last gate. "Looks fine" is not evidence |

Sam named Sonnet 5.5. The Sonnet model available here is Sonnet 5, so that is
the one used.

## The stages, in each lead session

1. **Spec.** The lead writes `SPEC-020` (M13) or `SPEC-021` (M14), with
   functional requirements, acceptance criteria and the choices for Sam.
2. **Spec review.** An independent Opus subagent reviews the spec, and the lead
   fixes what it finds. This is recorded as `REVIEW-020` or `REVIEW-021`.
3. **Development plan.** A task table in Subutai's own dev-plan shape, with
   ids, dependencies, and the files each task touches. Tasks with no
   dependency between them run together.
4. **Implementation.** Sonnet subagents, one per task, each in its own git
   worktree. Each runs `go vet`, `gofmt`, and the tests for the packages it
   touched. The lead merges each one when it's done.
5. **The review cycle.** This is mandatory, with at least one round. Two
   reviewers run in parallel on the whole diff:
   - **bugs and spec conformance** (Opus);
   - **code quality** (Sonnet).

   Every finding carries a severity. *Major* findings are fixed and reviewed
   again; the cycle repeats until none remain. *Minor* findings are listed in
   the handoff as bug reports, ready for M12's triage queue. After three rounds
   with majors still open, the lead stops and asks Sam.
6. **Verification.** An Opus subagent checks each acceptance criterion and
   cites evidence for it: a test name, a `file:line`, or a demo screenshot.
7. **Definition of done** (below). Then the walkthrough, the handoff, and a
   push to the lead's own branch.

## The definition of done

These are Subutai's own feature checks
([definition-of-done research](../research/definition-of-done-and-ready.md)
§3), applied to the work of building Subutai. Each check reports pass or fail,
with a line of evidence.

| Check | What it means here | Who checks it |
|---|---|---|
| `git_status_clean` | Nothing is left uncommitted on the branch | Lead |
| `tests_pass` | `go vet` clean, `gofmt -l` empty, and `go test -race -count=1 -v ./...` green, with the integration tests seen to run, not skip | Lead, then me again after integrating |
| `contract_documents_approved` | The spec has been approved by Sam | Sam |
| `all_tasks_terminal` | Every task in the dev plan is done or explicitly dropped | Lead |
| `reviews_present` | The spec review, plus at least one round of both code reviews, with no major findings open | Lead |
| `verification_passed` | Every acceptance criterion has cited evidence | Verifier |
| `worktree_removed` | Every subagent worktree is merged and removed, and `git worktree list` shows only the main checkout | Lead |
| `branch_merged` | The work is in `main`, and CI on `main` is green | Me |

**When work reaches `main`.** I merge a milestone into `main` once every check
above passes except approval, the integration checks between the two
milestones pass, and the full suite is green on the combined code. If Sam has
already approved the spec, `contract_documents_approved` passes at the same
time. Otherwise the approval is recorded when it comes, as it was for M3 to
M12. Sam can tell me to wait for approval before merging instead.

**Branches.** After a merge, the milestone's branch should be deleted. This
session can't delete branches other sessions created, so each lead deletes its
own once I've merged it. If that's refused too, the handoff says so, and Sam
gets the one-line command.

## The integration check, between M13 and M14

Two builds running in parallel always leave gaps, as M9 and M10, and M11 and
M12, showed. When I merge M14 after M13, I check the following and add tests
for anything missing:
- A spike run in chat or by a person is held by M13's claim expiry.
- A spike's executor is recorded.
- A chat-run spike's tokens are marked *unmeasured*.
- Neither milestone's tools escape the MCP tool-set test.
