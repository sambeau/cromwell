package server

// Unmeasured actuals over the API (SPEC-020 FR-7.2, FR-7.5).

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"subutai/internal/store"
)

func TestEstimateAPIReportsUnmeasured(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if code, _ := h.call("POST", "/api/initiatives", map[string]string{"slug": "auth", "name": "Auth"}); code != 201 {
		t.Fatal("initiative")
	}
	h.createFeature("auth", "login", "Login", "the login form")
	h.createFeature("auth", "logout", "Logout", "the logout link")
	h.call("POST", "/api/estimate/set", map[string]any{"ref": "auth/login", "tokens": 1000})
	h.call("POST", "/api/estimate/set", map[string]any{"ref": "auth/logout", "tokens": 1000})

	login, err := h.srv.featureByPath(ctx, "auth/login")
	if err != nil {
		t.Fatal(err)
	}
	logout, err := h.srv.featureByPath(ctx, "auth/logout")
	if err != nil {
		t.Fatal(err)
	}
	// Login has 1,200 measured tokens, and a task done in chat. Logout has
	// 1,500 measured tokens and nothing else.
	err = h.srv.Store.WithTx(ctx, func(tx pgx.Tx) error {
		task, err := store.CreateTask(ctx, tx, login.ID, 0, "T1", "form", "", "sam")
		if err != nil {
			return err
		}
		c, err := store.CreateClaim(ctx, tx, "task", task.ID, nil, "chat", "claude", "mcp", "")
		if err != nil {
			return err
		}
		if _, err := store.RecordClaimExecution(ctx, tx, "task", task.ID, c, "abc"); err != nil {
			return err
		}
		for key, d := range map[string]struct {
			tokens int64
			f      *store.Feature
		}{"login": {1200, login}, "logout": {1500, logout}} {
			disp, err := store.EnqueueDispatch(ctx, tx, "implement-task", "implementer", "m", "feature", d.f.ID, "un:"+key)
			if err != nil {
				return err
			}
			if _, err := store.MarkDispatchRunning(ctx, tx, disp.ID, nil); err != nil {
				return err
			}
			if err := store.MarkDispatchSucceeded(ctx, tx, disp.ID, store.TokenUsage{Input: d.tokens}, 0.01, map[string]string{}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	_, un := h.call("GET", "/api/estimate?ref=auth/login", nil)
	if v, present := un["actual_tokens"]; !present || v != nil {
		t.Errorf("unmeasured actual_tokens = %v (present %v), want null", v, present)
	}
	if v, present := un["delta"]; !present || v != nil {
		t.Errorf("unmeasured delta = %v (present %v), want null", v, present)
	}
	if un["unmeasured"] != true || un["measured_part"] != float64(1200) {
		t.Errorf("unmeasured = %v, measured_part = %v; want true, 1200", un["unmeasured"], un["measured_part"])
	}
	// The initiative includes the unmeasured feature.
	_, in := h.call("GET", "/api/estimate?ref=auth", nil)
	if in["unmeasured"] != true || in["actual_tokens"] != nil || in["measured_part"] != float64(2700) {
		t.Errorf("initiative = %v, want unmeasured with measured_part 2700", in)
	}

	_, ok := h.call("GET", "/api/estimate?ref=auth/logout", nil)
	if ok["actual_tokens"] != float64(1500) || ok["delta"] != float64(500) || ok["unmeasured"] != false {
		t.Errorf("measured feature = %v, want actual 1500, delta 500, unmeasured false", ok)
	}
	if _, has := ok["measured_part"]; has {
		t.Errorf("measured feature has measured_part %v", ok["measured_part"])
	}

	// Cost is unchanged, and the unmeasured feature adds its sentence (FR-7.5).
	_, cost := h.call("GET", "/api/cost/rollup?ref=auth/login", nil)
	if cost["cost_usd"] != 0.01 || cost["note"] != "It also includes work done in chat or by a person, which isn't measured." {
		t.Errorf("unmeasured cost = %v", cost)
	}
	_, cost = h.call("GET", "/api/cost/rollup?ref=auth/logout", nil)
	if _, has := cost["note"]; has {
		t.Errorf("measured cost has a note: %v", cost)
	}
}
