package scheduler

import (
	"context"
	"reflect"
	"testing"
	"time"

	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
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

func TestAuditOnceTraversesAllBatchesAndRestartsAfterEnd(t *testing.T) {
	reader := &candidateReaderStub{ids: []uint64{1, 2, 3, 4, 5}}
	service := NewService(assessmentRepoStub{}, outcomeRepoStub{}, reader)
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

	cases := []struct {
		name       string
		status     domainassessment.Status
		hasOutcome bool
		run        *evalrun.EvaluationRun
		wantKind   mismatchKind
	}{
		{name: "submitted+outcome", status: domainassessment.StatusSubmitted, hasOutcome: true, wantKind: mismatchOutcomeWithoutEvaluatedStatus},
		{name: "submitted+outcome+succeeded", status: domainassessment.StatusSubmitted, hasOutcome: true, run: &succeeded, wantKind: mismatchSuccessProjectionDrift},
		{name: "lease expired", status: domainassessment.StatusSubmitted, run: &runningExpired, wantKind: mismatchLeaseRecoveryCandidate},
		{name: "evaluated missing outcome", status: domainassessment.StatusEvaluated, wantKind: mismatchCanonicalOutcomeMissing},
		{name: "evaluated run mismatch", status: domainassessment.StatusEvaluated, hasOutcome: true, run: &failedRun, wantKind: mismatchRunStatusMismatch},
		{name: "terminal conflict", status: domainassessment.StatusFailed, hasOutcome: true, run: &succeeded, wantKind: mismatchTerminalConflict},
		{name: "healthy submitted", status: domainassessment.StatusSubmitted, wantKind: ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := classifyDrift(tc.status, tc.hasOutcome, tc.run, now)
			if tc.wantKind == "" {
				if got != nil {
					t.Fatalf("classifyDrift() = %#v, want nil", got)
				}
				return
			}
			if got == nil || got.Kind != tc.wantKind || got.RecommendedAction == "" {
				t.Fatalf("classifyDrift() = %#v, want kind %s", got, tc.wantKind)
			}
		})
	}
}

func TestAuditOnceRejectsMissingDependencies(t *testing.T) {
	reader := &candidateReaderStub{}
	for _, service := range []Service{
		NewService(nil, outcomeRepoStub{}, reader),
		NewService(assessmentRepoStub{}, nil, reader),
		NewService(assessmentRepoStub{}, outcomeRepoStub{}, nil),
	} {
		if _, err := service.AuditOnce(context.Background(), 10); err == nil {
			t.Fatal("expected module configuration error")
		}
	}
}
