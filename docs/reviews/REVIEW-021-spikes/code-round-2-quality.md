# M14 spikes: code-quality review, round 2 (HEAD 00e918e)

gofmt and go vet are clean. Round-1 majors are all fixed except where noted. No new majors; the new problems are minor.

## New findings (all minor unless stated)

1. internal/store/spikes.go:484-487 `OpenSpikesUnderInitiative` is now dead. Production code calls `OpenSpikeIDsUnderInitiative` (http.go:169, ui_entity_actions.go:340); only spikes_test.go:481-485 uses the old one. It duplicates the recursive CTE in spike_open_ids.go. Fix: delete it and its test, or make it `len(ids)`. Also fold spike_open_ids.go into spikes.go or spike_reads.go (the store has five spike files).
2. internal/server/ui_spikes.go:302 and :595, mcp_spike_tools.go:212 compare against or key a map by the literals "override" and "default", although `store.BudgetFromOverride/BudgetFromDefault` now exist. A typo compiles and silently drops the note. Fix: use the constants.
3. internal/server/ui_spikes.go:583-590 still hand-rolls the dedupe loop (`have = have || x == t`). `slices.Contains` fixes it. The `"save_findings"`/`"finish_spike"` literals are still repeated here, in spikes_plan.go:161 and in `planSpike`. This is the round-1 #20 leftover.
4. internal/server/planner.go:146,261,312 and documents.go:536 still inline the logic of the new `turnCapFor(cfg, role)` (spikes_end.go). Leaving the old code is reasonable because it is pre-existing, but the helper now exists, so switching is a four-line change. It is also in the wrong file: it belongs in planner.go.
5. internal/server/ui_spikes.go (spikeOwnerOf): `"/ui/" + o.Type[:1] + "/" + o.Path` derives the URL prefix from the first letter of the type. It is brittle (it works only for feature and initiative). Fix: put a `URL` on `spikeOwnerInfo`, or use a two-entry map.
6. internal/server/ui_spikes.go (spikeNoticeFor): the "did" values are hard-coded in the redirect and not escaped. They are constants, so this is safe. A comment is enough.
7. internal/server/ui.go:1079 `groupThousands` is now a one-line wrapper around `dispatch.Thousands`, with 8 callers. Round-1 #3 asked for one implementation, and this achieves it. A cleaner end state is to call `dispatch.Thousands` directly, or move it to a leaf package, because server now imports dispatch for a number formatter. Optional.
8. internal/lifecycle/gates.go `joinIDs` repeats `joinWords` (server). Different packages, tiny; keep, but note it.
9. internal/timeline/timeline.go (Rules): spike moments use state "idea" as "neutral". That makes `data-state="idea"` mean "no state" in the CSS. A named neutral state, or an empty one the CSS styles as quiet, would not borrow the idea colour. The `.moment--aside` and `.journey-step--aside` styles are small and fine. timeline now imports store for `SpikeEndingOf`. That layering is acceptable (store is a leaf) but it is a new dependency for a phrase table.
10. internal/timeline/timeline.go `Strip.last` is an unexported field used only inside `Line`. Fine, but the `Current` loop (`out[i].last == cur`) is easy to misread. A one-line comment would help. `CurrentIndex` is named well and has a doc comment.
11. internal/server/spikes_leak.go:~165 `rev-list <made...> --not <base> <every RefsAtStart sha>` puts every ref on the command line. A repository with thousands of refs can hit ARG_MAX, and the leak check then fails closed, so every spike in such a repo ends with a "couldn't check" checkpoint. Fix: `rev-list --stdin`.
12. internal/server/spikes_leak.go, the `switch` in `leakedRefs`: `case name == "refs/stash" || strings.HasPrefix(name, "refs/tags/"):` has an empty body, and it works only because the code after the switch appends. Add a comment ("always a leak") or restructure as `isLeak := ...`.
13. internal/server/spikes_leak.go: `setOf`, `isSHA`, `reason`, `canonPath` are generic names in package server. Prefix them (`spikeReason`) or move the git helpers to a file named for them. `worktreeCommits` uses `slices.Contains` in a loop, which is quadratic; harmless.
14. internal/server/spikes.go:~700 (`writeFindings`) builds the findings text up to four times: `checkFindings`, `buildFindings`, and a defused check. The comment on the third build does not say why the question is replaced there. `isOrphanFindings`' caller rebuilds `filepath.Join(home,id)+".md"` by hand, which duplicates what `freePath` does with the extension. Not wrong; a short comment is enough.
15. internal/server/spikes.go (`startSpike`): `if budget != 0 { checkSpikeBudget(&budget) }` followed by `if budget == 0` is a little roundabout. The seam (`kick bool`) is the right size.
16. internal/server/ui_spikes.go:251,258 `numField`/`strField` still have generic names (round-1 #28, not fixed). Rename to `payloadInt/payloadStr`.
17. internal/server/ui.go uiFuncs: `code` and `closeForm` are global names for one template's use. `closeForm` builds a struct only to feed `{{template}}`, which is the idiom available in html/template, so it is acceptable. `code` is a generic name; `codeSpans` would match the Go function. `codeSpans` itself is correct (escapes first; the backtick is not escaped by HTMLEscapeString; an odd number of marks falls back to plain text).
18. internal/server/mcp_spike_tools.go:212 builds a `map[string]string{...}[source]` inline to translate a source. Declare it as a package-level var next to `spikeBudgetFor`.
19. Test quality. `integration_spikes_ui_test.go:44,53` `wants`/`lacks` (round-1 #37) are unchanged: the `wants` parameter still shadows the function. `store/spikes_test.go:481` tests dead code (finding 1). `setBuild`, `runLeakScript` and `keptCheckpoints` are good helpers. The new leak tests use `t.Run` subtests. I found no remaining `time.Sleep` in the spike tests. `startSpikeQuiet` still marks the dispatch running with raw SQL, which is a small hidden coupling, but it now goes through the real `startSpike`.
20. internal/server/mcp.go chat instructions: fixed; reads well.

## Round-1 verification

| # | Status | Evidence |
|---|---|---|
| 1 icons.html i-document | Fixed | icons.html:23 has the space; the diff touches only that line and the `i-spike` line |
| 2 budgetSource comment | Fixed | store/spikes.go constants `BudgetFromOverride/Default/StartScreen`, `SpikeStart.BudgetSource` comment; literals remain in 3 reading places (finding 2) |
| 3 commas duplicate | Fixed | `dispatch.Thousands`; ui.go:1079 wrapper (finding 7) |
| 4 postValuesGetLocation | Fixed | grep finds no such name |
| 5 100ms sleep | Fixed | spikes test:1015-1019 `h.quiet()` and a count check; no Sleep in spike tests |
| 6 startSpikeQuiet copy | Fixed | calls `srv.startSpike(..., false)` (spikes.go:~200) |
| 7 spikeDir | Fixed | helper at integration_spikes_test.go:158 now delegates (no hand-built "spk-" id found by grep) |
| 8 settle duplication | Fixed | `settle(sp, sweep)`, with a `spikeSettled` wrapper kept (97-110), used 20 times, reasonable |
| 9 wrong comment | Fixed | the function is gone; `SpikeEnding` table |
| 10 how-ended switches | Fixed | store/spike_endings.go; used in notAnswered, endedSentence, commit message, page, timeline. Three rule/MCP switches on the raw value remain, which is fine since they map to state, not words |
| 11 dead branch, needsFill | Fixed | spikes_plan.go:186, spikes.go needsFill used in both |
| 12 removed/prune | Fixed | removed flag and trailing prune deleted (spikes_end.go diff) |
| 13 memberFromForm query | Fixed | ui_plan_actions.go:508-535, with `memberName` and a comment |
| 14 spikeResult dup | Fixed | uses `spikeOwnerRef` and `spikeBudgetFor` (map literal, finding 18) |
| 15 owner derivation | Fixed | `Spike.Owner()` used in CloseSpike, notifySpikeChanged, spikeOwnerSection, spikeOwnerPath |
| 16 raw SQL in server | Fixed | `store.SpikeKeptRecord`/`SpikeKept`; errors now propagate (spikeKept returns err) |
| 17 SpikeConfig | Fixed | `*int64`; accessor is the only default; validate no longer mutates |
| 18 spikePage dead fields | Fixed | State/Used/Budget/Answered/DraftSavedAt/CanCloseIdea removed; SD12 -> BuildOnSentence; FollowBy -> FollowedBy; the Workers/Running/Free trio kept (used at spike.html:311) |
| 19 run/runs | Not checked in detail (minor); not in the fix diff |
| 20 slices.Contains and tool const | Not fixed (finding 3) |
| 21 bad id 404 vs 400 | Fixed, consistently 400 (ui_spikes.go ~629) |
| 22 budget<0 duplicate | Fixed via `checkSpikeBudget` |
| 23 SpikeCloseAgain comment | Fixed |
| 24 turn cap | Partly: `turnCapFor` used by planSpike and spikeTurnCap, `roleOnly` removed, config error no longer reported as 0 turns in the findings path; planner.go/documents.go left as they were (reasonable, finding 4) |
| 25 backtickJoin | Fixed (spikes_leak.go; used in keptParagraph and spikeKept) |
| 26 findingsFront | Fixed (constant plus a single NewReplacer with the ordering comment) |
| 27 dropTitle | Fixed (fence-aware) |
| 28 numField/strField | Not fixed (finding 16) |
| 29 spikeWhen Local | Changed to no `.Local()`; the zone is whatever pgx returns. Acceptable |
| 30 rules literals | Fixed (store.* and dispatch.* constants) |
| 31 FinishSpikeTool | Fixed |
| 32 mcp.go instructions | Fixed |
| 33 store file layout, duplicate follow reads | Not fixed: spike_mcp_reads.go (13 lines) remains, `SpikesFollowing` and `SpikeFollowedBy` both remain. Minor, leaving is acceptable; a new spike_open_ids.go makes the file count worse |
| 34 SpikesToReconcile Scan | Fixed (`scanSpike(row, extra...)`) |
| 35 testdb suffix | Not fixed, scope creep, in its own commit 9eb4f33 so it can be dropped separately; acceptable |
| 36 Terminal() | Not fixed; ui_entity.go:612 is pre-existing; acceptable |
| 37 wants/lacks names | Not fixed (finding 19); minor |
| 38 rule loop can't fail | Fixed (`len(actions) != 1` check at the new test); the route loop was not examined |
| 39 split tests | Partly: leak tests use `t.Run`; the long appendix/no-merge tests were left, which the caller said was knowing and is reasonable |
| 40 spike.html close form, aria | Fixed (`spike-close-form` template, aria-describedby on all four inputs; `{{code .Error}}` added) |

## Whole-branch pass

I read the leak check (spikes_leak.go), the findings pipeline, the timeline aside handling, the store changes, the template and the config change. I found nothing major. Items worth a human look: finding 11 (ARG_MAX), finding 9 (aside state "idea"), and findings 1-3, which are cheap cleanups.
