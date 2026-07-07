package execute

import (
	"context"
	"testing"

	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
)

type stubRunRepo struct {
	latest *evalrun.EvaluationRun
	saved  []evalrun.EvaluationRun
}

func (r *stubRunRepo) Save(_ context.Context, run evalrun.EvaluationRun) error {
	r.saved = append(r.saved, run)
	return nil
}

func (r *stubRunRepo) FindLatestByAssessmentID(_ context.Context, _ uint64) (*evalrun.EvaluationRun, error) {
	return r.latest, nil
}

var _ evaluationrun.Repository = (*stubRunRepo)(nil)

func TestNewEvaluationRunUsesNextAttemptAfterRetryableFailure(t *testing.T) {
	t.Parallel()

	repo := &stubRunRepo{
		latest: &evalrun.EvaluationRun{
			AssessmentID: 99,
			Attempt:      evalrun.Attempt{Number: 1, Status: evalrun.StatusFailed},
			Failure:      &evalrun.Failure{Retryable: true},
		},
	}
	svc := &service{runRepo: repo}

	run, err := svc.newEvaluationRun(context.Background(), 99)
	if err != nil {
		t.Fatal(err)
	}
	if run.Attempt.Number != 2 {
		t.Fatalf("attempt=%d, want 2", run.Attempt.Number)
	}
	if run.RunID != "99:2" {
		t.Fatalf("run id=%s", run.RunID)
	}
}
