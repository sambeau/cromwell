# M14 spikes: code-quality review, round 1

gofmt and go vet are clean on internal/server, store, timeline and starter. File sizes are acceptable (spikes.go 727, ui_spikes.go 723).

## Major

1. **internal/server/ui/templates/icons.html:23** (major). The diff rewrote the existing `i-document` symbol to `<symbol id="i-document"viewBox=...`, with the space before `viewBox` missing. This is an unrelated edit to a line that was fine. It is malformed markup, and spike.html uses `#i-document` twice. Fix: restore the line byte-for-byte from main and keep only the added `i-spike` line.

2. **internal/store/spikes.go:225-228, internal/server/spikes.go:225** (major, misleading comment). The `StartSpike` doc says budgetSource is "override", "default" or "entered". The server passes `"start screen"` (or the override/default source). The code and the comment disagree. Fix: list the real values ("override", "default", "start screen"), or define constants in store and use them in `StartSpike`, `startSpikeQuiet` and the tests.

3. **internal/dispatch/dispatch.go (new `commas`)** (major, duplicate helper). `commas` re-implements the thousands separator that `groupThousands` already provides in internal/server/ui.go:1074. The spike code now has two formatters with different code. Fix: move one implementation into a small shared package (for example `internal/textfmt`) and use it from both. Alternatively, make `stopRun` take the figures preformatted.

4. **internal/server/integration_spikes_ui_test.go:369-378** (major, duplicate helper, misleading name). `postValuesGetLocation` is a copy of `redirectOf` in integration_identity_test.go:115. Despite its name it does a GET, and it is used for `/ui/id/...`. Fix: delete it and call `h.redirectOf`.

5. **internal/server/integration_spikes_test.go:976** (major, flaky). A bare `time.Sleep(100 * time.Millisecond)` follows three `RetrySweep` calls, with no condition behind it. The wait is meant to let a republished event be handled, but 100ms is a guess. If the event is slower, the `spike.ended == 1` assertion passes without checking anything. Fix: make the sweeps deterministic. For example, call `h.quiet()` (the helper used elsewhere in the suite), or poll `h.eventually` for the dispatch queue to be empty and then assert.

6. **internal/server/integration_spikes_test.go:73-99 `startSpikeQuiet`** (major, test reimplements the code under test). It copies the body of `StartSpike`: the transaction, `store.StartSpike`, `EnqueueDispatch` with a hard-coded role "spike-runner" and model "claude-sonnet-5", and the `"start screen"` source. When `StartSpike` changes, the helper drifts and the tests still pass against the old shape. Fix: give `StartSpike` an unexported seam (for example `startSpike(ctx, id, budget, actor, kick bool)`, where the public method passes true), or start with the real `StartSpike` and then mark the dispatch running. Do not repeat the SQL path in the test.

7. **internal/server/integration_spikes_test.go:165-168 `spikeDir`** (major). The helper hand-builds `"spk-"+id[len-6:]`, which re-implements `store.ShortID("spk", id)`. If the short-ID rule changes, the test checks the wrong path, and `os.IsNotExist` then passes spuriously. Fix: use `store.ShortID("spk", sp.ID)` (or `h.srv.worktreeAbs(h.srv.spikeWorktreeRel(sp.ID))`).

8. **internal/server/integration_spikes_test.go:101-120 `spikeSettled` / `sweepUntilSettled`** (major, duplication). The two helpers are identical except for the `RetrySweep` call. Fix: one helper, `settle(sp, sweep bool)`.

9. **internal/server/ui_spikes.go:55** (major, wrong comment). The comment reads `// spikeEndReason says how a run ended, to follow "its run has ended: "`, but the function is `spikeEndedWords`. `spikeEndReason` is a different function in spikes_end.go. Fix: correct the name.

10. **"How a run ended" is switched on in five places** (major, duplication). The places are `notAnswered` (spikes.go), `endedSentence` (spikes.go), `spikeEndReason` (spikes_end.go), `spikeEndedWords` (ui_spikes.go) and `timeline.spikeEndWords`. Three more switches sit in the rules, `conclude` (observe.go) and the MCP tool. Each handles the four values separately, so adding a value means editing every one, and the wording has already drifted ("concluded" / "the agent reached a conclusion" / "it reached a conclusion"). Fix: put the how-to-words table in one place. For example, add `func (How) Words()` or a `var endWords = map[string]struct{ Commit, UI, Timeline string }` next to the constants in store/spikes.go, and have the callers index into it.

11. **internal/server/spikes_plan.go:150-153 `validateFinishSpike`** (major, dead branch and duplicated logic). The `(h == headingFound && strings.TrimSpace(t) == "")` clause can never matter: `isPlaceholderAnswer("")` is already true. The "Answer or What we found is empty or placeholder" rule is also written a second time in `buildFindings` (spikes.go, the `answer` / `found` fill-ins). Fix: drop the redundant clause. Better, extract `needsFill(body) bool` and call it from both places.

12. **internal/server/spikes_end.go:~395 `removed` / `worktree prune`** (major, dead code). `removeSpikeDir` already runs `git worktree prune` on every call, so the `removed` flag and the final prune in `ReconcileSpikes` do nothing extra. Fix: delete `removed` and the trailing prune.

13. **internal/server/ui_plan_actions.go:508** (major, odd pattern with a hidden cost). `if _, serr := store.GetSpike(ctx, ..., memberID); memberType == "spike" || serr == nil` runs a database query on every milestone-member add, just to detect a spike. `memberType == "spike"` is checked after the query, so the query always runs. The `serr == nil` check by itself is cryptic. Fix: check `memberType == "spike"` first and add a comment saying why the id lookup is needed (the id can arrive with another type); better, reuse the type switch already below, which looks up by type.

14. **internal/server/mcp_spike_tools.go:`spikeResult`** (major, duplication). It rebuilds the owner lookup (`spikeOwnerOf` in ui_spikes.go does the same walk with `featurePath` and `initiativePath`) and the budget source logic (`spikeBudgetFor`, plus the "given at the start" case). Fix: use `spikeOwnerOf`, or extract an `ownerRef` returning id, name, path and type. Make `spikeBudgetFor` handle the `TokenBudget` case too (`budgetFor(cfg, sp) (n, source)`) and map its source to the MCP wording.

15. **internal/server/spikes.go: owner type/id derivation repeated** (major). `ownerType, ownerID := "initiative", ...; if FeatureID != nil {...}` appears in `CloseSpike` and in `spikeOwnerSection` (spikes_plan.go). `notifySpikeChanged` and `spikeOwnerPath` branch on the same condition. Fix: one method, `func (sp *Spike) Owner() (typ string, id uuid.UUID)`, in store, and use it in all four places.

16. **internal/server/spikes_end.go: raw SQL in the server** (minor to major). `recordedKeptRefs` queries `audit_events` directly. The branch's own store layer has `SpikeHistory`, and this file's neighbours use store functions. This is common elsewhere in server, so it is not wrong, but it is inconsistent within this feature. Fix: add `store.SpikeKeptRefs(ctx, q, id) ([]string, bool, error)`. This also stops it swallowing database errors as "not recorded", which would lead to a second leak checkpoint.

17. **internal/config/config.go SpikeConfig.UnmarshalYAML** (major, over-engineering). A custom unmarshaller exists only to learn whether a key was present. `validate()` then overwrites 0 with the default, and `SpikeDefaultTokenBudget()` re-defaults `<=0`, so the default is applied twice. Fix: `DefaultTokenBudget *int64 `yaml:"default_token_budget"``. Validate with `!= nil && *p <= 0`. Keep the accessor as the only place that applies the default, and remove the mutation in `validate()`.

18. **internal/server/ui_spikes.go `spikePage` struct** (major, dead fields). `State`, `Used`, `Budget`, `Answered` and `DraftSavedAt` are set but never read by spike.html or Go (verify with `grep`; `FindingsID` is only used to build `SD12`). The `StartScreen.Workers/Running/Free` trio could be a single sentence. Fix: delete the unused fields. Rename `SD12` to `BuildOnSentence` (a spec decision number is not a name for a reader). Rename `FollowBy` to `FollowedBy`.

19. **internal/server/ui_spikes.go:~340 and the `run`, `runs` variables** (minor). `run, runs := "", []store.RunSummary(nil)` followed by `if len(runs) > 0` re-checks what the earlier `if` already knew. Fix: compute `last := runs[len(runs)-1]` once inside the first `if`.

20. **internal/server/ui_spikes.go `startScreenFor`** (minor). The tool-dedupe loop (`have := false; for ... have = have || x == t`) should be `slices.Contains`. The "save_findings" / "finish_spike" tool list, written here by hand, is also in `planSpike` and must be kept in step. Fix: `slices.Contains`, and one `spikeRunTools = []string{"save_findings", "finish_spike"}` constant.

21. **internal/server/ui_spikes.go handlers** (minor, inconsistent). `handleUISpikeStart` answers a bad id with `uiNotFound`, while `handleUISpikeStartPost` and `handleUISpikeClose` answer `http.Error(400, "bad spike id")`. A bad `?spike=` on the GET is 404 and the same bad id on the POST is 400. Fix: pick one (404 via `uiNotFound`, as in bug handlers).

22. **internal/server/spikes.go StartSpike:186-195** (minor). The `case budget < 0` sentence is a duplicate of `checkSpikeBudget`. Fix: build a `*int64` and call `checkSpikeBudget`, or call a shared `errBudget` var.

23. **internal/server/spikes.go:21-22 `SpikeCloseAgain`** (minor). The doc comment starts "Ways a person closes..." and does not name the constant; it is also the only exported string constant here without a `Spike` prefix pattern (`store.SpikeAnswered` is the style). Fix: `// SpikeCloseAgain is the third way ...` and put it next to `store.SpikeAnswered/Unanswered` (these are the values of `as`).

24. **internal/server/spikes_end.go `spikeTurnCap` / `roleOnly`** (minor). `roleOnly` is a poor name (it is "role's own turn cap"). `spikeTurnCap` swallows config errors and returns 0, which makes the findings say "turn limit of 0 turns". `planSpike` computes the same turn cap with the same fallback in `turnCap := cfg.Dispatch.TurnCap; if role.Limits != nil ...`, and so does planner.go:146 and :261. Fix: one `func (s *Server) turnCapFor(cfg, role)` used by all three; rename `roleOnly`.

25. **internal/server/spikes.go `endedSentence` and `spikeKeptRefs`** (minor). `quoted[i] = "`" + r + "`"` is written three times (here, spikes_end.go, ui_spikes.go timeline). Fix: `backtickJoin(refs)`.

26. **internal/server/spikes.go `findingsFront`** (minor). Two `strings.NewReplacer` passes to handle quoted/unquoted `{{title}}`, with the fallback string as a literal built inside the function. Fix: make the fallback a package constant, and say in a comment why the quoted replacer exists.

27. **internal/server/spikes.go `dropTitle`** (minor). It removes every line starting `# ` and not only a leading level-1 heading, including `# ` lines inside fenced code (a shell comment in a code block). The comment says "a leading level-1 heading". Fix: remove only the first line if it is `# `, or skip fenced code as `splitFindings` does.

28. **internal/server/ui_spikes.go:229-242 `numField`/`strField`** (minor). Generic names in package server for a timeline payload. Fix: unmarshal into a typed struct per kind, or prefix `payload` (`payloadInt`, `payloadStr`).

29. **internal/server/ui_spikes.go `spikeWhen`** (minor). It uses `t.Local()`, so output depends on the machine time zone and no test can assert it. Use the same time-formatting helper the other pages use (`ago` / existing format func) or pass UTC in tests.

30. **internal/rules/rules_spike.go** (minor). `"concluded"`, `"failed"`, `"budget"`, `"turn_limit"` are literals while `dispatch.StopBudget/StopTurnLimit` and `store.Spike*` exist. A typo here would silently end a spike as the wrong thing. Fix: import the constants (or define the `how` constants once in a leaf package the three can share).

31. **internal/dispatch/tools.go** (minor). `SaveFindingsTool` / `FindingsOutcomeTool`: the second is named after the document, the first after the tool, and the outcome tool is `finish_spike`. Rename to `FinishSpikeTool`.

32. **internal/server/mcp.go:318** (minor). The chat instructions edit splices a new sentence into the middle of "you hold no verdict of your own. You cannot send ..." so the sentence now reads awkwardly (`You may write down a spike's question ... You cannot send work to development...`). Fix: move the spike sentence before "you hold no verdict" or give it its own string line.

33. **internal/store/spike_reads.go, spike_mcp_reads.go, spikes.go** (minor, organisation). Three files for one entity, where one is 13 lines. `SpikesFollowing` (list) and `SpikeFollowedBy` (first of the same query) duplicate each other. Fix: fold `spike_mcp_reads.go` into `spike_reads.go`, and have `SpikeFollowedBy` call `SpikesFollowing` or have the UI use the list too.

34. **internal/store/spikes.go `SpikesToReconcile`** (minor). It repeats the 28-column Scan list from `scanSpike`, so adding a column means editing two lists. Fix: make `scanSpike` take extra dests (`scanSpike(row, extra ...any)`).

35. **internal/testdb/testdb.go** (minor, scope). The `SUBUTAI_TEST_DB_SUFFIX` change has nothing to do with spikes. Move it to its own commit, or drop it.

36. **internal/server/ui_entity.go (existing `page.IsTerminal = f.State == "done" || ...`)** and `spikeOwner` (`f.State.Terminal()`) (minor). Two ways of asking "done or abandoned" now sit near each other. Fix: use `f.State.Terminal()` in both (the existing line is not in the diff, so optional).

## Test-quality nits (minor)

37. **integration_spikes_ui_test.go:39-56** (minor). `wants(t, what, body string, wants ...string)` shadows the function with its own parameter, and the helper names `wants` / `lacks` / `i64` / `jsonText` / `contains` are generic in a package shared by many test files (there is already a `contains` in integration_phase3_test.go). Fix: `expectIn` / `expectNotIn`, rename the parameter `parts`.

38. **integration_spikes_test.go:1385-1395** (minor, assertion that can't fail). The "rules emit no merge" loop passes if `Decide` returns an empty slice, so it cannot fail on an empty result. Add `if len(actions) != 1`. The route loop (`/api/spikes/merge` etc. returning 404/405) tests that routes were never created, so it passes before any code exists; it is cheap, but it is not a test of the code. Keep one representative check.

39. **integration_spikes_test.go** (minor, size). `TestSpikeHasNoMergePath` (~120 lines), `TestSpikeAppendixSites` and `TestSpikeStartsFromTheWebUIOnly` each check six or seven unrelated things in sequence. The first failure hides the rest (`t.Fatalf` in the middle). Fix: split into subtests with `t.Run`, one per FR.

40. **internal/server/ui/templates/spike.html** (minor). Mostly consistent with bug.html (panels, banners, labelled inputs). The three `<form method="post" action="/ui/spikes/close">` blocks repeat the hidden spike field; the "Close without running" and "Close without an answer" button/form pairs are the same markup. Fix: `{{define "spike-close-form"}}` taking `as`, label and icon. The `again-budget` input has a label but its hint is outside `aria-describedby`; add `aria-describedby` on the three budget inputs that have a `field-hint`.
