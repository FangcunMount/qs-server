package retirement

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
)

// These are the private entry-format tests, NOT fabricated opaque authority or
// a positive production/native qualification. The real public factory's proof
// rejection tests below keep these helper inputs outside its capability path.
func qualifiedCASEntryFixture(eventType string) qualifiedCASRow {
	engine, pk, kind := "mysql", "mysql_uint64", SQLRowDigestKind
	if eventType == "answersheet.submitted" || eventType == "interpretation.report.generated" {
		engine, pk, kind = "mongodb", "mongodb_objectid", MongoRowDigestKind
	}
	facts := &DecodedSourceEvent{Source: evidence.HistoricalSourceReferenceV1{Database: engine, Object: "domain_event_outbox", PrimaryKeyKind: pk, PrimaryKeySHA256: strings.Repeat("1", 64), Digest: evidence.Digest{Kind: kind, SHA256: strings.Repeat("2", 64)}}, EventID: "owned-" + eventType, EventType: eventType, OrgID: 7, ContentDigest: evidence.Digest{Kind: ContentDigestKind, SHA256: strings.Repeat("3", 64)}, BusinessIDs: map[string]string{"assessment_id": "42", "outcome_id": "9001", "answersheet_id": "73", "generation_id": "84"}}
	candidate := coordinatorEventCandidate(facts)
	candidate.LocalQualified = true
	return qualifiedCASRow{facts: facts, candidate: candidate, bindingSHA: strings.Repeat("4", 64)}
}

func TestQualifiedCASEntrySixTypesRetainExactDigestsAndOriginalIdentity(t *testing.T) {
	clock := time.Date(2026, 10, 9, 1, 2, 3, 123456789, time.UTC)
	binding := HistoricalCoordinatorBinding{SourceSHA: strings.Repeat("a", 40), OperationID: "123-1"}
	for _, eventType := range []string{"evaluation.requested", "evaluation.retry.requested", "evaluation.failed", "evaluation.outcome.committed", "answersheet.submitted", "interpretation.report.generated"} {
		t.Run(eventType, func(t *testing.T) {
			row := qualifiedCASEntryFixture(eventType)
			if eventType == "evaluation.outcome.committed" || eventType == "interpretation.report.generated" {
				row.facts.OriginalRun.RunID = "42:1"
				if eventType == "interpretation.report.generated" {
					attempt := uint32(1)
					row.facts.OriginalRun.Attempt = &attempt
				}
				row.candidate.ActualOriginalRun = &evidence.HistoricalRunReferenceV1{RunID: "42:1", Attempt: 1}
			}
			entry, err := qualifiedCASEntry(binding, row, clock)
			if err != nil || entry.Validate() != nil || entry.EventID != row.facts.EventID || entry.Source != row.facts.Source || entry.Proof.Digest != row.facts.Source.Digest || entry.Proof.Digest == row.facts.ContentDigest || entry.Proof.BusinessBindingSHA256 != row.bindingSHA || entry.Proof.Reference != nil || entry.Proof.Verification.OperationID != binding.OperationID || !entry.Proof.Verification.VerifiedAt.Equal(clock.Truncate(time.Millisecond)) {
				t.Fatal("entry lost source/content/business digest separation")
			}
			if _, _, err := qualifiedCASOwnerIDs(row.facts, row.candidate); err != nil {
				t.Fatal(err)
			}
			if row.candidate.ActualOriginalRun != nil {
				entry.Run.RunID = "changed-copy"
				if row.candidate.ActualOriginalRun.RunID != "42:1" {
					t.Fatal("original Run alias escaped")
				}
			}
		})
	}
}

func TestQualifiedCASEntryHistoricalGapAndMissingRunPolicy(t *testing.T) {
	clock := time.Date(2026, 10, 9, 1, 2, 3, 0, time.UTC)
	binding := HistoricalCoordinatorBinding{SourceSHA: strings.Repeat("a", 40), OperationID: "123-1"}
	t.Run("requested_does_not_invent_executed_run", func(t *testing.T) {
		row := qualifiedCASEntryFixture("evaluation.requested")
		row.candidate.OriginalRunMissing = []string{"original_run_id", "original_attempt"}
		row.candidate.HistoricalGaps = []string{"storage_precision_gap", "missing_original_run_identity", "storage_precision_gap"}
		before := append([]string(nil), row.candidate.HistoricalGaps...)
		entry, err := qualifiedCASEntry(binding, row, clock)
		if err != nil || entry.Run != nil || entry.Proof.Class != evidence.Unverifiable || entry.Proof.Verification.Reason != "missing_original_run_identity;storage_precision_gap" || !reflect.DeepEqual(before, row.candidate.HistoricalGaps) {
			t.Fatal("missing original Run/precision policy changed")
		}
	})
	t.Run("outcome_absent_run_exact_audit_reason", func(t *testing.T) {
		row := qualifiedCASEntryFixture("evaluation.outcome.committed")
		row.facts.OriginalRun.RunID = "42:1"
		row.candidate.HistoricalGaps = []string{"original_outcome_run_absent", "storage_precision_gap"}
		entry, err := qualifiedCASEntry(binding, row, clock)
		if err != nil || entry.Run != nil || entry.Proof.Class != evidence.Unverifiable || entry.Proof.Verification.Reason != "original_outcome_run_absent" {
			t.Fatal("absent Outcome Run exact contract changed")
		}
	})
	t.Run("undeclared_gap_does_not_authorize_absent_run", func(t *testing.T) {
		row := qualifiedCASEntryFixture("evaluation.outcome.committed")
		row.facts.OriginalRun.RunID = "42:1"
		if _, err := qualifiedCASEntry(binding, row, clock); !errors.Is(err, ErrCoordinatorCASQualification) {
			t.Fatal("missing native absence qualification accepted")
		}
	})
}

func TestQualifiedCASEntryRejectsUnknownAndIdentityConflicts(t *testing.T) {
	clock := time.Date(2026, 10, 9, 1, 2, 3, 0, time.UTC)
	binding := HistoricalCoordinatorBinding{SourceSHA: strings.Repeat("a", 40), OperationID: "123-1"}
	cases := []struct {
		name   string
		change func(*qualifiedCASRow)
	}{
		{"scoped_business_is_not_whole_source_evidence", func(r *qualifiedCASRow) { r.sourceObservation = &HistoricalComponentSourceObservation{} }},
		{"current_coverage_gap_is_not_historical", func(r *qualifiedCASRow) {
			r.candidate.HistoricalGaps = []string{"inbox_and_global_unbound_coverage_require_actual_runtime_coordinator"}
		}},
		{"external_responsibility_is_not_historical", func(r *qualifiedCASRow) {
			r.candidate.HistoricalGaps = []string{"independent_admission_unique_sql_absence_observed_external_coverage_required"}
		}},
		{"original_run_invented", func(r *qualifiedCASRow) {
			r.candidate.ActualOriginalRun = &evidence.HistoricalRunReferenceV1{RunID: "42:1", Attempt: 1}
		}},
		{"unknown_gap", func(r *qualifiedCASRow) { r.candidate.HistoricalGaps = []string{"unknown_execution"} }},
		{"pending", func(r *qualifiedCASRow) { r.candidate.BlockingReasons = []string{"current_pending"} }},
		{"terminal_dto_false", func(r *qualifiedCASRow) { r.candidate.LocalQualified = false }},
		{"cross_org", func(r *qualifiedCASRow) { r.candidate.OrganizationID = "8" }},
		{"raw_source_conflict", func(r *qualifiedCASRow) { r.candidate.Source.Digest.SHA256 = strings.Repeat("5", 64) }},
		{"legacy_content_conflict", func(r *qualifiedCASRow) { r.candidate.ContentDigest.SHA256 = strings.Repeat("5", 64) }},
		{"wrong_owner", func(r *qualifiedCASRow) { r.candidate.OwnerID = "43" }},
		{"binding_absent", func(r *qualifiedCASRow) { r.bindingSHA = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := qualifiedCASEntryFixture("evaluation.requested")
			tc.change(&row)
			if _, err := qualifiedCASEntry(binding, row, clock); !errors.Is(err, ErrCoordinatorCASQualification) {
				t.Fatal("unqualified entry accepted")
			}
		})
	}
}

func TestQualifiedCASPublicFactoryCannotMintFromZeroOpaqueHandles(t *testing.T) {
	for _, owner := range []*HistoricalCoordinator{nil, {}} {
		for _, proof := range []*AIReverseFreshProof{nil, {}} {
			sqlPlan, mongoPlan, sealed, err := owner.PrepareQualifiedHistoricalCAS(context.Background(), &WholeSourceJointPage{}, &SourceOriginRecheckProof{}, proof)
			if sqlPlan != nil || mongoPlan != nil || sealed != nil || !errors.Is(err, ErrCoordinatorCASQualification) {
				t.Fatal("zero/private-report constructor minted plans")
			}
		}
	}
	for _, observed := range []*HistoricalComponentObservation{nil, {}, {nativeRW: true}} {
		for _, external := range []*AIExternalExecutionQualification{nil, {}} {
			for _, persisted := range []*AICommandPersistenceBatch{nil, {}} {
				sql, mongo, refs, err := ApplyQualifiedHistoricalComponent(t.Context(), observed, external, persisted)
				if len(sql) != 0 || mongo != nil || refs != 0 || !errors.Is(err, ErrCoordinatorCASQualification) {
					t.Fatal("zero handles or asserted native mode minted component effects")
				}
			}
		}
	}
}

func TestQualifiedCASBatchBindingsHaveNoCallerFlagConstructor(t *testing.T) {
	for _, b := range []*sqlevaluation.SQLHistoricalOwnerBatch{nil, {}} {
		for _, selections := range [][]sqlevaluation.SQLHistoricalBatchAttachment{nil, {{}}, make([]sqlevaluation.SQLHistoricalBatchAttachment, 513)} {
			result, err := sqlevaluation.SQLHistoricalBatchBindings(context.Background(), b, selections)
			if result != nil || err == nil {
				t.Fatal("no real opaque batch minted a binding")
			}
		}
	}
}

func TestQualifiedCASGlobalUnknownIsNotHistoricalGap(t *testing.T) {
	base := MongoResponsibilityCycleReport{Complete: true, LocalGraphChecked: true, Rows: 1, ClassifiedRows: 1, ClassCounts: map[string]uint64{"current_owner": 1}}
	if !qualifiedCASMongoGlobalKnown(base) {
		t.Fatal("complete ordinary observed catalog rejected")
	}
	for _, gap := range []string{"unknown_catalog_namespace_coverage_required", "unknown_standard_event_type_global_coverage_required", "catalog_repair_plan_execution_coverage_required"} {
		changed := base
		changed.CoverageGaps = []string{gap}
		if qualifiedCASMongoGlobalKnown(changed) {
			t.Fatal("unknown current catalog/operation became history", gap)
		}
	}
	for _, class := range []string{"unsupported_standard_message_type", "maintenance_repair_authorization_unknown"} {
		changed := base
		changed.ClassCounts = map[string]uint64{class: 1}
		if qualifiedCASMongoGlobalKnown(changed) {
			t.Fatal("unknown current responsibility class accepted", class)
		}
	}
	for _, change := range []func(*MongoResponsibilityCycleReport){func(r *MongoResponsibilityCycleReport) { r.Complete = false }, func(r *MongoResponsibilityCycleReport) { r.LocalGraphChecked = false }, func(r *MongoResponsibilityCycleReport) { r.ClassifiedRows = 0 }, func(r *MongoResponsibilityCycleReport) { r.BlockingReasons = []string{"orphan"} }} {
		changed := base
		change(&changed)
		if qualifiedCASMongoGlobalKnown(changed) {
			t.Fatal("partial/orphan global observation accepted")
		}
	}
}

func TestFreshComponentEntryRetainsGapRulesAndDoesNotClaimOldWholeMethod(t *testing.T) {
	row := qualifiedCASEntryFixture("evaluation.requested")
	row.sourceObservation = &HistoricalComponentSourceObservation{}
	row.candidate.HistoricalGaps = []string{"storage_precision_gap"}
	binding := HistoricalCoordinatorBinding{SourceSHA: strings.Repeat("a", 40), OperationID: "123-1"}
	at := time.Date(2026, 10, 10, 1, 2, 3, 123456789, time.UTC)
	if _, err := qualifiedCASEntry(binding, row, at); err == nil {
		t.Fatal("scoped candidates entered old whole-source evidence path")
	}
	entry, err := qualifiedCASReferenceEntry(binding, row, at, "actual-fresh-owner-component-source-business-related-ai")
	if err != nil || entry.Proof.Class != evidence.Unverifiable || entry.Proof.Verification.Method != "actual-fresh-owner-component-source-business-related-ai" || entry.Proof.Verification.Reason != "storage_precision_gap" || entry.Proof.Digest != row.facts.Source.Digest || entry.Proof.Digest == row.facts.ContentDigest {
		t.Fatal("fresh scoped entry changed identity, evidence scope or explicit gap")
	}
	if _, err := qualifiedCASReferenceEntry(binding, row, at, "actual-whole-source-joint-fresh-origin-ai14"); err == nil {
		t.Fatal("method scope mismatched native row origin")
	}
	if _, err := qualifiedCASReferenceEntry(binding, row, at, "caller_approved_complete"); err == nil {
		t.Fatal("unknown verification method admitted")
	}
	// This is an entry-format regression only. No actual qualification is
	// manufactured from the private row fixture above.
	for _, o := range []*HistoricalComponentObservation{nil, {}, {source: row.sourceObservation, rows: []qualifiedCASRow{row}}} {
		sql, mongo, refs, err := ApplyQualifiedHistoricalComponent(t.Context(), o, &AIExternalExecutionQualification{}, &AICommandPersistenceBatch{})
		if err == nil || len(sql) != 0 || mongo != nil || refs != 0 {
			t.Fatal("unbound opaque input performed physical effects")
		}
	}
}
