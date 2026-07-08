package execute

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
)

type stubRunRepo struct {
	latest   *evalrun.EvaluationRun
	saved    []evalrun.EvaluationRun
	saveErr  error
	saveErrs []error
}

func (r *stubRunRepo) Save(_ context.Context, run evalrun.EvaluationRun) error {
	r.saved = append(r.saved, run)
	if len(r.saveErrs) > 0 {
		err := r.saveErrs[0]
		r.saveErrs = r.saveErrs[1:]
		return err
	}
	return r.saveErr
}

func (r *stubRunRepo) FindLatestByAssessmentID(_ context.Context, _ uint64) (*evalrun.EvaluationRun, error) {
	return r.latest, nil
}

func (r *stubRunRepo) ListByAssessmentID(_ context.Context, _ uint64, _ int) ([]evalrun.EvaluationRun, error) {
	if r.latest == nil {
		return nil, nil
	}
	return []evalrun.EvaluationRun{*r.latest}, nil
}

func (r *stubRunRepo) ListRetryableFailed(_ context.Context, _ evaluationrun.ListRetryableFailedParams) (*evaluationrun.ListRetryableFailedResult, error) {
	return &evaluationrun.ListRetryableFailedResult{}, nil
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

func TestEvaluateReturnsRunPersistenceErrorBeforeExecuting(t *testing.T) {
	t.Parallel()

	persistErr := errors.New("run save failed")
	a := splitPhaseAssessment(t)
	evaluator := &countingEvaluator{key: evaluation.ExecutionIdentityScaleDefault}
	registry, err := NewEvaluatorRegistry(evaluator)
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry: %v", err)
	}
	capture := &splitPhaseCapture{}
	svc := newSplitPhaseTestService(
		&fakeAssessmentRepo{assessment: a},
		stubInputResolver{},
		capture,
		WithEvaluatorRegistry(registry),
		WithRunRepository(&stubRunRepo{saveErr: persistErr}),
	)

	err = svc.Evaluate(context.Background(), a.ID().Uint64())
	if !errors.Is(err, persistErr) {
		t.Fatalf("Evaluate error = %v, want run persistence error", err)
	}
	if evaluator.calls != 0 {
		t.Fatalf("evaluator calls = %d, want 0 after start run persist failure", evaluator.calls)
	}
	if capture.ScoringCalls != 0 || capture.InterpretationCalls != 0 {
		t.Fatalf("split phase calls = scoring:%d interpretation:%d, want none", capture.ScoringCalls, capture.InterpretationCalls)
	}
}

func TestEvaluateReturnsCurrentRunPersistenceErrorBeforeExecuting(t *testing.T) {
	t.Parallel()

	persistErr := errors.New("current run id save failed")
	a := splitPhaseAssessment(t)
	evaluator := &countingEvaluator{key: evaluation.ExecutionIdentityScaleDefault}
	registry, err := NewEvaluatorRegistry(evaluator)
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry: %v", err)
	}
	capture := &splitPhaseCapture{}
	repo := &fakeAssessmentRepo{assessment: a, saveErr: persistErr}
	svc := newSplitPhaseTestService(
		repo,
		stubInputResolver{},
		capture,
		WithEvaluatorRegistry(registry),
		WithRunRepository(&stubRunRepo{}),
	)

	err = svc.Evaluate(context.Background(), a.ID().Uint64())
	if !errors.Is(err, persistErr) {
		t.Fatalf("Evaluate error = %v, want current run persistence error", err)
	}
	if evaluator.calls != 0 {
		t.Fatalf("evaluator calls = %d, want 0 after current run persist failure", evaluator.calls)
	}
	if !a.Status().IsFailed() {
		t.Fatalf("assessment status = %s, want failed for start persistence failure", a.Status())
	}
}

func TestEvaluateReturnsOriginalExecutionErrorWhenFailedRunPersists(t *testing.T) {
	t.Parallel()

	executeErr := errors.New("calculator failed")
	a := splitPhaseAssessment(t)
	registry, err := NewEvaluatorRegistry(evaluatorStub{
		key: evaluation.ExecutionIdentityScaleDefault,
		execute: func(context.Context, ExecutionInput) (*domainAssessment.AssessmentOutcome, error) {
			return nil, executeErr
		},
	})
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry: %v", err)
	}
	runRepo := &stubRunRepo{}
	svc := NewService(
		&fakeAssessmentRepo{assessment: a},
		stubInputResolver{},
		WithEvaluatorRegistry(registry),
		WithRunRepository(runRepo),
	).(*service)

	err = svc.Evaluate(context.Background(), a.ID().Uint64())
	if !errors.Is(err, executeErr) {
		t.Fatalf("Evaluate error = %v, want original execution error", err)
	}
	if len(runRepo.saved) != 2 {
		t.Fatalf("saved runs = %d, want running and failed", len(runRepo.saved))
	}
	if got := runRepo.saved[len(runRepo.saved)-1].Attempt.Status; got != evalrun.StatusFailed {
		t.Fatalf("last run status = %s, want failed", got)
	}
}

func TestEvaluateReturnsFailedRunPersistenceErrorWhenExecutionFails(t *testing.T) {
	t.Parallel()

	executeErr := errors.New("calculator failed")
	persistErr := errors.New("failed run save failed")
	a := splitPhaseAssessment(t)
	registry, err := NewEvaluatorRegistry(evaluatorStub{
		key: evaluation.ExecutionIdentityScaleDefault,
		execute: func(context.Context, ExecutionInput) (*domainAssessment.AssessmentOutcome, error) {
			return nil, executeErr
		},
	})
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry: %v", err)
	}
	runRepo := &stubRunRepo{saveErrs: []error{nil, persistErr}}
	svc := NewService(
		&fakeAssessmentRepo{assessment: a},
		stubInputResolver{},
		WithEvaluatorRegistry(registry),
		WithRunRepository(runRepo),
	).(*service)

	err = svc.Evaluate(context.Background(), a.ID().Uint64())
	if !errors.Is(err, persistErr) {
		t.Fatalf("Evaluate error = %v, want failed run persistence error", err)
	}
	if len(runRepo.saved) != 2 {
		t.Fatalf("saved runs = %d, want running and failed", len(runRepo.saved))
	}
	if got := runRepo.saved[len(runRepo.saved)-1].Attempt.Status; got != evalrun.StatusFailed {
		t.Fatalf("last run status = %s, want failed", got)
	}
	if !a.Status().IsFailed() {
		t.Fatalf("assessment status = %s, want failed after execution failure", a.Status())
	}
}

func TestEvaluateReturnsSucceededRunPersistenceErrorAfterScoring(t *testing.T) {
	t.Parallel()

	persistErr := errors.New("succeeded run save failed")
	a := splitPhaseAssessment(t)
	outcome := domainAssessment.NewAssessmentOutcome(
		*a.EvaluationModelRef(),
		domainAssessment.ResultSummary{PrimaryLabel: "ok"},
		domainAssessment.EvaluationDetail{Kind: domainAssessment.EvaluationModelKindScale},
	)
	evaluator := &countingEvaluator{key: evaluation.ExecutionIdentityScaleDefault, outcome: outcome}
	registry, err := NewEvaluatorRegistry(evaluator)
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry: %v", err)
	}
	capture := &splitPhaseCapture{}
	runRepo := &stubRunRepo{saveErrs: []error{nil, persistErr}}
	svc := newSplitPhaseTestService(
		&fakeAssessmentRepo{assessment: a},
		stubInputResolver{},
		capture,
		WithEvaluatorRegistry(registry),
		WithRunRepository(runRepo),
	)

	err = svc.Evaluate(context.Background(), a.ID().Uint64())
	if !errors.Is(err, persistErr) {
		t.Fatalf("Evaluate error = %v, want succeeded run persistence error", err)
	}
	if evaluator.calls != 1 {
		t.Fatalf("evaluator calls = %d, want 1", evaluator.calls)
	}
	if capture.ScoringCalls != 1 || capture.InterpretationCalls != 1 {
		t.Fatalf("split phase calls = scoring:%d interpretation:%d, want 1 each", capture.ScoringCalls, capture.InterpretationCalls)
	}
	if len(runRepo.saved) != 2 {
		t.Fatalf("saved runs = %d, want running and succeeded attempt", len(runRepo.saved))
	}
	if got := runRepo.saved[len(runRepo.saved)-1].Attempt.Status; got != evalrun.StatusSucceeded {
		t.Fatalf("last run status = %s, want succeeded", got)
	}
	if a.Status().IsFailed() {
		t.Fatalf("assessment status = %s, terminal run persistence error must not mark assessment failed", a.Status())
	}
}

func TestPersistStartedEvaluationRunReturnsCurrentRunSaveError(t *testing.T) {
	t.Parallel()

	persistErr := errors.New("assessment save failed")
	a := splitPhaseAssessment(t)
	run := evalrun.NewEvaluationRunWithAttempt(a.ID().Uint64(), 1)
	run.Start(time.Now())
	a.SetCurrentRunID(run.RunID)
	svc := &service{
		assessmentRepo: &fakeAssessmentRepo{assessment: a, saveErr: persistErr},
		runRepo:        &stubRunRepo{},
	}

	err := svc.persistStartedEvaluationRun(context.Background(), a, run)
	if !errors.Is(err, persistErr) {
		t.Fatalf("persistStartedEvaluationRun error = %v, want assessment save error", err)
	}
}
