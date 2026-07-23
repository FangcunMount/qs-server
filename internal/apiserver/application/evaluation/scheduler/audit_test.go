package scheduler

import (
	"context"
	"reflect"
	"testing"
	"time"

	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationconsistency"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type candidateReaderStub struct {
	ids   []uint64
	after []uint64
}

func (r *candidateReaderStub) ListSubmittedAssessmentIDsAfter(_ context.Context, after uint64, limit int) ([]uint64, error) {
	r.after = append(r.after, after)
	result := make([]uint64, 0, limit)
	for _, id := range r.ids {
		if id > after && len(result) < limit {
			result = append(result, id)
		}
	}
	return result, nil
}

type assessmentRepoStub struct{}

func (assessmentRepoStub) Save(context.Context, *domainassessment.Assessment) error { return nil }
func (assessmentRepoStub) FindByID(context.Context, domainassessment.ID) (*domainassessment.Assessment, error) {
	return nil, nil
}
func (assessmentRepoStub) Delete(context.Context, domainassessment.ID) error { return nil }
func (assessmentRepoStub) FindByAnswerSheetID(context.Context, domainassessment.AnswerSheetRef) (*domainassessment.Assessment, error) {
	return nil, nil
}

type outcomeRepoStub struct{}

func (outcomeRepoStub) Save(context.Context, *domainoutcome.Record) error { return nil }
func (outcomeRepoStub) FindByID(context.Context, domainoutcome.ID) (*domainoutcome.Record, error) {
	return nil, nil
}
func (outcomeRepoStub) FindByAssessmentID(context.Context, domainoutcome.ID) (*domainoutcome.Record, error) {
	return nil, nil
}

type latestRunReaderStub struct {
	run *evalrun.EvaluationRun
}

func (s latestRunReaderStub) FindLatestByAssessmentID(context.Context, uint64) (*evalrun.EvaluationRun, error) {
	return s.run, nil
}

type consistencyReaderStub struct {
	projection *evaluationconsistency.ProjectionEvidence
	outbox     *evaluationconsistency.CommittedOutboxEvidence
}

func (s consistencyReaderStub) FindProjectionEvidence(context.Context, uint64) (*evaluationconsistency.ProjectionEvidence, error) {
	return s.projection, nil
}

func (s consistencyReaderStub) FindCommittedOutboxEvidence(context.Context, uint64) (*evaluationconsistency.CommittedOutboxEvidence, error) {
	return s.outbox, nil
}

func TestAuditOnceTraversesAllBatchesAndRestartsAfterEnd(t *testing.T) {
	reader := &candidateReaderStub{ids: []uint64{1, 2, 3, 4, 5}}
	service := NewService(assessmentRepoStub{}, outcomeRepoStub{}, reader, latestRunReaderStub{}, consistencyReaderStub{})
	for range 4 {
		if _, err := service.AuditOnce(context.Background(), 2); err != nil {
			t.Fatal(err)
		}
	}
	if want := []uint64{0, 2, 4, 5}; !reflect.DeepEqual(reader.after, want) {
		t.Fatalf("keyset cursors = %v, want %v", reader.after, want)
	}
	if _, err := service.AuditOnce(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	if reader.after[len(reader.after)-1] != 0 {
		t.Fatalf("scan did not restart: %v", reader.after)
	}
}

func TestClassifyDriftMatrix(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	runningExpired := evalrun.Reconstruct(evalrun.ReconstructInput{
		RunID: "r1", AssessmentID: 1, Attempt: evalrun.Attempt{Number: 1, Status: evalrun.StatusRunning},
		LeaseExpiresAt: &expired, StartedAt: now.Add(-2 * time.Minute),
	})
	succeeded := evalrun.Reconstruct(evalrun.ReconstructInput{
		RunID: "r2", AssessmentID: 1, Attempt: evalrun.Attempt{Number: 1, Status: evalrun.StatusSucceeded},
		StartedAt: now.Add(-time.Minute), FinishedAt: &now,
	})
	failedRun := evalrun.Reconstruct(evalrun.ReconstructInput{
		RunID: "r3", AssessmentID: 1, Attempt: evalrun.Attempt{Number: 1, Status: evalrun.StatusFailed},
		StartedAt: now.Add(-time.Minute), FinishedAt: &now,
	})
	outcome := mustTestOutcome(t, modelcatalog.KindScale, "r2", now)
	projection := &evaluationconsistency.ProjectionEvidence{
		RowCount: 1, DistinctOutcomeCount: 1, OutcomeID: outcome.ID().String(),
	}
	outbox := &evaluationconsistency.CommittedOutboxEvidence{
		RowCount: 1, OutcomeID: outcome.ID().String(), RunID: outcome.RunID(), Status: "published",
	}

	cases := []struct {
		name     string
		evidence consistencyEvidence
		wantKind mismatchKind
	}{
		{name: "submitted+outcome", evidence: consistencyEvidence{status: domainassessment.StatusSubmitted, outcome: outcome, projection: projection, outbox: outbox}, wantKind: mismatchOutcomeWithoutEvaluatedStatus},
		{name: "submitted+outcome+succeeded", evidence: consistencyEvidence{status: domainassessment.StatusSubmitted, outcome: outcome, run: &succeeded, projection: projection, outbox: outbox}, wantKind: mismatchSuccessProjectionDrift},
		{name: "lease expired", evidence: consistencyEvidence{status: domainassessment.StatusSubmitted, run: &runningExpired}, wantKind: mismatchLeaseRecoveryCandidate},
		{name: "evaluated missing outcome", evidence: consistencyEvidence{status: domainassessment.StatusEvaluated}, wantKind: mismatchCanonicalOutcomeMissing},
		{name: "evaluated run mismatch", evidence: consistencyEvidence{status: domainassessment.StatusEvaluated, outcome: outcome, run: &failedRun, projection: projection, outbox: outbox}, wantKind: mismatchRunStatusMismatch},
		{name: "terminal conflict", evidence: consistencyEvidence{status: domainassessment.StatusFailed, outcome: outcome, run: &succeeded, projection: projection, outbox: outbox}, wantKind: mismatchTerminalConflict},
		{name: "projection without outcome", evidence: consistencyEvidence{status: domainassessment.StatusSubmitted, projection: projection}, wantKind: mismatchProjectionWithoutOutcome},
		{name: "scale projection missing", evidence: consistencyEvidence{status: domainassessment.StatusEvaluated, outcome: outcome, run: &succeeded, outbox: outbox}, wantKind: mismatchProjectionMissing},
		{name: "projection outcome mismatch", evidence: consistencyEvidence{status: domainassessment.StatusEvaluated, outcome: outcome, run: &succeeded, projection: &evaluationconsistency.ProjectionEvidence{RowCount: 1, DistinctOutcomeCount: 1, OutcomeID: "other"}, outbox: outbox}, wantKind: mismatchProjectionOutcomeMismatch},
		{name: "outbox without outcome", evidence: consistencyEvidence{status: domainassessment.StatusSubmitted, outbox: outbox}, wantKind: mismatchCommittedOutboxWithoutOutcome},
		{name: "committed outbox missing", evidence: consistencyEvidence{status: domainassessment.StatusEvaluated, outcome: outcome, run: &succeeded, projection: projection}, wantKind: mismatchCommittedOutboxMissing},
		{name: "committed outbox mismatch", evidence: consistencyEvidence{status: domainassessment.StatusEvaluated, outcome: outcome, run: &succeeded, projection: projection, outbox: &evaluationconsistency.CommittedOutboxEvidence{RowCount: 1, OutcomeID: "other", RunID: "other"}}, wantKind: mismatchCommittedOutboxMismatch},
		{name: "run outcome reference mismatch", evidence: consistencyEvidence{status: domainassessment.StatusEvaluated, outcome: outcome, run: &failedRun, projection: projection, outbox: outbox}, wantKind: mismatchRunOutcomeReferenceMismatch},
		{name: "healthy submitted", evidence: consistencyEvidence{status: domainassessment.StatusSubmitted}, wantKind: ""},
		{name: "healthy evaluated", evidence: consistencyEvidence{status: domainassessment.StatusEvaluated, outcome: outcome, run: &succeeded, projection: projection, outbox: outbox}, wantKind: ""},
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

func TestAuditOnceRejectsMissingDependencies(t *testing.T) {
	reader := &candidateReaderStub{}
	for _, service := range []Service{
		NewService(nil, outcomeRepoStub{}, reader, latestRunReaderStub{}, consistencyReaderStub{}),
		NewService(assessmentRepoStub{}, nil, reader, latestRunReaderStub{}, consistencyReaderStub{}),
		NewService(assessmentRepoStub{}, outcomeRepoStub{}, nil, latestRunReaderStub{}, consistencyReaderStub{}),
		NewService(assessmentRepoStub{}, outcomeRepoStub{}, reader, nil, consistencyReaderStub{}),
		NewService(assessmentRepoStub{}, outcomeRepoStub{}, reader, latestRunReaderStub{}, nil),
	} {
		if _, err := service.AuditOnce(context.Background(), 10); err == nil {
			t.Fatal("expected module configuration error")
		}
	}
}

func mustTestOutcome(t *testing.T, kind modelcatalog.Kind, runID string, evaluatedAt time.Time) *domainoutcome.Record {
	t.Helper()
	record, err := domainoutcome.NewRecord(domainoutcome.NewRecordInput{
		ID:           meta.FromUint64(101),
		AssessmentID: meta.FromUint64(1),
		TesteeID:     7,
		RunID:        runID,
		Model:        domainoutcome.ModelIdentity{Kind: kind, Code: "MODEL", Version: "1"},
		Payload:      []byte(`{"schema_version":2}`),
		EvaluatedAt:  evaluatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func containsMismatch(items []*mismatch, kind mismatchKind) bool {
	for _, item := range items {
		if item != nil && item.Kind == kind && item.RecommendedAction != "" {
			return true
		}
	}
	return false
}
