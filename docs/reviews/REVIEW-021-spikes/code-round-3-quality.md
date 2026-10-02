# M14 spikes: code-quality review, round 3 (HEAD 4a565d3)
gofmt and go vet clean. No majors. Fix diff reviewed (13 files).

## Round-2 findings
1 dead OpenSpikesUnderInitiative: FIXED (func and test deleted; renamed test TestEndedSpikesCount). spike_open_ids.go still a separate file (minor, left). Subtree CTE now covered only indirectly (lifecycle TestG5CountsOpenSpikes, integration TestOpenSpikesBlockArchiving...), acceptable.
2 "override"/"default" literals: FIXED at ui_spikes.go:306, :595; mcp_spike_tools.go uses new spikeBudgetSourceWords map (keyed by constants, package-level). #18 FIXED.
3 dedupe loop: FIXED (slices.Contains, tool names from dispatch.SaveFindingsTool().Name/FinishSpikeTool().Name). Literals in spikes_plan.go:161/planSpike not re-checked, left.
4 turnCapFor still in spikes_end.go:147, planner.go/documents.go not switched: LEFT.
5 spikeOwnerOf prefix: FIXED (URL field on spikeOwnerInfo, Owner() used).
6 spikeNoticeFor: now state-checked and tested (TestSpikeNoticeSaysOnlyWhatHappened). Fixed.
7 groupThousands wrapper: LEFT (optional).
8 joinIDs/joinWords: LEFT.
9 timeline "idea" neutral state (timeline.go:255): LEFT.
10 Strip.last comment: LEFT.
11 ARG_MAX: FIXED via rev-list --stdin (see N1 for placement).
12 empty switch case: FIXED with comment.
13 generic names setOf/isSHA/reason/canonPath: LEFT.
14 writeFindings rebuilds: LEFT.
15 startSpike budget roundabout: LEFT.
16 numField/strField (ui_spikes.go:255,262): LEFT.
17 `code` template func name: LEFT.
19 wants/lacks shadowing (integration_spikes_ui_test.go:44,53): LEFT.

## New findings in the fix (all minor)
N1 gitInWithStdin sits in planner.go:482 beside gitIn. That is defensible (they are twins) but the only caller is spikes_leak.go, and the two functions duplicate ~8 lines (error format, stderr capture). Make gitIn call gitInWithStdin(dir, "", args...) (nil Stdin when empty), or move both to a git helper file.
N2 `SpikeEnding.Run` is a good fit for the table, but observe.go:750 changed behaviour: SpikeFailed and any unknown SpikeHow now produce a spike conclusion ("Its run failed." / "Its run is over.") where before they fell through to the outcome-JSON path. Probably intended and better, but not mentioned in the commit; the fallback sentence "Its run is over." is a fifth literal repeated in the SpikeEndingOf default. Also the `p.SpikeHow != ""` guard keeps the old shape; fine.
N3 spikeKept now has two callers that must hold spikeEndMu (finishSpikeEnding path and ensureSpikeWorktree, spikes_end.go:248-250). The lock/unlock pair is inlined three times (68, 248, 312) and the requirement is only in the ensureSpikeWorktree comment, not on spikeKept's doc. Add "callers hold spikeEndMu" to spikeKept, or a small `s.withSpikeEnd(func)` helper. Check it cannot deadlock: ensureSpikeWorktree runs in planSpike, not under spikeEndMu, so fine today.
N4 ensureSpikeWorktree comment is now a 6-line paragraph mixing two concerns (what it makes, why it records first). Reads fine but "the end of the run then finds that record" relies on spikeKept's prior-record shortcut; a test covers it (damaged working copy subtest).
N5 `escapeLeadingHash` handles only "#" at the start; the question is one line after oneLineKeep, so that is enough, but leading whitespace is trimmed first (order: TrimSpace inside escape call, correct). A question starting with "---", "=== " or "> " is not a heading risk. Fine. Doc comment accurate. Test is appended to TestSpikeWithATodoQuestionConcludes rather than a subtest, which makes that test longer (it now creates two spikes with unrelated `hash` var shadow of nothing but reusing `text`); a t.Run would match the leak tests.
N6 TestSpikePostsAnswerBadIDs: name says POSTs but it also checks a GET at the end, using bare http.Get with the resp body never closed and `resp` printed with %v when nil-or-not. Use h's get helper (as the neighbouring TestSpikeBadIDsAnswerAlike does if it has one) and close the body. The test comment says "as the page's GET does for an unknown one" which is the justification, fine.
N7 TestSpikeNoticeSaysOnlyWhatHappened: good table test, names clear, builds Spike literals directly. Minor: case names "answered, answered" / "unanswered, unanswered" read as typos; "did=answered on an answered spike" is clearer. Missing case: started with State closed (ran, then closed) which spikeNoticeFor accepts via StartedAt only; arguably fine.
N8 the two new leak subtests are good (they assert the checkpoint payload and audit counts once), but the "retry would remake" subtest queries checkpoints with raw SQL inline (`SELECT count(*) FROM checkpoints ...`) with context.Background() while the rest use the harness; the keptCheckpoints helper already returns the same rows (it is asserted one line above with len(cps) != 1), so the raw count is redundant. Remove it.
N9 the "deleted and pruned" subtest builds its script by string-concatenating h.root into single quotes; a path with a quote breaks it. Test-only, harmless in t.TempDir.
N10 spikes_leak.go: `if kept.Refs == nil { kept.Refs = []string{} }` is there so JSON gives [] not null; comment says nothing. Add "so the payload says [] not null". The new `payload` map also now drops "couldnt_check" when empty, good.
N11 leakedRefs new error text "the working copy was removed, so git's record of it is gone" is duplicated as a literal between spikes_leak.go and the test's expected sentence; acceptable for a test.

## Consolidated open quality minors (rounds 1-3), bug-report candidates
R1/R2 leftovers:
- store spike_open_ids.go (13-line file) and spike_mcp_reads.go: file layout; SpikesFollowing vs SpikeFollowedBy duplicate reads (r1 #33).
- turnCapFor (spikes_end.go:147) belongs in planner.go; planner.go:146,261,312 and documents.go:536 still inline it (r2 #4).
- groupThousands wrapper over dispatch.Thousands, 8 callers; server imports dispatch for a formatter (r2 #7).
- lifecycle joinIDs duplicates server joinWords (r2 #8).
- timeline.go:255 aside moments use state "idea" as neutral, so data-state="idea" means "no state" in CSS; timeline imports store for a phrase table (r2 #9).
- timeline Strip.last / Current loop lacks a comment (r2 #10).
- spikes_leak.go generic helper names setOf, isSHA, reason, canonPath; worktreeCommits quadratic slices.Contains (r2 #13).
- spikes.go writeFindings builds findings up to 4 times; uncommented defused rebuild; hand-built filepath.Join(home,id)+".md" duplicates freePath (r2 #14).
- startSpike budget != 0 / == 0 roundabout (r2 #15).
- ui_spikes.go:255,262 numField/strField generic names (r1 #28, r2 #16).
- ui.go `code` template func name (r2 #17).
- wants/lacks helper parameter shadows function (integration_spikes_ui_test.go:44,53) (r1 #37).
- startSpikeQuiet marks dispatch running with raw SQL (r2 #19).
- testdb suffix change is scope creep in its own commit 9eb4f33 (r1 #35).
- ui_entity.go:612 Terminal() pre-existing (r1 #36).
- long appendix/no-merge tests not split (r1 #39).
- spikes_plan.go:161 and planSpike still repeat tool-name literals (r2 #3, partly).
R3 new: N1 (gitIn/gitInWithStdin duplication and placement), N2 (observe.go behaviour change and repeated fallback literal), N3 (spikeEndMu requirement undocumented on spikeKept, lock triple inlined), N5 (hash test appended instead of subtest), N6 (TestSpikePostsAnswerBadIDs name/GET/unclosed body), N7 (case names), N8 (redundant raw SQL in subtest), N10 (Refs nil comment).
