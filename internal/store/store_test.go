package store

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"subutai/internal/content"
	"subutai/internal/lifecycle"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	conn := freshConn(t)
	ctx := context.Background()
	if err := Migrate(ctx, conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_ = conn.Close(ctx)
	pool, err := pgxpool.New(ctx, testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return &Store{Pool: pool}
}

func TestEntityDocumentDispatchFlow(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	var featID = struct{ f *Feature }{}
	var doc *Document

	// Initiative → feature → registered spec, all audited.
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		in, err := CreateInitiative(ctx, tx, nil, "auth", "Authentication", "", "sam")
		if err != nil {
			return err
		}
		f, err := CreateFeature(ctx, tx, in.ID, "login", "Login form", "", "sam")
		if err != nil {
			return err
		}
		featID.f = f
		ownerID := f.ID
		doc, err = RegisterDocument(ctx, tx, "spec", "feature", &ownerID,
			"docs/specs/login.md", "Login form", "hash-v1", nil, "sam")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// G5 sees one non-terminal feature in the subtree.
	n, err := NonTerminalFeatureCount(ctx, s.Pool, featID.f.InitiativeID)
	if err != nil || n != 1 {
		t.Fatalf("non-terminal count = %d, %v; want 1", n, err)
	}

	// Submit: draft → reviewing, then enqueue the review dispatch.
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := TransitionDocument(ctx, tx, doc, lifecycle.DocSubmit, "sam", nil); err != nil {
			return err
		}
		d, err := EnqueueDispatch(ctx, tx, "review-spec", "spec-reviewer", "claude-sonnet-5",
			"document", doc.ID, "review:"+doc.ID.String()+":hash-v1")
		if err != nil {
			return err
		}
		if d == nil {
			t.Error("first enqueue should insert")
		}
		// Replaying the same key inside a live window is a no-op (FR-8.1).
		d2, err := EnqueueDispatch(ctx, tx, "review-spec", "spec-reviewer", "claude-sonnet-5",
			"document", doc.ID, "review:"+doc.ID.String()+":hash-v1")
		if err != nil {
			return err
		}
		if d2 != nil {
			t.Error("duplicate enqueue should be idempotent")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Claim → succeed with usage and cost.
	queued, err := s.QueuedDispatches(ctx)
	if err != nil || len(queued) != 1 {
		t.Fatalf("queued = %d, %v", len(queued), err)
	}
	dispatchID := queued[0].ID
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		ok, err := MarkDispatchRunning(ctx, tx, dispatchID, map[string]float64{"input": 3.0})
		if err != nil || !ok {
			t.Fatalf("claim failed: %v %v", ok, err)
		}
		return MarkDispatchSucceeded(ctx, tx, dispatchID,
			TokenUsage{Input: 1000, Output: 200}, 0.006,
			map[string]string{"verdict": "approve"})
	})
	if err != nil {
		t.Fatal(err)
	}

	// Cost rollup reflects the dispatch.
	rollup, err := s.CostRollup(ctx)
	if err != nil || len(rollup) != 1 || rollup[0].CostUSD != 0.006 {
		t.Fatalf("rollup: %+v, %v", rollup, err)
	}

	// Approve: reviewing → approved, then the feature's G1 fires.
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := TransitionDocument(ctx, tx, doc, lifecycle.DocApprove, "spec-reviewer", nil); err != nil {
			return err
		}
		g := lifecycle.G1(true, false, false)
		if err := Audit(ctx, tx, "orchestrator", "gate.evaluated", "feature", &featID.f.ID,
			map[string]any{"gate": string(g.Gate), "pass": g.Pass, "reason": g.Reason}); err != nil {
			return err
		}
		return TransitionFeature(ctx, tx, featID.f, lifecycle.FeatContractApproved, "orchestrator", nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	f2, _ := GetFeature(ctx, s.Pool, featID.f.ID)
	if f2.State != lifecycle.FeatReady {
		t.Errorf("feature state = %s, want ready", f2.State)
	}

	// The audit trail has the full expected sequence, no gaps (FR-7.1 shape).
	events, err := s.AuditTail(ctx, "", nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, e := range events {
		kinds = append(kinds, e.Kind)
	}
	want := []string{"initiative.created", "feature.created", "document.registered",
		"document.transition", "dispatch.queued", "dispatch.running",
		"dispatch.succeeded", "document.transition", "gate.evaluated", "feature.transition"}
	if len(kinds) != len(want) {
		t.Fatalf("audit kinds = %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Errorf("audit[%d] = %s, want %s", i, kinds[i], want[i])
		}
	}
}

func TestSectionsAndSearch(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	var doc *Document
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		doc, err = RegisterDocument(ctx, tx, "note", "project", nil,
			"docs/notes/sessions.md", "Session handling", "h1", nil, "sam")
		if err != nil {
			return err
		}
		return ReplaceSections(ctx, tx, doc.ID, "h1", []content.Section{
			{Position: 0, Heading: "Session handling", Level: 1, Content: "Sessions expire after thirty minutes of inactivity."},
			{Position: 1, Heading: "Storage", Level: 2, Content: "Tokens are stored in an http-only cookie."},
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	hits, err := s.SearchSections(ctx, "cookie", 10)
	if err != nil || len(hits) != 1 || hits[0].Heading != "Storage" {
		t.Fatalf("search: %+v, %v", hits, err)
	}

	secs, err := Sections(ctx, s.Pool, doc.ID)
	if err != nil || len(secs) != 2 {
		t.Fatalf("sections: %+v, %v", secs, err)
	}
}

func TestCheckpointIdempotencyAndResponse(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	var doc *Document
	_ = s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		doc, err = RegisterDocument(ctx, tx, "spec", "project", nil, "d.md", "D", "h", nil, "sam")
		return err
	})

	// Create twice: second is a no-op (FR-6.2).
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		cp, err := CreateCheckpoint(ctx, tx, "review-escalation", "document", doc.ID, "Approve?", map[string]any{"reasoning": "unsure"})
		if err != nil || cp == nil {
			t.Fatalf("first create: %v %v", cp, err)
		}
		dup, err := CreateCheckpoint(ctx, tx, "review-escalation", "document", doc.ID, "Approve?", nil)
		if err != nil {
			return err
		}
		if dup != nil {
			t.Error("second pending create should be idempotent")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	pending, err := s.PendingCheckpoints(ctx)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending: %d, %v", len(pending), err)
	}

	// Respond, then a new checkpoint of the same kind may be created again.
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		cp, err := RespondCheckpoint(ctx, tx, pending[0].ID, map[string]any{"decision": "approve"}, "sam")
		if err != nil {
			return err
		}
		if cp.State != "answered" || *cp.RespondedBy != "sam" {
			t.Errorf("respond: %+v", cp)
		}
		again, err := CreateCheckpoint(ctx, tx, "review-escalation", "document", doc.ID, "Approve again?", nil)
		if err != nil || again == nil {
			t.Errorf("after answer, a new pending checkpoint should insert: %v %v", again, err)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRequeueAndStallDetection(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	var doc *Document
	var dispatchID = struct{ id *Dispatch }{}
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		doc, err = RegisterDocument(ctx, tx, "spec", "project", nil, "e.md", "E", "h", nil, "sam")
		if err != nil {
			return err
		}
		dispatchID.id, err = EnqueueDispatch(ctx, tx, "review-spec", "r", "m", "document", doc.ID, "k1")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	id := dispatchID.id.ID

	// running with an old heartbeat → stalled
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := MarkDispatchRunning(ctx, tx, id, nil); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE dispatches SET heartbeat_at = now() - interval '10 minutes' WHERE id = $1`, id)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	stalled, err := s.StalledRunning(ctx, 2*60*1e9) // 2m
	if err != nil || len(stalled) != 1 {
		t.Fatalf("stalled: %d, %v", len(stalled), err)
	}

	// fail → requeue increments attempt
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := MarkDispatchFailed(ctx, tx, id, "stalled"); err != nil {
			return err
		}
		return RequeueDispatch(ctx, tx, id)
	})
	if err != nil {
		t.Fatal(err)
	}
	d, _ := GetDispatch(ctx, s.Pool, id)
	if d.State != "queued" || d.Attempt != 2 {
		t.Errorf("requeued dispatch: state=%s attempt=%d", d.State, d.Attempt)
	}
}
