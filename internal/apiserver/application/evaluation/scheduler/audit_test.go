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

func TestClassifyDriftsKeepsOrderedCompoundFindings(t *testing.T) {
	now := time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)
	outcome := testOutcome(modelcatalog.KindScale, "canonical-run")
	evidence := consistencyEvidence{
		status: domainassessment.StatusEvaluated, outcome: outcome,
		run:        &evaluationconsistency.RunEvidence{ID: "other-run", Status: string(evalrun.StatusFailed)},
		projection: &evaluationconsistency.ProjectionEvidence{RowCount: 2, UnlinkedRowCount: 1, DistinctOutcomeCount: 2},
		outbox:     &evaluationconsistency.CommittedOutboxEvidence{RowCount: 2},
	}
	want := []*mismatch{
		{Kind: mismatchProjectionOutcomeMismatch, Severity: severityHigh, RecommendedAction: "replace projection from the canonical outcome after operator confirmation", DetectedAt: now},
		{Kind: mismatchCommittedOutboxMismatch, Severity: severityHigh, RecommendedAction: "quarantine conflicting outbox evidence and require operator decision", DetectedAt: now},
		{Kind: mismatchRunOutcomeReferenceMismatch, Severity: severityHigh, RecommendedAction: "locate the exact run referenced by the canonical outcome", DetectedAt: now},
		{Kind: mismatchRunStatusMismatch, Severity: severityMedium, RecommendedAction: "audit run/status mismatch; manual confirmation required", DetectedAt: now},
	}
	if got := classifyDrifts(evidence, now); !reflect.DeepEqual(got, want) {
		t.Fatalf("ordered findings=%#v want=%#v", got, want)
	}
	evidence.status, evidence.outcome = domainassessment.StatusSubmitted, nil
	evidence.run = &evaluationconsistency.RunEvidence{Status: string(evalrun.StatusRunning), LeaseExpiresAt: &now}
	want = []*mismatch{
		{Kind: mismatchProjectionWithoutOutcome, Severity: severityHigh, RecommendedAction: "remove or rebuild projection only after locating the canonical outcome", DetectedAt: now},
		{Kind: mismatchCommittedOutboxWithoutOutcome, Severity: severityHigh, RecommendedAction: "quarantine committed event and investigate missing canonical outcome", DetectedAt: now},
		{Kind: mismatchLeaseRecoveryCandidate, Severity: severityMedium, RecommendedAction: "lease recovery / redelivery; do not rewrite assessment status here", DetectedAt: now},
	}
	if got := classifyDrifts(evidence, now); !reflect.DeepEqual(got, want) {
		t.Fatalf("orphan/lease findings=%#v want=%#v", got, want)
	}
	if got := classifyDrifts(consistencyEvidence{}, now); got == nil || len(got) != 0 {
		t.Fatalf("empty result shape changed: %#v", got)
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

	unsupported := base
	unsupported.outbox = &evaluationconsistency.CommittedOutboxEvidence{OutcomeID: outcome.ID, RunID: outcome.RunID, LegacyCanonicalAbsent: true, InvalidReason: "canonical outcome lacks classified committed event evidence"}
	unsupportedKind := mismatchKind("committed_event_audit_unsupported")
	if got := classifyDrifts(unsupported, time.Now()); len(got) != 1 || got[0].Kind != unsupportedKind || got[0].Severity != severityLow || got[0].RecommendedAction == "" {
		t.Fatalf("unclassified provenance must remain explicit unsupported coverage, got %#v", got)
	}
	batch := evaluationconsistency.Batch{Items: []evaluationconsistency.AssessmentEvidence{{AssessmentID: 42, Status: string(unsupported.status), Outcome: unsupported.outcome, Run: unsupported.run, Projection: unsupported.projection, Outbox: unsupported.outbox}}}
	if got := (&service{now: time.Now}).classifyReadOnlyBatch(batch, false); got.Detected != 1 || got.HistoricalGaps != 0 {
		t.Fatalf("unsupported coverage became message verification or retained provenance: %#v", got)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*evaluationconsistency.CommittedOutboxEvidence)
	}{
		{"canonical ID or evidence present", func(v *evaluationconsistency.CommittedOutboxEvidence) { v.LegacyCanonicalAbsent = false }},
		{"standard row present", func(v *evaluationconsistency.CommittedOutboxEvidence) { v.RowCount = 1 }},
		{"damaged evidence", func(v *evaluationconsistency.CommittedOutboxEvidence) {
			v.InvalidReason = "standard fingerprint damaged"
		}},
		{"standard class", func(v *evaluationconsistency.CommittedOutboxEvidence) { v.Class = eventevidence.StandardReferenceClass }},
		{"different outcome", func(v *evaluationconsistency.CommittedOutboxEvidence) { v.OutcomeID = "other" }},
		{"different run", func(v *evaluationconsistency.CommittedOutboxEvidence) { v.RunID = "other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := unsupported
			outbox := *v.outbox
			v.outbox = &outbox
			tc.mutate(v.outbox)
			got := classifyDrifts(v, time.Now())
			if containsMismatch(got, unsupportedKind) || !containsMismatch(got, mismatchCommittedOutboxMismatch) {
				t.Fatal("unsupported coverage concealed a current-message conflict", got)
			}
		})
	}
	missingRun := unsupported
	missingRun.run = nil
	if got := classifyDrifts(missingRun, time.Now()); !containsMismatch(got, unsupportedKind) || !containsMismatch(got, mismatchRunOutcomeReferenceMismatch) {
		t.Fatal("unsupported message coverage hid the actual missing Run", got)
	}
	missingProjection := unsupported
	missingProjection.projection = nil
	if got := classifyDrifts(missingProjection, time.Now()); !containsMismatch(got, unsupportedKind) || !containsMismatch(got, mismatchProjectionMissing) {
		t.Fatal("unsupported message coverage hid the actual missing projection", got)
	}
	terminalConflict := unsupported
	terminalConflict.status = domainassessment.StatusFailed
	if got := classifyDrifts(terminalConflict, time.Now()); !containsMismatch(got, unsupportedKind) || !containsMismatch(got, mismatchTerminalConflict) {
		t.Fatal("unsupported message coverage hid the actual terminal conflict", got)
	}
}

func TestMissingOriginalRunIsNeverHiddenByUnclassifiedEventHistory(t *testing.T) {
	outcome := testOutcome(modelcatalog.KindScale, "original-run")
	for _, test := range []struct {
		name    string
		run     *evaluationconsistency.RunEvidence
		missing bool
	}{
		{"absent", nil, true},
		{"other closed run", &evaluationconsistency.RunEvidence{ID: "another-run", Status: "succeeded"}, true},
		{"original run", &evaluationconsistency.RunEvidence{ID: "original-run", Status: "succeeded"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := consistencyEvidence{status: domainassessment.StatusEvaluated, outcome: outcome, run: test.run, outbox: &evaluationconsistency.CommittedOutboxEvidence{LegacyCanonicalAbsent: true, OutcomeID: outcome.ID, RunID: outcome.RunID, InvalidReason: "canonical outcome lacks classified committed event evidence"}}
			if got := classifyDrifts(state, time.Now()); containsMismatch(got, mismatchRunOutcomeReferenceMismatch) != test.missing {
				t.Fatalf("original Run responsibility was hidden: %#v", got)
			}
		})
	}
}
