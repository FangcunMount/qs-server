package evaluation

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
)

func stableAssessmentFixture() historicalSQLRow {
	return historicalSQLRow{"id": evidence.String("42"), "org_id": evidence.String("7"), "testee_id": evidence.String("21"), "answer_sheet_id": evidence.String("1042"), "questionnaire_code": evidence.String("Q"), "questionnaire_version": evidence.String("1"), "origin_type": evidence.String("adhoc"), "origin_id": nil, "conducting_context": nil, "evaluation_model_kind": evidence.String("scale"), "evaluation_model_code": evidence.String("S"), "evaluation_model_version": nil, "version": evidence.String("1"), "status": evidence.String("failed"), "updated_at": evidence.String("original")}
}

func cloneHistoricalRow(row historicalSQLRow) historicalSQLRow {
	copy := historicalSQLRow{}
	for key, value := range row {
		copy[key] = value
	}
	return copy
}

func TestHistoricalAnchorSeparatesStableIdentityFromCASFacts(t *testing.T) {
	base := stableAssessmentFixture()
	want, err := historicalStableBinding("server", "database", "assessment", "evaluation.requested", base, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"version", "updated_at", "status", "future_column", "evaluation_model_version"} {
		changed := cloneHistoricalRow(base)
		changed[name] = evidence.String("new")
		if name == "evaluation_model_version" {
			// Domain reconstruction maps an absent optional version to empty.
			changed[name] = evidence.String("")
		}
		got, err := historicalStableBinding("server", "database", "assessment", "evaluation.requested", changed, nil)
		if err != nil || got != want || historicalSameFacts(base, changed, "history") {
			t.Fatal("stable anchor and exact CAS conflated", name, err)
		}
	}
	for _, name := range []string{"id", "org_id", "testee_id", "answer_sheet_id", "questionnaire_code", "questionnaire_version", "evaluation_model_code", "origin_id"} {
		changed := cloneHistoricalRow(base)
		changed[name] = evidence.String("43")
		got, err := historicalStableBinding("server", "database", "assessment", "evaluation.requested", changed, nil)
		if err == nil && got == want {
			t.Fatal("different original owner accepted", name)
		}
	}
	run := historicalSQLRow{"scope": evidence.String("evaluation_run"), "resource_id": evidence.String("42:1"), "assessment_id": evidence.String("42"), "attempt_no": evidence.String("1")}
	withRun, err := historicalStableBinding("server", "database", "assessment", "evaluation.failed", base, run)
	if err != nil {
		t.Fatal(err)
	}
	run["attempt_no"] = evidence.String("2")
	otherRun, err := historicalStableBinding("server", "database", "assessment", "evaluation.failed", base, run)
	if err != nil || otherRun == withRun {
		t.Fatal("original attempt replaced", err)
	}
	run["scope"] = evidence.String("EVALUATION_RUN")
	if _, err := historicalStableBinding("server", "database", "assessment", "evaluation.failed", base, run); err == nil {
		t.Fatal("casefold run accepted")
	}
	otherDB, err := historicalStableBinding("server", "another-database", "assessment", "evaluation.requested", base, nil)
	if err != nil || otherDB == want {
		t.Fatal("database identity not bound", err)
	}
}

func TestHistoricalAnchorTimeUsesOriginalMilliseconds(t *testing.T) {
	a, err := historicalMillisecond("2026-10-08T01:02:03.456Z")
	if err != nil {
		t.Fatal(err)
	}
	b, err := historicalMillisecond("2026-10-08 01:02:03.456")
	if err != nil || a != b {
		t.Fatal("driver date representations disagree", err)
	}
	if _, err := historicalMillisecond("2026-10-08T01:02:03.456001Z"); err == nil {
		t.Fatal("sub-millisecond original fact rounded")
	}
}
