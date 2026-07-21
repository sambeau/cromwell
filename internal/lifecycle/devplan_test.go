package lifecycle

import (
	"strings"
	"testing"
)

const goodDevPlan = `---
title: Login form — dev plan
type: dev_plan
owner: auth/login
---

# Login form — dev plan

## Approach

Build the form, then the session layer, then the lockout counter.

## Tasks

| id | title | depends_on | description |
|----|-------|------------|-------------|
| T1 | Form markup | | The email/password form and client validation |
| T2 | Session layer | T1 | Cookie issuance and expiry |
| T3 | Lockout counter | T2 | Five-attempt lockout with reset |
`

func TestParseDevPlanTasks(t *testing.T) {
	rows, err := ParseDevPlanTasks(goodDevPlan)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	if rows[0].LocalID != "T1" || len(rows[0].DependsOn) != 0 {
		t.Errorf("T1 wrong: %+v", rows[0])
	}
	if rows[1].LocalID != "T2" || len(rows[1].DependsOn) != 1 || rows[1].DependsOn[0] != "T1" {
		t.Errorf("T2 wrong: %+v", rows[1])
	}
	if rows[2].Title != "Lockout counter" {
		t.Errorf("T3 title wrong: %+v", rows[2])
	}
}

func TestTaskTableDefects(t *testing.T) {
	cases := []struct {
		name, table, wantErr string
	}{
		{
			"missing column",
			"| id | title | depends_on |\n|----|----|----|\n| T1 | x | |\n",
			"description",
		},
		{
			"duplicate id",
			"| id | title | depends_on | description |\n|----|----|----|----|\n| T1 | a | | x |\n| T1 | b | | y |\n",
			"duplicate id",
		},
		{
			"dangling dep",
			"| id | title | depends_on | description |\n|----|----|----|----|\n| T1 | a | T9 | x |\n",
			"unknown id",
		},
		{
			"cycle",
			"| id | title | depends_on | description |\n|----|----|----|----|\n| T1 | a | T2 | x |\n| T2 | b | T1 | y |\n",
			"cycle",
		},
		{
			"self dep",
			"| id | title | depends_on | description |\n|----|----|----|----|\n| T1 | a | T1 | x |\n",
			"itself",
		},
		{
			"no rows",
			"| id | title | depends_on | description |\n|----|----|----|----|\n",
			"no rows",
		},
		{
			"no table",
			"just prose, no table here\n",
			"no task table",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseTaskTable(c.table)
			if err == nil {
				t.Fatalf("expected error for %s", c.name)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), c.wantErr)
			}
		})
	}
}

func TestCycleMessageNamesNodes(t *testing.T) {
	_, err := ParseTaskTable("| id | title | depends_on | description |\n|--|--|--|--|\n| T1 | a | T3 | x |\n| T2 | b | T1 | y |\n| T3 | c | T2 | z |\n")
	if err == nil || !strings.Contains(err.Error(), "→") {
		t.Errorf("cycle error should show the path: %v", err)
	}
}

func TestPlanDecompositionFirstTime(t *testing.T) {
	rows, _ := ParseDevPlanTasks(goodDevPlan)
	plan := PlanDecomposition(rows, nil)
	if len(plan.Creates) != 3 || len(plan.Updates) != 0 || len(plan.DeleteLocalIDs) != 0 {
		t.Errorf("first decomposition should be all creates: %+v", plan)
	}
}

func TestPlanDecompositionReconcile(t *testing.T) {
	rows, _ := ParseDevPlanTasks(goodDevPlan) // T1, T2, T3
	existing := []ExistingTask{
		{LocalID: "T1", State: TaskDone},    // done, dropped from new table below
		{LocalID: "T2", State: TaskReady},   // pending-ish, dropped → delete
		{LocalID: "T3", State: TaskPending}, // still present → update
	}
	// New table keeps only T3 and adds T4.
	newRows := []TaskRow{
		{Position: 0, LocalID: "T3", Title: "Lockout", DependsOn: nil},
		{Position: 1, LocalID: "T4", Title: "Audit log", DependsOn: []string{"T3"}},
	}
	_ = rows
	plan := PlanDecomposition(newRows, existing)

	if len(plan.Creates) != 1 || plan.Creates[0].LocalID != "T4" {
		t.Errorf("T4 should be created: %+v", plan.Creates)
	}
	if len(plan.Updates) != 1 || plan.Updates[0].LocalID != "T3" {
		t.Errorf("T3 should be updated: %+v", plan.Updates)
	}
	if len(plan.DeleteLocalIDs) != 1 || plan.DeleteLocalIDs[0] != "T2" {
		t.Errorf("T2 (ready, dropped) should be deleted: %+v", plan.DeleteLocalIDs)
	}
	if len(plan.KeptMismatches) != 1 || plan.KeptMismatches[0] != "T1" {
		t.Errorf("T1 (done, dropped) should be kept as a mismatch, never deleted: %+v", plan.KeptMismatches)
	}
}
