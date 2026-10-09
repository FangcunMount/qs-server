package evaluation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
)

func TestSQLHistoricalBatchCASRejectsForgedOrNilCapabilities(t *testing.T) {
	for _, b := range []*SQLHistoricalOwnerBatch{nil, {report: SQLHistoricalOwnerBatchReport{Complete: true}}} {
		if p, e := PrepareSQLHistoricalBatchCAS(context.Background(), b, []SQLHistoricalBatchAttachment{{}}); e == nil || p != nil {
			t.Fatal("caller-set complete authorized CAS")
		}
	}
	var p *SQLHistoricalBatchCASPlan
	if s, e := p.Apply(context.Background()); e == nil || s != nil {
		t.Fatal("nil plan accepted")
	}
	var statement *SQLHistoricalBatchCASStatement
	if e := statement.VerifyPersisted(context.Background(), nil); !errors.Is(e, ErrSQLHistoricalBatchCAS) {
		t.Fatal(e)
	}
	r := statement.Report()
	if r.StatementApplied || r.HostCommitVerified || r.BusinessClosureVerified || r.DropReady || !r.HostCommitRequired || !r.IndependentReadbackRequired {
		t.Fatal("missing capability upgraded to committed proof")
	}
	if _, e := json.Marshal(&SQLHistoricalBatchCASPlan{}); !errors.Is(e, ErrSQLHistoricalFactsSerialization) {
		t.Fatal("raw private baseline serialized")
	}
}

func TestSQLHistoricalBatchCASCloneAndOriginalRunSelectors(t *testing.T) {
	raw := "42"
	image := sqlHistoricalCASImage{rows: map[string][]historicalSQLRow{"assessment": {{"id": &raw}}}, schema: map[string]string{"assessment": "hash"}, columns: map[string][]string{"assessment": {"id"}}}
	image.rows["evaluation_outcome"] = nil
	clone := casCloneImage(image)
	if clone.rows["evaluation_outcome"] != nil {
		t.Fatal("empty query result baseline changed representation")
	}
	*clone.rows["assessment"][0]["id"] = "43"
	clone.columns["assessment"][0] = "unknown"
	if raw != "42" || image.columns["assessment"][0] != "id" {
		t.Fatal("private raw baseline shared")
	}
	v := sqlHistoricalCASImage{rows: map[string][]historicalSQLRow{"assessment": {{"id": evidence.String("42")}}, "runtime_checkpoint": {{"resource_id": evidence.String("42:1"), "scope": evidence.String("evaluation_run"), "assessment_id": evidence.String("42"), "attempt_no": evidence.String("1"), "deleted_at": nil}}}}
	a := SQLHistoricalBatchAttachment{AssessmentID: 42, Entry: evidence.HistoricalReferenceEntryV1{EventType: "evaluation.retry.requested", Run: &evidence.HistoricalRunReferenceV1{RunID: "42:2", Attempt: 2}}}
	if _, _, _, _, _, e := casTarget(v, a); e == nil {
		t.Fatal("latest run substituted for original Run")
	}
	a.Entry.Run = &evidence.HistoricalRunReferenceV1{RunID: "42:1", Attempt: 1}
	v.rows["runtime_checkpoint"] = append(v.rows["runtime_checkpoint"], v.rows["runtime_checkpoint"][0])
	if _, _, _, _, _, e := casTarget(v, a); e == nil {
		t.Fatal("duplicate original Run silently selected")
	}
}

func TestSQLHistoricalBatchCASTypedNilAndUnknownPoolDoNotExecute(t *testing.T) {
	var sqlTx *sql.Tx
	var prepared *gorm.PreparedStmtTX
	p := &SQLHistoricalBatchCASPlan{groups: []sqlHistoricalCASGroup{{table: "assessment"}}}
	for _, pool := range []gorm.ConnPool{sqlTx, prepared, &gorm.PreparedStmtTX{Tx: sqlTx}, &historicalUnknownPool{}} {
		g := &gorm.DB{Config: &gorm.Config{}, Statement: &gorm.Statement{ConnPool: pool}}
		if s, e := p.Apply(hostmysql.WithTx(t.Context(), g)); e == nil || s != nil {
			t.Fatal("unknown/typed-nil pool accepted")
		}
	}
}

func missingOutcomeRunCASFixture() (sqlHistoricalCASImage, SQLHistoricalBatchAttachment) {
	v := sqlHistoricalCASImage{rows: map[string][]historicalSQLRow{
		"assessment":         {{"id": evidence.String("42"), "org_id": evidence.String("7"), "testee_id": evidence.String("21"), "status": evidence.String("evaluated")}},
		"evaluation_outcome": {{"id": evidence.String("9001"), "assessment_id": evidence.String("42"), "org_id": evidence.String("7"), "testee_id": evidence.String("21"), "evaluation_run_id": evidence.String("original-run")}},
	}}
	a := SQLHistoricalBatchAttachment{AssessmentID: 42, OutcomeID: 9001, Entry: evidence.HistoricalReferenceEntryV1{EventType: "evaluation.outcome.committed", Proof: &evidence.EventEvidenceV1{Class: evidence.Unverifiable, Verification: evidence.Verification{Reason: "original_outcome_run_absent"}}}}
	return v, a
}

func TestSQLHistoricalBatchCASNilOutcomeRunOnlyExactUnverifiableGap(t *testing.T) {
	v, a := missingOutcomeRunCASFixture()
	if table, _, id, _, run, err := casTarget(v, a); err != nil || table != "evaluation_outcome" || id != 9001 || run != nil {
		t.Fatal("explicit absent Run gap rejected or invented a Run", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*sqlHistoricalCASImage, *SQLHistoricalBatchAttachment)
	}{
		{"verified zero attempt", func(_ *sqlHistoricalCASImage, a *SQLHistoricalBatchAttachment) {
			a.Entry.Proof.Class = evidence.RetiredVerified
		}},
		{"missing proof", func(_ *sqlHistoricalCASImage, a *SQLHistoricalBatchAttachment) { a.Entry.Proof = nil }},
		{"similar reason", func(_ *sqlHistoricalCASImage, a *SQLHistoricalBatchAttachment) {
			a.Entry.Proof.Verification.Reason = "not_original_outcome_run_absent"
		}},
		{"composite reason", func(_ *sqlHistoricalCASImage, a *SQLHistoricalBatchAttachment) {
			a.Entry.Proof.Verification.Reason += ",other_gap"
		}},
		{"owner unfinished", func(v *sqlHistoricalCASImage, _ *SQLHistoricalBatchAttachment) {
			v.rows["assessment"][0]["status"] = evidence.String("submitted")
		}},
		{"owner org", func(v *sqlHistoricalCASImage, _ *SQLHistoricalBatchAttachment) {
			v.rows["evaluation_outcome"][0]["org_id"] = evidence.String("8")
		}},
		{"retained cross owner scope deleted", func(v *sqlHistoricalCASImage, _ *SQLHistoricalBatchAttachment) {
			v.rows["runtime_checkpoint"] = []historicalSQLRow{{"resource_id": evidence.String("original-run"), "assessment_id": evidence.String("43"), "scope": evidence.String("other_scope"), "deleted_at": evidence.String("2026-10-09")}}
		}},
		{"unretained declared entry", func(_ *sqlHistoricalCASImage, a *SQLHistoricalBatchAttachment) {
			a.Entry.Run = &evidence.HistoricalRunReferenceV1{RunID: "original-run", Attempt: 1}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, a := missingOutcomeRunCASFixture()
			tc.mutate(&v, &a)
			if _, _, _, _, _, err := casTarget(v, a); err == nil {
				t.Fatal("nil Run exception relaxed actual identity/proof constraint")
			}
		})
	}
	v, a = missingOutcomeRunCASFixture()
	a.Entry.Proof = nil
	if _, _, _, _, run, err := casTargetForBinding(v, a, true); err != nil || run != nil {
		t.Fatal("digest-only owner binding unavailable", err)
	}
	if _, _, _, _, _, err := casTarget(v, a); err == nil {
		t.Fatal("digest-only reader authorized CAS without classified proof")
	}
}
