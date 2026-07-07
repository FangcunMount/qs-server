package runquery

import (
	"context"
	"testing"
	"time"

	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
)

type stubRunRepo struct {
	listByAssessment []evalrun.EvaluationRun
	latest           *evalrun.EvaluationRun
	failedPage       *evaluationrun.ListRetryableFailedResult
}

func (s *stubRunRepo) Save(context.Context, evalrun.EvaluationRun) error { return nil }

func (s *stubRunRepo) FindLatestByAssessmentID(context.Context, uint64) (*evalrun.EvaluationRun, error) {
	return s.latest, nil
}

func (s *stubRunRepo) ListByAssessmentID(context.Context, uint64, int) ([]evalrun.EvaluationRun, error) {
	return s.listByAssessment, nil
}

func (s *stubRunRepo) ListRetryableFailed(context.Context, evaluationrun.ListRetryableFailedParams) (*evaluationrun.ListRetryableFailedResult, error) {
	return s.failedPage, nil
}

func TestServiceListByAssessmentIDMapsRuns(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	repo := &stubRunRepo{
		listByAssessment: []evalrun.EvaluationRun{
			{
				RunID:        evalrun.ID("99:2"),
				AssessmentID: 99,
				Attempt:      evalrun.Attempt{Number: 2, Status: evalrun.StatusFailed},
				StartedAt:    started,
				Failure:      &evalrun.Failure{Kind: evalrun.FailureKindCalculation, Message: "boom", Retryable: true},
			},
		},
	}
	svc := NewService(repo)
	result, err := svc.ListByAssessmentID(context.Background(), 99, 10)
	if err != nil {
		t.Fatalf("ListByAssessmentID returned error: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].AttemptNo != 2 || !result.Items[0].Retryable {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestServiceListRetryableFailedRequiresOrg(t *testing.T) {
	t.Parallel()

	svc := NewService(&stubRunRepo{})
	if _, err := svc.ListRetryableFailed(context.Background(), 0, 10, 0); err == nil {
		t.Fatal("expected invalid org error")
	}
}
