package scheduler

import (
	"context"
	"reflect"
	"testing"
	"time"

	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationconsistency"
	eventevidence "github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
)

type consistencyReaderStub struct {
	batches map[uint64]evaluationconsistency.Batch
	after   []uint64
}

func (s *consistencyReaderStub) ReadBatch(_ context.Context, after uint64, _ int) (evaluationconsistency.Batch, error) {
	s.after = append(s.after, after)
	return s.batches[after], nil
}

func TestAuditBatchUsesExplicitWatermarkAndReportsCompletion(t *testing.T) {
	reader := &consistencyReaderStub{batches: map[uint64]evaluationconsistency.Batch{
		0: {
			Items: []evaluationconsistency.AssessmentEvidence{
				{AssessmentID: 1, Status: string(domainassessment.StatusSubmitted)},
				{AssessmentID: 2, Status: string(domainassessment.StatusSubmitted)},
			},
			NextCursor: 2,
		},
		2: {
			Items: []evaluationconsistency.AssessmentEvidence{
				{AssessmentID: 3, Status: string(domainassessment.StatusSubmitted)},
			},
			NextCursor:    3,
			CycleComplete: true,
		},
	}}
	service := NewService(reader)

	first, err := service.AuditBatch(context.Background(), 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if first.Scanned != 2 || first.NextCursor != 2 || first.CycleComplete {
		t.Fatalf("first batch = %#v", first)
	}
	second, err := service.AuditBatch(context.Background(), first.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if second.Scanned != 1 || second.NextCursor != 3 || !second.CycleComplete {
		t.Fatalf("second batch = %#v", second)
	}
	if want := []uint64{0, 2}; !reflect.DeepEqual(reader.after, want) {
		t.Fatalf("watermarks = %v, want %v", reader.after, want)
	}
}

func TestClassifyDriftMatrix(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	runningExpired := &evaluationconsistency.RunEvidence{ID: "r1", Status: string(evalrun.StatusRunning), LeaseExpiresAt: &expired}
	succeeded := &evaluationconsistency.RunEvidence{ID: "r2", Status: string(evalrun.StatusSucceeded)}
	failedRun := &evaluationconsistency.RunEvidence{ID: "r3", Status: string(evalrun.StatusFailed)}
	outcome := testOutcome(modelcatalog.KindScale, "r2")
	projection := &evaluationconsistency.ProjectionEvidence{
		RowCount: 1, DistinctOutcomeCount: 1, OutcomeID: outcome.ID,
	}
	outbox := &evaluationconsistency.CommittedOutboxEvidence{
		RowCount: 1, OutcomeID: outcome.ID, RunID: outcome.RunID, Status: "published",
	}

	cases := []struct {
		name     string
		evidence consistencyEvidence
		wantKind mismatchKind
	}{
		{name: "submitted+outcome", evidence: consistencyEvidence{status: domainassessment.StatusSubmitted, outcome: outcome, projection: projection, outbox: outbox}, wantKind: mismatchOutcomeWithoutEvaluatedStatus},
		{name: "submitted+outcome+succeeded", evidence: consistencyEvidence{status: domainassessment.StatusSubmitted, outcome: outcome, run: succeeded, projection: projection, outbox: outbox}, wantKind: mismatchSuccessProjectionDrift},
		{name: "lease expired", evidence: consistencyEvidence{status: domainassessment.StatusSubmitted, run: runningExpired}, wantKind: mismatchLeaseRecoveryCandidate},
		{name: "evaluated missing outcome", evidence: consistencyEvidence{status: domainassessment.StatusEvaluated}, wantKind: mismatchCanonicalOutcomeMissing},
		{name: "evaluated run mismatch", evidence: consistencyEvidence{status: domainassessment.StatusEvaluated, outcome: outcome, run: failedRun, projection: projection, outbox: outbox}, wantKind: mismatchRunStatusMismatch},
		{name: "terminal conflict", evidence: consistencyEvidence{status: domainassessment.StatusFailed, outcome: outcome, run: succeeded, projection: projection, outbox: outbox}, wantKind: mismatchTerminalConflict},
		{name: "projection without outcome", evidence: consistencyEvidence{status: domainassessment.StatusSubmitted, projection: projection}, wantKind: mismatchProjectionWithoutOutcome},
		{name: "scale projection missing", evidence: consistencyEvidence{status: domainassessment.StatusEvaluated, outcome: outcome, run: succeeded, outbox: outbox}, wantKind: mismatchProjectionMissing},
		{name: "projection outcome mismatch", evidence: consistencyEvidence{status: domainassessment.StatusEvaluated, outcome: outcome, run: succeeded, projection: &evaluationconsistency.ProjectionEvidence{RowCount: 1, DistinctOutcomeCount: 1, OutcomeID: "other"}, outbox: outbox}, wantKind: mismatchProjectionOutcomeMismatch},
		{name: "outbox without outcome", evidence: consistencyEvidence{status: domainassessment.StatusSubmitted, outbox: outbox}, wantKind: mismatchCommittedOutboxWithoutOutcome},
		{name: "committed outbox missing", evidence: consistencyEvidence{status: domainassessment.StatusEvaluated, outcome: outcome, run: succeeded, projection: projection}, wantKind: mismatchCommittedOutboxMissing},
		{name: "committed outbox mismatch", evidence: consistencyEvidence{status: domainassessment.StatusEvaluated, outcome: outcome, run: succeeded, projection: projection, outbox: &evaluationconsistency.CommittedOutboxEvidence{RowCount: 1, OutcomeID: "other", RunID: "other"}}, wantKind: mismatchCommittedOutboxMismatch},
		{name: "run outcome reference mismatch", evidence: consistencyEvidence{status: domainassessment.StatusEvaluated, outcome: outcome, run: failedRun, projection: projection, outbox: outbox}, wantKind: mismatchRunOutcomeReferenceMismatch},
		{name: "healthy submitted", evidence: consistencyEvidence{status: domainassessment.StatusSubmitted}, wantKind: ""},
		{name: "healthy evaluated", evidence: consistencyEvidence{status: domainassessment.StatusEvaluated, outcome: outcome, run: succeeded, projection: projection, outbox: outbox}, wantKind: ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := classifyDrifts(tc.evidence, now)
			if tc.wantKind == "" {
				if len(got) != 0 {
					t.Fatalf("classifyDrift() = %#v, want nil", got)
				}
				return
			}
			if !containsMismatch(got, tc.wantKind) {
				t.Fatalf("classifyDrift() = %#v, want kind %s", got, tc.wantKind)
			}
		})
	}
}

func TestAuditBatchRejectsMissingReader(t *testing.T) {
	if _, err := NewService(nil).AuditBatch(context.Background(), 0, 10); err == nil {
		t.Fatal("expected module configuration error")
	}
}

func testOutcome(kind modelcatalog.Kind, runID string) *evaluationconsistency.OutcomeEvidence {
	return &evaluationconsistency.OutcomeEvidence{ID: "101", RunID: runID, ModelKind: string(kind)}
}

func containsMismatch(items []*mismatch, kind mismatchKind) bool {
	for _, item := range items {
		if item != nil && item.Kind == kind && item.RecommendedAction != "" {
			return true
		}
	}
	return false
}

func TestHistoricalEventProvenanceIsNotCurrentMessageVerification(t *testing.T) {
	outcome := testOutcome(modelcatalog.KindScale, "r2")
	base := consistencyEvidence{status: domainassessment.StatusEvaluated, outcome: outcome, run: &evaluationconsistency.RunEvidence{ID: "r2", Status: string(evalrun.StatusSucceeded)}, projection: &evaluationconsistency.ProjectionEvidence{RowCount: 1, DistinctOutcomeCount: 1, OutcomeID: outcome.ID}, outbox: &evaluationconsistency.CommittedOutboxEvidence{Class: eventevidence.RetiredVerified, RowCount: 0, OutcomeID: outcome.ID, RunID: outcome.RunID}}
	if got := classifyDrifts(base, time.Now()); len(got) != 0 {
		t.Fatalf("verified retirement generated a false current-message missing alert: %#v", got)
	}
	base.outbox.Class = eventevidence.Unverifiable
	base.outbox.HistoricalReason = "original message absent, terminal ownership and closure verified"
	if got := classifyDrifts(base, time.Now()); !containsMismatch(got, mismatchHistoricalEventGap) {
		t.Fatal("historical gap was displayed as complete message verification")
	}
	base.outbox.InvalidReason = "business binding conflict"
	if got := classifyDrifts(base, time.Now()); !containsMismatch(got, mismatchCommittedOutboxMismatch) {
		t.Fatal("historical class hid a binding conflict")
	}
}

func TestMissingRunAuditAcceptsOnlyReaderVerifiedHistoricalGap(t *testing.T) {
	outcome := testOutcome(modelcatalog.KindScale, "original-run")
	base := consistencyEvidence{status: domainassessment.StatusEvaluated, outcome: outcome, projection: &evaluationconsistency.ProjectionEvidence{RowCount: 1, DistinctOutcomeCount: 1, OutcomeID: outcome.ID}, outbox: &evaluationconsistency.CommittedOutboxEvidence{OutcomeID: outcome.ID, RunID: outcome.RunID, LegacyCanonicalAbsent: true, InvalidReason: "canonical outcome lacks classified committed event evidence"}, committedHistory: &evaluationconsistency.CommittedHistoricalEvidence{Class: eventevidence.Unverifiable, OutcomeID: outcome.ID, RunID: outcome.RunID, Reasons: []string{"original_outcome_run_absent"}}}
	if got := classifyDrifts(base, time.Now()); len(got) != 0 {
		t.Fatalf("qualified original-ID gap was double-counted or made a high current-Run finding: %#v", got)
	}
	otherClosed := base
	otherClosed.run = &evaluationconsistency.RunEvidence{ID: "retained-earlier-run", Status: string(evalrun.StatusSucceeded)}
	if got := classifyDrifts(otherClosed, time.Now()); len(got) != 0 {
		t.Fatalf("another closed Run was substituted for the missing original identity: %#v", got)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*consistencyEvidence)
	}{
		{"pending other Run", func(v *consistencyEvidence) {
			v.run = &evaluationconsistency.RunEvidence{ID: "other-run", Status: string(evalrun.StatusPending)}
		}},
		{"running other Run", func(v *consistencyEvidence) {
			v.run = &evaluationconsistency.RunEvidence{ID: "other-run", Status: string(evalrun.StatusRunning)}
		}},
		{"leased terminal other Run", func(v *consistencyEvidence) {
			lease := time.Now().Add(time.Hour)
			v.run = &evaluationconsistency.RunEvidence{ID: "other-run", Status: string(evalrun.StatusSucceeded), LeaseExpiresAt: &lease}
		}},
		{"unqualified closed other Run", func(v *consistencyEvidence) {
			v.run = &evaluationconsistency.RunEvidence{ID: "other-run", Status: string(evalrun.StatusSucceeded)}
			v.committedHistory.Reasons = []string{"not_original_outcome_run_absent"}
		}},
		{"reason similar", func(v *consistencyEvidence) { v.committedHistory.Reasons = []string{"not_original_outcome_run_absent"} }},
		{"verified missing Run", func(v *consistencyEvidence) { v.committedHistory.Class = eventevidence.RetiredVerified }},
		{"standard reference", func(v *consistencyEvidence) { v.outbox.Class = eventevidence.StandardReferenceClass }},
		{"standard corruption", func(v *consistencyEvidence) { v.outbox.InvalidReason = "standard fingerprint damaged" }},
		{"canonical pair present", func(v *consistencyEvidence) { v.outbox.LegacyCanonicalAbsent = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := base
			o, h := *base.outbox, *base.committedHistory
			v.outbox, v.committedHistory = &o, &h
			tc.mutate(&v)
			if got := classifyDrifts(v, time.Now()); !containsMismatch(got, mismatchRunOutcomeReferenceMismatch) {
				t.Fatal("historical gap suppressed an unqualified actual-Run mismatch")
			}
		})
	}
	reader := &consistencyReaderStub{batches: map[uint64]evaluationconsistency.Batch{0: {Items: []evaluationconsistency.AssessmentEvidence{{AssessmentID: 42, Status: string(base.status), Outcome: base.outcome, Run: otherClosed.run, Projection: base.projection, Outbox: base.outbox, CommittedHistory: base.committedHistory, HistoricalReferences: []evaluationconsistency.HistoricalReferenceEvidence{{Owner: "evaluation_outcome", OwnerID: outcome.ID, EventType: "evaluation.outcome.committed", Class: eventevidence.Unverifiable, RunID: outcome.RunID, HistoricalReason: "original_outcome_run_absent", LegacyCanonicalAbsent: true}}}}}}}
	result, err := NewService(reader).AuditBatch(t.Context(), 0, 1)
	if err != nil || result.Detected != 1 {
		t.Fatal("original historical gap was lost or counted twice", result, err)
	}
}
