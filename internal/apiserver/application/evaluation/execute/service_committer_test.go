package execute

import (
	"context"
	"testing"
	"time"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcomecommit "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/commit"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

type evaluationCommitterStub struct {
	calls   int
	request outcomecommit.Request
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
