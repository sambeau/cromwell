package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// What is unmeasured (SPEC-020 SD-13, FR-7.1). Tokens don't describe the chat
// agent's or a person's work, so an entity that includes any is "unmeasured":
// its dispatch tokens are shown as "at least", and calibration leaves it out.
// The definition lives here once, as SQL fragments, so that ActualTokens, the
// corpus and the calibration list agree.

// unmeasuredTaskSQL is an EXISTS expression, true when the task whose id is
// idExpr has an execution that isn't measured.
func unmeasuredTaskSQL(idExpr string) string {
	return `EXISTS (SELECT 1 FROM executions ux
		WHERE ux.ref_type = 'task' AND ux.ref_id = ` + idExpr + ` AND NOT ux.measured)`
}

// unmeasuredAuthoringSQL is an EXISTS expression, true when the latest
// writing act (wrote, revised or added) on the feature's live spec or dev plan
// was by the chat agent or a person (SD-13). idExpr and kindExpr name the
// feature's id and kind. A bug's spec is its bug report (lifecycle.IsSpecType).
func unmeasuredAuthoringSQL(idExpr, kindExpr string) string {
	return `EXISTS (SELECT 1 FROM documents ud
		WHERE ud.owner_type = 'feature' AND ud.owner_id = ` + idExpr + `
		  AND ud.state <> 'superseded'
		  AND ud.type::text IN (CASE WHEN ` + kindExpr + ` = 'bug' THEN 'bug_report' ELSE 'spec' END, 'dev_plan')
		  AND (SELECT uw.writer_kind FROM document_writers uw
		       WHERE uw.document_id = ud.id AND uw.act IN ('wrote', 'revised', 'added')
		       ORDER BY uw.at DESC, uw.id DESC LIMIT 1) IN ('chat', 'person'))`
}

// unmeasuredFeatureSQL is a boolean expression, true when the feature (named
// by idExpr and kindExpr) is unmeasured: it or any of its tasks has an
// unmeasured execution, or its spec or plan was last written in chat or by a
// person.
func unmeasuredFeatureSQL(idExpr, kindExpr string) string {
	return `(EXISTS (SELECT 1 FROM executions fx
			WHERE fx.ref_type = 'feature' AND fx.ref_id = ` + idExpr + ` AND NOT fx.measured)
		OR EXISTS (SELECT 1 FROM tasks ut WHERE ut.feature_id = ` + idExpr + ` AND ` + unmeasuredTaskSQL("ut.id") + `)
		OR ` + unmeasuredAuthoringSQL(idExpr, kindExpr) + `)`
}

// Unmeasured reports whether a task, feature or initiative includes work done
// in chat or by a person (SD-13). An initiative is unmeasured when any
// feature in its subtree is.
func Unmeasured(ctx context.Context, q Querier, refType string, refID uuid.UUID) (bool, error) {
	var b bool
	var err error
	switch refType {
	case "task":
		err = q.QueryRow(ctx, `SELECT `+unmeasuredTaskSQL("$1::uuid"), refID).Scan(&b)
	case "feature":
		err = q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM features f
			WHERE f.id = $1 AND `+unmeasuredFeatureSQL("f.id", "f.kind")+`)`, refID).Scan(&b)
	case "initiative":
		err = q.QueryRow(ctx, `
			WITH RECURSIVE subtree AS (
				SELECT id FROM initiatives WHERE id = $1
				UNION ALL
				SELECT i.id FROM initiatives i JOIN subtree s ON i.parent_id = s.id
			)
			SELECT EXISTS (SELECT 1 FROM features f
				WHERE f.initiative_id IN (SELECT id FROM subtree) AND `+unmeasuredFeatureSQL("f.id", "f.kind")+`)`,
			refID).Scan(&b)
	default:
		return false, errors.New("unmeasured only defined for task, feature, initiative")
	}
	return b, err
}

// UnmeasuredCostNote is the sentence a cost line adds for an unmeasured
// feature or initiative (SPEC-020 FR-7.5). Cost itself is unchanged: it sums
// dispatches, which are real money.
const UnmeasuredCostNote = "It also includes work done in chat or by a person, which isn't measured."
