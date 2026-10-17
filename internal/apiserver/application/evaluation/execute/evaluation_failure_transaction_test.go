package execute

import (
	"context"
	"errors"
	"fmt"
	calcscoring "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scoring"
	"slices"
	"testing"

	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	routing "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
)

func TestEvaluatorFailureCommitsRetryInBusinessTransaction(t *testing.T) {
	assertEvaluatorFailureTransaction(t, errors.New("isolated evaluator failure"), true)
}

func TestDeterministicScoringFailureDoesNotScheduleRetry(t *testing.T) {
	_, scoringErr := (calcscoring.DefaultStrategyRegistry{}).ScoreFactor(t.Context(), calcscoring.Factor{Code: "bad", ScoringStrategy: "invalid"}, []float64{1})
	if scoringErr == nil {
		t.Fatal("invalid strategy accepted")
	}
	cause := fmt.Errorf("execute scoring: %w", scoringErr)
	assertEvaluatorFailureTransaction(t, cause, false)
}

func assertEvaluatorFailureTransaction(t *testing.T, cause error, retryable bool) {
	t.Helper()
	a := splitPhaseAssessment(t)
	repo := &fakeAssessmentRepo{assessment: a}
	runs := &stubRunRepo{}
	stager := &engineRecordingEventStager{}
	calls := 0
	evaluator := evaluatorStub{key: routing.ExecutionIdentityScaleDefault, execute: func(context.Context, ExecutionInput) (*domainoutcome.Execution, error) {
		calls++
		return nil, cause
	}}
	svc := NewEngine(repo, stubInputResolver{},
		withTestEvaluator(evaluator), WithRunRepository(runs),
		WithTransactionalOutbox(&engineRecordingTxRunner{}, stager),
	).(*service)

	if err := svc.Evaluate(t.Context(), a.ID().Uint64()); err == nil {
		t.Fatal("controlled failure returned no error")
	}
	if calls != 1 {
		t.Fatalf("evaluator calls=%d, want 1", calls)
	}
	if repo.assessment == nil || !repo.assessment.Status().IsFailed() {
		t.Fatalf("assessment status=%v, want failed", repo.assessment)
	}
	if runs.latest == nil || runs.latest.Attempt().Status != evalrun.StatusFailed || runs.latest.Retryable() != retryable {
		t.Fatalf("latest run=%v, want failed run with configured retry policy", runs.latest)
	}
	if failure := runs.latest.Failure(); failure == nil || failure.Message != cause.Error() || failure.Kind != evalrun.FailureKindCalculation {
		t.Fatalf("failure=%v, want original evaluator failure", failure)
	}
	if !slices.Contains(stager.eventTypes, eventcatalog.EvaluationFailed) {
		t.Fatalf("missing failed event: %v", stager.eventTypes)
	}
	wantScheduled := []string(nil)
	if retryable {
		wantScheduled = []string{eventcatalog.EvaluationRetryRequested}
	}
	if !slices.Equal(stager.scheduledEventTypes, wantScheduled) {
		t.Fatalf("scheduled=%v want %v", stager.scheduledEventTypes, wantScheduled)
	}
	if !stager.ctxHadTxMarker {
		t.Fatal("failure and retry intents were not staged in the business transaction")
	}
}
