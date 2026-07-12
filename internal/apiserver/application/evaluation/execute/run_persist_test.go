package execute

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
)

type stubRunRepo struct {
	mu                 sync.Mutex
	latest             *evalrun.EvaluationRun
	saved              []evalrun.EvaluationRun
	saveErr            error
	saveErrs           []error
	saveCtxHadTxMarker bool
}

func (r *stubRunRepo) Claim(_ context.Context, request evaluationrun.ClaimRequest) (evaluationrun.ClaimResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := evalrun.NewEvaluationRun(request.AssessmentID)
	if r.latest != nil {
		run = *r.latest
		switch run.Attempt.Status {
		case evalrun.StatusRunning:
			if run.HasActiveLease(request.ClaimedAt) {
				return evaluationrun.ClaimResult{Run: run}, nil
			}
		case evalrun.StatusFailed:
			if !run.Retryable() {
				return evaluationrun.ClaimResult{Run: run}, nil
			}
			run = evalrun.NextEvaluationRun(run)
		case evalrun.StatusSucceeded:
			return evaluationrun.ClaimResult{Run: run}, nil
		}
	}
	run.TraceID = request.TraceID
	if err := run.Claim(request.Token, request.ClaimedAt, request.LeaseUntil); err != nil {
		return evaluationrun.ClaimResult{}, err
	}
	copy := run
	r.latest = &copy
	return evaluationrun.ClaimResult{Run: run, Claimed: true}, nil
}

func (r *stubRunRepo) SaveClaimed(ctx context.Context, run evalrun.EvaluationRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saveCtxHadTxMarker, _ = ctx.Value(engineTxCtxMarker{}).(bool)
	r.saved = append(r.saved, run)
	if len(r.saveErrs) > 0 {
		err := r.saveErrs[0]
		r.saveErrs = r.saveErrs[1:]
		return err
	}
	if r.saveErr != nil {
		return r.saveErr
	}
	copy := run
	r.latest = &copy
	return nil
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

	now := time.Now()
	claim, err := svc.claimEvaluationRun(context.Background(), 99, "claim-2", "", now)
	if err != nil {
		t.Fatal(err)
	}
	run := claim.Run
	if !claim.Claimed || run.Attempt.Number != 2 {
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
	capture := &splitPhaseCapture{}
	svc := newSplitPhaseTestService(
		&fakeAssessmentRepo{assessment: a},
		stubInputResolver{},
		capture,
		withTestEvaluator(evaluator),
		WithRunRepository(&stubRunRepo{saveErr: persistErr}),
	)

	err := svc.Evaluate(context.Background(), a.ID().Uint64())
	if !errors.Is(err, persistErr) {
		t.Fatalf("Evaluate error = %v, want run persistence error", err)
	}
	if evaluator.calls != 0 {
		t.Fatalf("evaluator calls = %d, want 0 after start run persist failure", evaluator.calls)
	}
	if capture.CommitCalls != 0 {
		t.Fatalf("committer calls = %d, want none", capture.CommitCalls)
	}
}

func TestEvaluateReturnsOriginalExecutionErrorWhenFailedRunPersists(t *testing.T) {
	t.Parallel()

	executeErr := errors.New("calculator failed")
	a := splitPhaseAssessment(t)
	evaluator := evaluatorStub{
		key: evaluation.ExecutionIdentityScaleDefault,
		execute: func(context.Context, ExecutionInput) (*domainoutcome.Execution, error) {
			return nil, executeErr
		},
	}
	runRepo := &stubRunRepo{}
	svc := NewEngine(
		&fakeAssessmentRepo{assessment: a},
		stubInputResolver{},
		withTestEvaluator(evaluator),
		WithRunRepository(runRepo),
		WithTransactionalOutbox(&engineRecordingTxRunner{}, &engineRecordingEventStager{}),
	).(*service)

	err := svc.Evaluate(context.Background(), a.ID().Uint64())
	if !errors.Is(err, executeErr) {
		t.Fatalf("Evaluate error = %v, want original execution error", err)
	}
	if len(runRepo.saved) != 2 {
		t.Fatalf("saved runs = %d, want input snapshot and terminal failed run", len(runRepo.saved))
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
	evaluator := evaluatorStub{
		key: evaluation.ExecutionIdentityScaleDefault,
		execute: func(context.Context, ExecutionInput) (*domainoutcome.Execution, error) {
			return nil, executeErr
		},
	}
	runRepo := &stubRunRepo{saveErrs: []error{nil, persistErr}}
	svc := NewEngine(
		&fakeAssessmentRepo{assessment: a},
		stubInputResolver{},
		withTestEvaluator(evaluator),
		WithRunRepository(runRepo),
		WithTransactionalOutbox(&engineRecordingTxRunner{}, &engineRecordingEventStager{}),
	).(*service)

	err := svc.Evaluate(context.Background(), a.ID().Uint64())
	if !errors.Is(err, persistErr) {
		t.Fatalf("Evaluate error = %v, want failed run persistence error", err)
	}
	if len(runRepo.saved) != 2 {
		t.Fatalf("saved runs = %d, want input snapshot and attempted failed run save", len(runRepo.saved))
	}
	if got := runRepo.saved[len(runRepo.saved)-1].Attempt.Status; got != evalrun.StatusFailed {
		t.Fatalf("last run status = %s, want failed", got)
	}
	if !a.Status().IsSubmitted() {
		t.Fatalf("assessment status = %s, want submitted when failure transaction does not commit", a.Status())
	}
	if runRepo.latest == nil || runRepo.latest.Attempt.Status != evalrun.StatusRunning {
		t.Fatalf("caller/latest run = %#v, want running when failure transaction does not commit", runRepo.latest)
	}
}

func TestPersistClaimedEvaluationRunUpdatesOnlyRunRepository(t *testing.T) {
	t.Parallel()

	a := splitPhaseAssessment(t)
	run := evalrun.NewEvaluationRunWithAttempt(a.ID().Uint64(), 1)
	if err := run.Start(time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	repo := &fakeAssessmentRepo{assessment: a}
	runRepo := &stubRunRepo{}
	svc := &service{
		assessmentRepo: repo,
		runRepo:        runRepo,
		txRunner:       &engineRecordingTxRunner{},
	}

	if err := svc.persistClaimedEvaluationRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if repo.saveCtxHadTxMarker || runRepo.saveCtxHadTxMarker {
		t.Fatalf("run update must not write Assessment or require a transaction: assessment=%v run=%v", repo.saveCtxHadTxMarker, runRepo.saveCtxHadTxMarker)
	}
	if len(runRepo.saved) != 1 || runRepo.saved[0].Attempt.Status != evalrun.StatusRunning {
		t.Fatalf("saved run = %#v, want one running run", runRepo.saved)
	}
}

func TestClaimEvaluationRunSkipsActiveAndTerminalAttempts(t *testing.T) {
	t.Parallel()

	running := evalrun.NewEvaluationRunWithAttempt(99, 3)
	if err := running.Start(time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	running.ClaimToken = "owner"
	running.LeaseExpiresAt = ptrTime(now.Add(time.Minute))
	claim, err := (&service{runRepo: &stubRunRepo{latest: &running}}).claimEvaluationRun(context.Background(), 99, "other", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if claim.Claimed || claim.Run.RunID != running.RunID {
		t.Fatalf("claim = %#v, want duplicate skip", claim)
	}

	nonRetryable := evalrun.NewEvaluationRunWithAttempt(99, 4)
	if err := nonRetryable.Start(time.Unix(200, 0)); err != nil {
		t.Fatal(err)
	}
	if err := nonRetryable.Fail(time.Unix(201, 0), evalrun.Failure{Kind: evalrun.FailureKindValidation, Message: "invalid input"}); err != nil {
		t.Fatal(err)
	}
	claim, err = (&service{runRepo: &stubRunRepo{latest: &nonRetryable}}).claimEvaluationRun(context.Background(), 99, "claim", "", now)
	if err != nil || claim.Claimed {
		t.Fatalf("claim = %#v, err=%v; want terminal duplicate skip", claim, err)
	}
}

func ptrTime(value time.Time) *time.Time { return &value }
