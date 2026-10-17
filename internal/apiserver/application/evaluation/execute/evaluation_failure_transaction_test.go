package execute

import (
	"context"
	"errors"
	"slices"
	"testing"

	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	routing "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
)

func TestEvaluatorFailureCommitsRetryInBusinessTransaction(t *testing.T) {
	a := splitPhaseAssessment(t)
	repo := &fakeAssessmentRepo{assessment: a}
	runs := &stubRunRepo{}
	stager := &engineRecordingEventStager{}
	calls := 0
	cause := errors.New("isolated evaluator failure")
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
	if runs.latest == nil || runs.latest.Attempt().Status != evalrun.StatusFailed || !runs.latest.Retryable() {
		t.Fatalf("latest run=%v, want retryable failure", runs.latest)
	}
	if failure := runs.latest.Failure(); failure == nil || failure.Message != cause.Error() || failure.Kind != evalrun.FailureKindCalculation {
		t.Fatalf("failure=%v, want original evaluator failure", failure)
	}
	if !slices.Contains(stager.eventTypes, eventcatalog.EvaluationFailed) ||
		len(stager.scheduledEventTypes) != 1 || stager.scheduledEventTypes[0] != eventcatalog.EvaluationRetryRequested {
		t.Fatalf("events=%v scheduled=%v, want failed and scheduled retry", stager.eventTypes, stager.scheduledEventTypes)
	}
	if !stager.ctxHadTxMarker {
		t.Fatal("failure and retry intents were not staged in the business transaction")
	}
}
