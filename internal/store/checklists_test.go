package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The store half of SPEC-014: checklists and jobs, and how a checklist counts
// in a milestone's progress, in G4 and in marking a milestone as shipped.

func mustTx(t *testing.T, s *Store, fn func(tx pgx.Tx) error) {
	t.Helper()
	if err := s.WithTx(context.Background(), fn); err != nil {
		t.Fatal(err)
	}
}

func auditRows(t *testing.T, s *Store, kind string, ref uuid.UUID) []map[string]any {
	t.Helper()
	rows, err := s.Pool.Query(context.Background(), `
		SELECT actor, payload FROM audit_events WHERE kind = $1 AND ref_id = $2 ORDER BY occurred_at, id`, kind, ref)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var actor string
		var raw []byte
		if err := rows.Scan(&actor, &raw); err != nil {
			t.Fatal(err)
		}
		p := map[string]any{}
		_ = json.Unmarshal(raw, &p)
		p["_actor"] = actor
		out = append(out, p)
	}
	return out
}

func jobTitles(t *testing.T, s *Store, checklistID uuid.UUID) []string {
	t.Helper()
	jobs, err := Jobs(context.Background(), s.Pool, checklistID)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for i, j := range jobs {
		if j.Position != i+1 {
			t.Errorf("job %q at position %d, want dense %d", j.Title, j.Position, i+1)
		}
		out = append(out, j.Title)
	}
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestChecklistStore covers FR-1.1 to FR-1.3: creating checklists with each
// owner, the table's CHECKs, adding, editing, moving and removing jobs with
// dense positions, ticking and unticking with who, how and the person's words,
// and one audit row per act.
func TestChecklistStore(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	in, _, _ := seedTree(t, s)

	// FR-1.2: each owner kind; a blank name or a bad owner writes nothing.
	var cl *Checklist
	mustTx(t, s, func(tx pgx.Tx) error {
		var err error
		cl, err = CreateChecklist(ctx, tx, "initiative", &in, "Launch chores", "Things only a person can do.", "sam")
		if err != nil {
			return err
		}
		_, err = CreateChecklist(ctx, tx, "project", nil, "Paperwork", "", "sam")
		return err
	})
	if got, _ := ChecklistsOwnedBy(ctx, s.Pool, "initiative", &in); len(got) != 1 || got[0].Name != "Launch chores" {
		t.Errorf("initiative's checklists = %+v", got)
	}
	if got, _ := ChecklistsOwnedBy(ctx, s.Pool, "project", nil); len(got) != 1 || got[0].Name != "Paperwork" {
		t.Errorf("project's checklists = %+v", got)
	}
	if rows := auditRows(t, s, "checklist.created", cl.ID); len(rows) != 1 || rows[0]["owner_type"] != "initiative" {
		t.Errorf("checklist.created rows = %v", rows)
	}
	for _, bad := range []func(tx pgx.Tx) error{
		func(tx pgx.Tx) error { _, err := CreateChecklist(ctx, tx, "project", nil, "  ", "", "sam"); return err },
		func(tx pgx.Tx) error { _, err := CreateChecklist(ctx, tx, "feature", &in, "X", "", "sam"); return err },
		func(tx pgx.Tx) error {
			_, err := CreateChecklist(ctx, tx, "initiative", nil, "X", "", "sam")
			return err
		},
	} {
		if err := s.WithTx(ctx, bad); err == nil {
			t.Error("a blank name or a bad owner was accepted")
		}
	}
	// FR-1.1: the table refuses a bad owner even if the store's check is skipped.
	if _, err := s.Pool.Exec(ctx, `INSERT INTO checklists (id, name, owner_type, owner_id) VALUES ($1, 'x', 'project', $2)`,
		NewID(), in); err == nil {
		t.Error("the checklists table accepted a project owner with an id")
	}
	if all, _ := ListChecklists(ctx, s.Pool); len(all) != 2 {
		t.Errorf("ListChecklists = %d, want 2", len(all))
	}
	if got, err := ChecklistByName(ctx, s.Pool, "Launch chores"); err != nil || got.ID != cl.ID {
		t.Errorf("ChecklistByName = %v, %v", got, err)
	}
	if _, err := ChecklistByName(ctx, s.Pool, "Nothing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown name: %v", err)
	}

	// Adding: at the end, dense; a blank title is refused.
	var key, contract, icon *Job
	mustTx(t, s, func(tx pgx.Tx) error {
		var err error
		if key, err = AddJob(ctx, tx, cl.ID, "Get the API key", "", "sam"); err != nil {
			return err
		}
		if contract, err = AddJob(ctx, tx, cl.ID, "Sign the contract", "With the lawyers.", "sam"); err != nil {
			return err
		}
		icon, err = AddJob(ctx, tx, cl.ID, "Choose an icon", "", "sam")
		return err
	})
	if got := jobTitles(t, s, cl.ID); !sameStrings(got, []string{"Get the API key", "Sign the contract", "Choose an icon"}) {
		t.Errorf("order after adding = %v", got)
	}
	if err := s.WithTx(ctx, func(tx pgx.Tx) error { _, err := AddJob(ctx, tx, cl.ID, " ", "", "sam"); return err }); !errors.Is(err, ErrJobTitleBlank) {
		t.Errorf("blank title: %v", err)
	}
	if n := len(auditRows(t, s, "job.added", cl.ID)); n != 3 {
		t.Errorf("job.added rows = %d, want 3", n)
	}

	// Editing: title and note, with the old values kept on the trail.
	mustTx(t, s, func(tx pgx.Tx) error {
		_, err := EditJob(ctx, tx, icon.ID, "Choose the app icon", "Two options from the designer.", "sam")
		return err
	})
	if j, _ := GetJob(ctx, s.Pool, icon.ID); j.Title != "Choose the app icon" || j.Note != "Two options from the designer." {
		t.Errorf("after edit: %+v", j)
	}
	if rows := auditRows(t, s, "job.edited", cl.ID); len(rows) != 1 || rows[0]["old_title"] != "Choose an icon" {
		t.Errorf("job.edited rows = %v", rows)
	}
	if err := s.WithTx(ctx, func(tx pgx.Tx) error { _, err := EditJob(ctx, tx, icon.ID, "", "", "sam"); return err }); !errors.Is(err, ErrJobTitleBlank) {
		t.Errorf("blank title on edit: %v", err)
	}

	// Moving: first, past the end, and 0 all keep positions dense.
	move := func(j *Job, place int) int {
		var final int
		mustTx(t, s, func(tx pgx.Tx) error {
			var err error
			final, err = PlaceJob(ctx, tx, j.ID, place, "sam")
			return err
		})
		return final
	}
	if got := move(icon, 1); got != 1 {
		t.Errorf("moved to %d, want 1", got)
	}
	if got := jobTitles(t, s, cl.ID); !sameStrings(got, []string{"Choose the app icon", "Get the API key", "Sign the contract"}) {
		t.Errorf("order after moving first = %v", got)
	}
	if got := move(icon, 99); got != 3 {
		t.Errorf("past the end moved to %d, want 3", got)
	}
	if got := move(contract, 0); got != 3 {
		t.Errorf("place 0 moved to %d, want 3 (the end)", got)
	}
	if got := jobTitles(t, s, cl.ID); !sameStrings(got, []string{"Get the API key", "Choose the app icon", "Sign the contract"}) {
		t.Errorf("order after moves = %v", got)
	}
	if n := len(auditRows(t, s, "job.moved", cl.ID)); n != 3 {
		t.Errorf("job.moved rows = %d, want 3", n)
	}

	// Ticking: who, how, and the words; a note replaces, a blank note keeps.
	tick := func(j *Job, on bool, note, via, quote, actor string) (*Job, error) {
		var out *Job
		err := s.WithTx(ctx, func(tx pgx.Tx) error {
			var err error
			out, err = TickJob(ctx, tx, j.ID, on, note, via, quote, actor)
			return err
		})
		return out, err
	}
	got, err := tick(key, true, "It is in the password manager.", "ui", "", "sam")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Ticked() || *got.TickedBy != "sam" || *got.TickedVia != "ui" || got.TickedQuote != nil || got.Note != "It is in the password manager." {
		t.Errorf("after a UI tick: %+v", got)
	}
	if _, err := tick(key, true, "", "ui", "", "sam"); !errors.Is(err, ErrJobAlreadyTicked) {
		t.Errorf("ticking twice: %v", err)
	}
	if _, err := tick(contract, false, "", "ui", "", "sam"); !errors.Is(err, ErrJobNotTicked) {
		t.Errorf("unticking an unticked job: %v", err)
	}
	if _, err := tick(contract, true, "", "fax", "", "sam"); err == nil {
		t.Error("an unknown surface was accepted")
	}
	// A tick through the chat agent always carries the person's words.
	if _, err := tick(contract, true, "", "mcp", "  ", "chat-agent"); !errors.Is(err, ErrRelayNeedsQuote) {
		t.Errorf("a relayed tick without words: %v", err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE jobs SET ticked_by = 'x', ticked_at = now(), ticked_via = 'mcp' WHERE id = $1`, contract.ID); err == nil {
		t.Error("the jobs table accepted a chat tick with no quote")
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE jobs SET ticked_via = 'ui' WHERE id = $1`, contract.ID); err == nil {
		t.Error("the jobs table accepted a surface on an unticked job")
	}
	got, err = tick(contract, true, "", "mcp", "I've signed the contract", "chat-agent")
	if err != nil {
		t.Fatal(err)
	}
	if *got.TickedVia != "mcp" || got.TickedQuote == nil || *got.TickedQuote != "I've signed the contract" || got.Note != "With the lawyers." {
		t.Errorf("after a relayed tick: %+v", got)
	}
	rows := auditRows(t, s, "job.ticked", cl.ID)
	if len(rows) != 2 || rows[1]["via"] != "mcp" || rows[1]["quote"] != "I've signed the contract" || rows[1]["_actor"] != "chat-agent" {
		t.Errorf("job.ticked rows = %v", rows)
	}
	got, err = tick(contract, false, "The lawyers found a mistake.", "ui", "", "sam")
	if err != nil {
		t.Fatal(err)
	}
	if got.Ticked() || got.TickedBy != nil || got.TickedVia != nil || got.TickedQuote != nil || got.Note != "The lawyers found a mistake." {
		t.Errorf("after unticking: %+v", got)
	}
	if rows := auditRows(t, s, "job.unticked", cl.ID); len(rows) != 1 || rows[0]["note"] != "The lawyers found a mistake." {
		t.Errorf("job.unticked rows = %v", rows)
	}

	// A ticked job's title can't change, so a tick never vouches for a job
	// nobody ticked; its note can (R14-2).
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := EditJob(ctx, tx, key.ID, "Something else", "", "sam")
		return err
	}); !errors.Is(err, ErrJobTickedRename) {
		t.Errorf("renaming a ticked job: %v", err)
	}
	mustTx(t, s, func(tx pgx.Tx) error {
		_, err := EditJob(ctx, tx, key.ID, "Get the API key", "Moved to the team vault.", "sam")
		return err
	})

	// Removing the last unticked job would finish the checklist unticked (R14-3).
	if fin, _ := WouldFinishChecklist(ctx, s.Pool, icon); fin {
		t.Error("removing one of two unticked jobs reported as finishing the checklist")
	}

	// Removing: the rest renumber; the trail keeps whether it was ticked.
	mustTx(t, s, func(tx pgx.Tx) error {
		_, err := RemoveJob(ctx, tx, key.ID, "Not needed after all.", "sam")
		return err
	})
	if got := jobTitles(t, s, cl.ID); !sameStrings(got, []string{"Choose the app icon", "Sign the contract"}) {
		t.Errorf("order after removing = %v", got)
	}
	if rows := auditRows(t, s, "job.removed", cl.ID); len(rows) != 1 || rows[0]["was_ticked"] != true || rows[0]["reason"] != "Not needed after all." {
		t.Errorf("job.removed rows = %v", rows)
	}
	if err := s.WithTx(ctx, func(tx pgx.Tx) error { _, err := RemoveJob(ctx, tx, key.ID, "", "sam"); return err }); !errors.Is(err, ErrNotFound) {
		t.Errorf("removing twice: %v", err)
	}

	// Titles resolve within a checklist; a shared title is ambiguous.
	if j, err := JobByTitle(ctx, s.Pool, cl.ID, "Sign the contract"); err != nil || j.ID != contract.ID {
		t.Errorf("JobByTitle = %v, %v", j, err)
	}
	mustTx(t, s, func(tx pgx.Tx) error { _, err := AddJob(ctx, tx, cl.ID, "Sign the contract", "", "sam"); return err })
	if _, err := JobByTitle(ctx, s.Pool, cl.ID, "Sign the contract"); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("a shared title was not reported as ambiguous: %v", err)
	}
}

// TestChecklistProgressAndShipping covers FR-2: when a checklist is done, how
// it counts in a milestone's items and not its tokens, how resolution treats
// initiatives and nested milestones, G4, and what marking as shipped records.
func TestChecklistProgressAndShipping(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	in, fA, _ := seedTree(t, s)
	if _, err := s.Pool.Exec(ctx, `UPDATE features SET state = 'done' WHERE id = $1`, fA); err != nil {
		t.Fatal(err)
	}

	var chores, empty, owned *Checklist
	var j1, j2 *Job
	var m, nested, solo uuid.UUID
	mustTx(t, s, func(tx pgx.Tx) error {
		var err error
		if chores, err = CreateChecklist(ctx, tx, "project", nil, "Chores", "", "sam"); err != nil {
			return err
		}
		if empty, err = CreateChecklist(ctx, tx, "project", nil, "Placeholder", "", "sam"); err != nil {
			return err
		}
		// Owned by the initiative, but never added to a milestone (SD-2).
		if owned, err = CreateChecklist(ctx, tx, "initiative", &in, "Auth chores", "", "sam"); err != nil {
			return err
		}
		if _, err = AddJob(ctx, tx, owned.ID, "Unrelated", "", "sam"); err != nil {
			return err
		}
		if j1, err = AddJob(ctx, tx, chores.ID, "Get the key", "", "sam"); err != nil {
			return err
		}
		if j2, err = AddJob(ctx, tx, chores.ID, "Sign it", "", "sam"); err != nil {
			return err
		}
		ms, err := CreateMilestone(ctx, tx, "project", nil, "Beta", "", nil, "sam")
		if err != nil {
			return err
		}
		m = ms.ID
		if err := AddMember(ctx, tx, m, "feature", fA, "sam"); err != nil {
			return err
		}
		return AddMember(ctx, tx, m, "checklist", chores.ID, "sam")
	})

	// An empty checklist is not done (SD-9); one with unticked jobs is not.
	if st, _ := GetChecklistStatus(ctx, s.Pool, empty.ID); st.Done() {
		t.Error("an empty checklist counts as done")
	}
	p, err := LiveProgress(ctx, s.Pool, m)
	if err != nil {
		t.Fatal(err)
	}
	if p.Total != 2 || p.Done != 1 || len(p.Leaves) != 1 || len(p.Checklists) != 1 {
		t.Errorf("with the checklist unticked: %+v, want 1 of 2 items, 1 feature leaf, 1 checklist", p)
	}
	// Tokens read Leaves, which stay the features alone.
	if feats, _ := ResolveMembers(ctx, s.Pool, m); len(feats) != 1 || feats[0] != fA {
		t.Errorf("ResolveMembers = %v, want the feature only", feats)
	}

	// An initiative member brings in features, not the checklists it owns.
	mustTx(t, s, func(tx pgx.Tx) error {
		ms, err := CreateMilestone(ctx, tx, "project", nil, "Whole auth", "", nil, "sam")
		if err != nil {
			return err
		}
		nested = ms.ID
		return AddMember(ctx, tx, nested, "initiative", in, "sam")
	})
	if lists, _ := ResolveChecklists(ctx, s.Pool, nested); len(lists) != 0 {
		t.Errorf("an initiative brought in its checklists: %v", lists)
	}
	// A nested milestone brings in its checklists.
	mustTx(t, s, func(tx pgx.Tx) error { return AddMember(ctx, tx, nested, "milestone", m, "sam") })
	if lists, _ := ResolveChecklists(ctx, s.Pool, nested); len(lists) != 1 || lists[0] != chores.ID {
		t.Errorf("nested checklists = %v, want Chores", lists)
	}

	// G4 on a milestone of one unticked checklist: refused in the one-item
	// sentence; with an empty milestone, the "comes down to" sentence.
	mustTx(t, s, func(tx pgx.Tx) error {
		ms, err := CreateMilestone(ctx, tx, "project", nil, "Paperwork", "", nil, "sam")
		if err != nil {
			return err
		}
		solo = ms.ID
		return AddMember(ctx, tx, solo, "checklist", chores.ID, "sam")
	})
	err = s.WithTx(ctx, func(tx pgx.Tx) error { _, err := LockMilestone(ctx, tx, solo, "sam"); return err })
	if err == nil || !strings.Contains(err.Error(), "the one item in it isn't done") {
		t.Errorf("shipping an unticked checklist alone: %v", err)
	}

	// Ship Beta with the checklist unticked: it is snapshotted and named as
	// not shipped (SD-4).
	mustTx(t, s, func(tx pgx.Tx) error { _, err := LockMilestone(ctx, tx, m, "sam"); return err })
	var snapTypes []string
	rows, _ := s.Pool.Query(ctx, `SELECT leaf_type FROM milestone_snapshots WHERE milestone_id = $1 ORDER BY leaf_type::text`, m)
	for rows.Next() {
		var lt string
		_ = rows.Scan(&lt)
		snapTypes = append(snapTypes, lt)
	}
	rows.Close()
	if !sameStrings(snapTypes, []string{"checklist", "feature"}) {
		t.Errorf("snapshot leaf types = %v", snapTypes)
	}
	locked := auditRows(t, s, "milestone.locked", m)
	if len(locked) != 1 {
		t.Fatalf("milestone.locked rows = %d", len(locked))
	}
	ns, _ := locked[0]["not_shipped"].([]any)
	if len(ns) != 1 || ns[0].(map[string]any)["type"] != "checklist" || ns[0].(map[string]any)["name"] != "Chores" {
		t.Errorf("not_shipped = %v, want the Chores checklist", locked[0]["not_shipped"])
	}
	if sp, _ := SnapshotProgress(ctx, s.Pool, m); sp.Total != 2 || sp.Done != 1 {
		t.Errorf("shipped progress = %+v, want 1 of 2", sp)
	}

	// Ticking every job afterwards moves the shipped milestone on, and makes
	// the lone-checklist milestone shippable (SD-3).
	for _, j := range []*Job{j1, j2} {
		mustTx(t, s, func(tx pgx.Tx) error { _, err := TickJob(ctx, tx, j.ID, true, "", "ui", "", "sam"); return err })
	}
	if sp, _ := SnapshotProgress(ctx, s.Pool, m); sp.Total != 2 || sp.Done != 2 {
		t.Errorf("shipped progress after ticking = %+v, want 2 of 2", sp)
	}
	mustTx(t, s, func(tx pgx.Tx) error { _, err := LockMilestone(ctx, tx, solo, "sam"); return err })

	// A checklist done when its milestone shipped stays done in the record,
	// even if a job is unticked or added later — as a feature's done is
	// terminal (SD-4, R14-1). Its own page shows it isn't done now.
	var late *Job
	mustTx(t, s, func(tx pgx.Tx) error {
		if _, err := TickJob(ctx, tx, j1.ID, false, "", "ui", "", "sam"); err != nil {
			return err
		}
		var err error
		late, err = AddJob(ctx, tx, chores.ID, "One more thing", "", "sam")
		return err
	})
	if sp, _ := SnapshotProgress(ctx, s.Pool, solo); sp.Total != 1 || sp.Done != 1 {
		t.Errorf("shipped-when-done progress after an untick = %+v, want 1 of 1", sp)
	}
	if st, _ := GetChecklistStatus(ctx, s.Pool, chores.ID); st.Done() {
		t.Error("the checklist itself still reads as done after an untick")
	}
	if fin, _ := WouldFinishChecklist(ctx, s.Pool, late); fin {
		t.Error("with two unticked jobs, removing one reported as finishing the checklist")
	}
	mustTx(t, s, func(tx pgx.Tx) error { _, err := TickJob(ctx, tx, j1.ID, true, "", "ui", "", "sam"); return err })
	if fin, _ := WouldFinishChecklist(ctx, s.Pool, late); !fin {
		t.Error("removing the last unticked job wasn't reported as finishing the checklist")
	}
	mustTx(t, s, func(tx pgx.Tx) error { _, err := RemoveJob(ctx, tx, late.ID, "", "sam"); return err })

	// Reopening keeps the snapshot's checklists on the trail.
	mustTx(t, s, func(tx pgx.Tx) error { return UnlockMilestone(ctx, tx, m, "one more chore", "sam") })
	un := auditRows(t, s, "milestone.unlocked", m)
	if lists, _ := un[0]["snapshot_checklists"].([]any); len(lists) != 1 || lists[0] != chores.ID.String() {
		t.Errorf("snapshot_checklists = %v", un[0]["snapshot_checklists"])
	}
	// A checklist can be taken out of a milestone like anything else.
	mustTx(t, s, func(tx pgx.Tx) error {
		return RemoveMember(ctx, tx, m, "checklist", chores.ID, "done elsewhere", "sam")
	})
	if p, _ := LiveProgress(ctx, s.Pool, m); p.Total != 1 || len(p.Checklists) != 0 {
		t.Errorf("after taking the checklist out: %+v", p)
	}
}

// TestChecklistCandidates covers FR-6.1: the picker offers checklists planned
// in its owner's subtree by default, anywhere when searching, and never one
// already in the milestone.
func TestChecklistCandidates(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	in, _, _ := seedTree(t, s)
	var m uuid.UUID
	var here, elsewhere *Checklist
	mustTx(t, s, func(tx pgx.Tx) error {
		ms, err := CreateMilestone(ctx, tx, "initiative", &in, "Auth beta", "", nil, "sam")
		if err != nil {
			return err
		}
		m = ms.ID
		if here, err = CreateChecklist(ctx, tx, "initiative", &in, "Auth chores", "", "sam"); err != nil {
			return err
		}
		elsewhere, err = CreateChecklist(ctx, tx, "project", nil, "Launch paperwork", "", "sam")
		return err
	})
	kinds := func(query string) map[uuid.UUID]string {
		cs, _, err := MemberCandidates(ctx, s.Pool, m, &in, query, 50)
		if err != nil {
			t.Fatal(err)
		}
		out := map[uuid.UUID]string{}
		for _, c := range cs {
			out[c.ID] = c.Kind
		}
		return out
	}
	got := kinds("")
	if got[here.ID] != "checklist" {
		t.Error("a checklist planned in the subtree isn't offered")
	}
	if _, ok := got[elsewhere.ID]; ok {
		t.Error("a checklist planned elsewhere is offered before searching")
	}
	if kinds("paperwork")[elsewhere.ID] != "checklist" {
		t.Error("searching doesn't reach a checklist planned elsewhere")
	}
	mustTx(t, s, func(tx pgx.Tx) error { return AddMember(ctx, tx, m, "checklist", here.ID, "sam") })
	if _, ok := kinds("")[here.ID]; ok {
		t.Error("a checklist already in the milestone is still offered")
	}
}
