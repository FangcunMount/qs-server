package execute

import (
	"context"
	"slices"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	routing "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
)

type preExecutionFailureGateStub struct{ calls int }

func (g *preExecutionFailureGateStub) TryFail(_ context.Context, _ *assessment.Assessment, _ *evaluationinput.InputSnapshot, run *evalrun.EvaluationRun) (bool, error) {
	g.calls++
	return run.Attempt().Number == 1, nil
}

func TestControlledEvaluationFailureCommitsRetryBeforeModelExecution(t *testing.T) {
	a := splitPhaseAssessment(t)
	repo := &fakeAssessmentRepo{assessment: a}
	runs := &stubRunRepo{}
	stager := &engineRecordingEventStager{}
	evaluator := &countingEvaluator{key: routing.ExecutionIdentityScaleDefault}
	gate := &preExecutionFailureGateStub{}
	svc := NewEngine(repo, stubInputResolver{},
		withTestEvaluator(evaluator), WithRunRepository(runs),
		WithTransactionalOutbox(&engineRecordingTxRunner{}, stager),
		WithPreExecutionFailureGate(gate),
	).(*service)

	if err := svc.Evaluate(t.Context(), a.ID().Uint64()); err == nil {
		t.Fatal("controlled failure returned no error")
	}
	if evaluator.calls != 0 || gate.calls != 1 {
		t.Fatalf("evaluator calls=%d gate calls=%d, want 0/1", evaluator.calls, gate.calls)
	}
	if repo.assessment == nil || !repo.assessment.Status().IsFailed() {
		t.Fatalf("assessment status=%v, want failed", repo.assessment)
	}
	if runs.latest == nil || runs.latest.Attempt().Status != evalrun.StatusFailed || !runs.latest.Retryable() {
		t.Fatalf("latest run=%v, want retryable failure", runs.latest)
	}
	if failure := runs.latest.Failure(); failure == nil || failure.Message != "controlled_evaluation_failure" {
		t.Fatalf("failure=%v, want controlled audit code", failure)
	}
	if !slices.Contains(stager.eventTypes, eventcatalog.EvaluationFailed) ||
		len(stager.scheduledEventTypes) != 1 || stager.scheduledEventTypes[0] != eventcatalog.EvaluationRetryRequested {
		t.Fatalf("events=%v scheduled=%v, want failed and scheduled retry", stager.eventTypes, stager.scheduledEventTypes)
	}
	if !stager.ctxHadTxMarker {
		t.Fatal("failure and retry intents were not staged in the business transaction")
	}
}
