package execute

import (
	"context"
	"errors"
	"testing"
	"time"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcomecommit "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/commit"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
)

type evaluationCommitterStub struct {
	calls   int
	request outcomecommit.CommitRequest
}

type failingEvaluationCommitter struct {
	err error
}

func (c failingEvaluationCommitter) Commit(context.Context, outcomecommit.CommitRequest) (*domainoutcome.Record, error) {
	return nil, c.err
}

func (s *evaluationCommitterStub) Commit(_ context.Context, request outcomecommit.CommitRequest) (*domainoutcome.Record, error) {
	s.calls++
	s.request = request
	if err := request.Assessment.ApplyScoringProjectionAt(evaloutcome.ScoringProjectionFromExecution(request.Execution), request.EvaluatedAt); err != nil {
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
	execution := executionForAssessment(a, "ok")
	evaluator := &countingEvaluator{key: evaluation.ExecutionIdentityScaleDefault, outcome: execution}
	runRepo := &stubRunRepo{}
	committer := &evaluationCommitterStub{}
	assessmentRepo := &fakeAssessmentRepo{assessment: a}
	svc := NewEngine(
		assessmentRepo,
		stubInputResolver{},
		withTestEvaluator(evaluator),
		WithRunRepository(runRepo),
		WithEvaluationCommitter(committer),
		WithTransactionalOutbox(&engineRecordingTxRunner{}, &engineRecordingEventStager{}),
	)

	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatal(err)
	}
	if committer.calls != 1 || committer.request.Run == nil || committer.request.Run.Attempt().Status.String() != "succeeded" {
		t.Fatalf("committer request = %#v calls=%d", committer.request, committer.calls)
	}
	for _, saved := range runRepo.saved {
		if saved.Attempt().Status.String() == "succeeded" {
			t.Fatalf("service persisted succeeded run outside committer: %#v", runRepo.saved)
		}
	}
}

func TestEvaluateSkipsAlreadyEvaluatedAssessment(t *testing.T) {
	t.Parallel()

	a := splitPhaseAssessment(t)
	execution := executionForAssessment(a, "ok")
	if err := a.ApplyScoringProjectionAt(evaloutcome.ScoringProjectionFromExecution(execution), time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	evaluator := &countingEvaluator{key: evaluation.ExecutionIdentityScaleDefault, outcome: execution}
	committer := &evaluationCommitterStub{}
	svc := NewEngine(
		&fakeAssessmentRepo{assessment: a},
		stubInputResolver{},
		withTestEvaluator(evaluator),
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
	svc := NewEngine(
		&fakeAssessmentRepo{assessment: a},
		stubInputResolver{},
		withTestEvaluator(evaluator),
		WithRunRepository(&stubRunRepo{}),
		WithTransactionalOutbox(&engineRecordingTxRunner{}, &engineRecordingEventStager{}),
	)

	err := svc.Evaluate(context.Background(), a.ID().Uint64())
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
	execution := executionForAssessment(a, "ok")
	evaluator := &countingEvaluator{key: evaluation.ExecutionIdentityScaleDefault, outcome: execution}
	assessmentRepo := &fakeAssessmentRepo{assessment: a}
	runRepo := &stubRunRepo{}
	stager := &engineRecordingEventStager{}
	svc := NewEngine(
		assessmentRepo,
		stubInputResolver{},
		withTestEvaluator(evaluator),
		WithRunRepository(runRepo),
		WithEvaluationCommitter(failingEvaluationCommitter{err: commitErr}),
		WithTransactionalOutbox(&engineRecordingTxRunner{}, stager),
	).(*service)

	err := svc.Evaluate(context.Background(), a.ID().Uint64())
	if !errors.Is(err, commitErr) {
		t.Fatalf("Evaluate error = %v, want %v", err, commitErr)
	}
	if !a.Status().IsFailed() {
		t.Fatalf("assessment status = %s, want failed", a.Status())
	}
	if len(runRepo.saved) != 2 {
		t.Fatalf("saved runs = %d, want input snapshot and terminal failed run", len(runRepo.saved))
	}
	failedRun := runRepo.saved[len(runRepo.saved)-1]
	if failedRun.Attempt().Status != evalrun.StatusFailed || !failedRun.Retryable() || failedRun.Failure() == nil || failedRun.Failure().Kind != evalrun.FailureKindInternal {
		t.Fatalf("failed run = %#v, want retryable internal failure", failedRun)
	}
	if len(stager.eventTypes) != 1 || stager.eventTypes[0] != domainAssessment.EventTypeFailed {
		t.Fatalf("staged events = %#v, want evaluation.failed", stager.eventTypes)
	}

	if err := a.RetryFromFailed(); err != nil {
		t.Fatalf("RetryFromFailed: %v", err)
	}
	a.ClearEvents()
	runRepo.mu.Lock()
	runRepo.latest = &failedRun
	runRepo.mu.Unlock()
	now := time.Now()
	claim, err := svc.claimEvaluationRun(context.Background(), a.ID().Uint64(), "retry-claim", "", now)
	if err != nil {
		t.Fatalf("claimEvaluationRun after recovery: %v", err)
	}
	nextRun := claim.Run
	if !claim.Claimed || nextRun.Attempt().Number != failedRun.Attempt().Number+1 || nextRun.Attempt().Status != evalrun.StatusRunning {
		t.Fatalf("next claim = %#v, want next running attempt", claim)
	}
}

func TestRetryableFailureRedeliveryClaimsNextAttemptAndCompletes(t *testing.T) {
	t.Parallel()

	a := splitPhaseAssessment(t)
	a.ClearEvents()
	var calls int
	evaluator := evaluatorStub{
		key: evaluation.ExecutionIdentityScaleDefault,
		execute: func(_ context.Context, input ExecutionInput) (*domainoutcome.Execution, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("temporary calculation failure")
			}
			return executionForAssessment(input.Assessment, "recovered"), nil
		},
	}
	runRepo := &stubRunRepo{}
	committer := &evaluationCommitterStub{}
	assessmentRepo := &fakeAssessmentRepo{assessment: a}
	svc := NewEngine(
		assessmentRepo,
		stubInputResolver{},
		withTestEvaluator(evaluator),
		WithRunRepository(runRepo),
		WithEvaluationCommitter(committer),
		WithTransactionalOutbox(&engineRecordingTxRunner{}, &engineRecordingEventStager{}),
	)

	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err == nil {
		t.Fatal("first Evaluate() error = nil, want retryable calculation failure")
	}
	if !a.Status().IsFailed() {
		t.Fatalf("assessment status after first attempt = %s, want failed", a.Status())
	}
	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("redelivered Evaluate(): %v", err)
	}
	if calls != 2 || committer.calls != 1 {
		t.Fatalf("evaluator calls=%d committer calls=%d, want 2/1", calls, committer.calls)
	}
	if committer.request.Run == nil || committer.request.Run.Attempt().Number != 2 || committer.request.Run.Attempt().Status != evalrun.StatusSucceeded {
		t.Fatalf("committed run = %#v, want succeeded attempt 2", committer.request.Run)
	}
	if assessmentRepo.assessment == nil || !assessmentRepo.assessment.Status().IsEvaluated() {
		t.Fatalf("assessment status = %v, want evaluated", assessmentRepo.assessment)
	}
	if len(assessmentRepo.assessment.Events()) != 0 {
		t.Fatalf("redelivery emitted duplicate requested events: %#v", assessmentRepo.assessment.Events())
	}
}

func TestFailedAssessmentReclaimsExpiredRunningAttempt(t *testing.T) {
	t.Parallel()

	a := splitPhaseAssessment(t)
	a.ClearEvents()
	if err := a.MarkAsFailed("worker crashed after failure state persisted"); err != nil {
		t.Fatal(err)
	}
	a.ClearEvents()
	run := evalrun.NewEvaluationRunWithAttempt(a.ID().Uint64(), 1)
	claimedAt := time.Now().Add(-2 * time.Minute)
	if err := run.Claim(evalrun.ClaimInput{Token: "expired-owner", ClaimedAt: claimedAt, LeaseExpiresAt: claimedAt.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	runRepo := &stubRunRepo{latest: &run}
	committer := &evaluationCommitterStub{}
	assessmentRepo := &fakeAssessmentRepo{assessment: a}
	evaluator := &countingEvaluator{key: evaluation.ExecutionIdentityScaleDefault, outcome: executionForAssessment(a, "reclaimed")}
	svc := NewEngine(
		assessmentRepo,
		stubInputResolver{},
		withTestEvaluator(evaluator),
		WithRunRepository(runRepo),
		WithEvaluationCommitter(committer),
		WithTransactionalOutbox(&engineRecordingTxRunner{}, &engineRecordingEventStager{}),
	)

	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatal(err)
	}
	if evaluator.calls != 1 || committer.request.Run == nil || committer.request.Run.Attempt().Number != 1 || committer.request.Run.Attempt().Status != evalrun.StatusSucceeded {
		t.Fatalf("reclaimed execution = calls:%d run:%#v", evaluator.calls, committer.request.Run)
	}
	if assessmentRepo.assessment == nil || !assessmentRepo.assessment.Status().IsEvaluated() {
		t.Fatalf("assessment after reclaim = %#v", assessmentRepo.assessment)
	}
}
