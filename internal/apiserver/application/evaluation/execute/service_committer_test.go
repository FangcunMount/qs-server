package execute

import (
	"context"
	"errors"
	"testing"
	"time"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcomecommit "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/commit"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
)

type evaluationCommitterStub struct {
	calls   int
	request outcomecommit.Request
}

type failingEvaluationCommitter struct {
	err error
}

func (c failingEvaluationCommitter) Commit(context.Context, outcomecommit.Request) (*domainoutcome.Record, error) {
	return nil, c.err
}

func (s *evaluationCommitterStub) Commit(_ context.Context, request outcomecommit.Request) (*domainoutcome.Record, error) {
	s.calls++
	s.request = request
	if err := request.Outcome.Assessment.ApplyScoringOutcome(evaloutcome.AssessmentOutcomeFromExecution(request.Outcome.Execution)); err != nil {
		return nil, err
	}
	if err := request.Run.Succeed(time.Unix(200, 0)); err != nil {
		return nil, err
	}
	return nil, nil
}

func TestEvaluateDelegatesSuccessfulTerminalPersistenceToEvaluationCommitter(t *testing.T) {
	t.Parallel()

	a := splitPhaseAssessment(t)
	execution := domainAssessment.NewAssessmentOutcome(
		*a.EvaluationModelRef(),
		domainAssessment.ResultSummary{PrimaryLabel: "ok"},
		domainAssessment.EvaluationDetail{Kind: domainAssessment.EvaluationModelKindScale},
	)
	evaluator := &countingEvaluator{key: evaluation.ExecutionIdentityScaleDefault, outcome: execution}
	registry, err := NewEvaluatorRegistry(evaluator)
	if err != nil {
		t.Fatal(err)
	}
	runRepo := &stubRunRepo{}
	committer := &evaluationCommitterStub{}
	svc := NewEngine(
		&fakeAssessmentRepo{assessment: a},
		stubInputResolver{},
		WithEvaluatorRegistry(registry),
		WithRunRepository(runRepo),
		WithEvaluationCommitter(committer),
		WithTransactionalOutbox(&engineRecordingTxRunner{}, &engineRecordingEventStager{}),
	)

	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatal(err)
	}
	if committer.calls != 1 || committer.request.Run == nil || committer.request.Run.Attempt.Status.String() != "succeeded" {
		t.Fatalf("committer request = %#v calls=%d", committer.request, committer.calls)
	}
	for _, saved := range runRepo.saved {
		if saved.Attempt.Status.String() == "succeeded" {
			t.Fatalf("service persisted succeeded run outside committer: %#v", runRepo.saved)
		}
	}
}

func TestEvaluateSkipsAlreadyEvaluatedAssessment(t *testing.T) {
	t.Parallel()

	a := splitPhaseAssessment(t)
	execution := domainAssessment.NewAssessmentOutcome(
		*a.EvaluationModelRef(),
		domainAssessment.ResultSummary{PrimaryLabel: "ok"},
		domainAssessment.EvaluationDetail{Kind: domainAssessment.EvaluationModelKindScale},
	)
	if err := a.ApplyScoringOutcome(execution); err != nil {
		t.Fatal(err)
	}
	evaluator := &countingEvaluator{key: evaluation.ExecutionIdentityScaleDefault, outcome: execution}
	registry, err := NewEvaluatorRegistry(evaluator)
	if err != nil {
		t.Fatal(err)
	}
	committer := &evaluationCommitterStub{}
	svc := NewEngine(
		&fakeAssessmentRepo{assessment: a},
		stubInputResolver{},
		WithEvaluatorRegistry(registry),
		WithRunRepository(&stubRunRepo{}),
		WithEvaluationCommitter(committer),
		WithTransactionalOutbox(&engineRecordingTxRunner{}, &engineRecordingEventStager{}),
	)

	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatal(err)
	}
	if evaluator.calls != 0 || committer.calls != 0 {
		t.Fatalf("duplicate evaluated execution: evaluator=%d committer=%d", evaluator.calls, committer.calls)
	}
}

func TestEvaluateRejectsSuccessfulPathWithoutEvaluationCommitter(t *testing.T) {
	t.Parallel()

	a := splitPhaseAssessment(t)
	evaluator := &countingEvaluator{key: evaluation.ExecutionIdentityScaleDefault}
	registry, err := NewEvaluatorRegistry(evaluator)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewEngine(
		&fakeAssessmentRepo{assessment: a},
		stubInputResolver{},
		WithEvaluatorRegistry(registry),
		WithRunRepository(&stubRunRepo{}),
		WithTransactionalOutbox(&engineRecordingTxRunner{}, &engineRecordingEventStager{}),
	)

	err = svc.Evaluate(context.Background(), a.ID().Uint64())
	if err == nil {
		t.Fatalf("Evaluate error = %v, want missing EvaluationCommitter", err)
	}
	if evaluator.calls != 1 {
		t.Fatalf("evaluator calls = %d, want 1", evaluator.calls)
	}
}

func TestEvaluateCommitFailureFinalizesRetryableRunAndAllowsNextAttempt(t *testing.T) {
	t.Parallel()

	commitErr := errors.New("evaluation commit failed")
	a := splitPhaseAssessment(t)
	a.ClearEvents()
	execution := domainAssessment.NewAssessmentOutcome(
		*a.EvaluationModelRef(),
		domainAssessment.ResultSummary{PrimaryLabel: "ok"},
		domainAssessment.EvaluationDetail{Kind: domainAssessment.EvaluationModelKindScale},
	)
	evaluator := &countingEvaluator{key: evaluation.ExecutionIdentityScaleDefault, outcome: execution}
	registry, err := NewEvaluatorRegistry(evaluator)
	if err != nil {
		t.Fatal(err)
	}
	assessmentRepo := &fakeAssessmentRepo{assessment: a}
	runRepo := &stubRunRepo{}
	stager := &engineRecordingEventStager{}
	svc := NewEngine(
		assessmentRepo,
		stubInputResolver{},
		WithEvaluatorRegistry(registry),
		WithRunRepository(runRepo),
		WithEvaluationCommitter(failingEvaluationCommitter{err: commitErr}),
		WithTransactionalOutbox(&engineRecordingTxRunner{}, stager),
	).(*service)

	err = svc.Evaluate(context.Background(), a.ID().Uint64())
	if !errors.Is(err, commitErr) {
		t.Fatalf("Evaluate error = %v, want %v", err, commitErr)
	}
	if !a.Status().IsFailed() {
		t.Fatalf("assessment status = %s, want failed", a.Status())
	}
	if len(runRepo.saved) != 3 {
		t.Fatalf("saved runs = %d, want running, input snapshot, and failed", len(runRepo.saved))
	}
	failedRun := runRepo.saved[len(runRepo.saved)-1]
	if failedRun.Attempt.Status != evalrun.StatusFailed || !failedRun.Retryable() || failedRun.Failure == nil || failedRun.Failure.Kind != evalrun.FailureKindInternal {
		t.Fatalf("failed run = %#v, want retryable internal failure", failedRun)
	}
	if len(stager.eventTypes) != 1 || stager.eventTypes[0] != domainAssessment.EventTypeFailed {
		t.Fatalf("staged events = %#v, want evaluation.failed", stager.eventTypes)
	}

	if err := a.RetryFromFailed(); err != nil {
		t.Fatalf("RetryFromFailed: %v", err)
	}
	a.ClearEvents()
	runRepo.latest = &failedRun
	nextRun, err := svc.newEvaluationRun(context.Background(), a.ID().Uint64())
	if err != nil {
		t.Fatalf("newEvaluationRun after recovery: %v", err)
	}
	if nextRun.Attempt.Number != failedRun.Attempt.Number+1 || nextRun.Attempt.Status != evalrun.StatusPending {
		t.Fatalf("next run = %#v, want next pending attempt", nextRun)
	}
}
