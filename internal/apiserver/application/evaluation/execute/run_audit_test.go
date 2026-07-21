package execute

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/component-base/pkg/log"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type modelInputResolver struct {
	model *evaluationinput.ModelSnapshot
}

func (r modelInputResolver) Resolve(_ context.Context, _ evaluationinput.InputRef) (*evaluationinput.InputSnapshot, error) {
	return &evaluationinput.InputSnapshot{Model: r.model}, nil
}

func scaleEvaluatorForAssessment(a *domainAssessment.Assessment) *countingEvaluator {
	return &countingEvaluator{
		key:     evaluation.ExecutionIdentityScaleDefault,
		outcome: executionForAssessment(a, "ok"),
	}
}

func TestEvaluatePersistsTraceIDFromContext(t *testing.T) {
	t.Parallel()

	a := splitPhaseAssessment(t)
	evaluator := scaleEvaluatorForAssessment(a)
	runRepo := &stubRunRepo{}
	svc := newSplitPhaseTestService(
		&fakeAssessmentRepo{assessment: a},
		modelInputResolver{model: &evaluationinput.ModelSnapshot{Code: "SCALE-1", Version: "1.0.0"}},
		&splitPhaseCapture{},
		withTestEvaluator(evaluator),
		WithRunRepository(runRepo),
	)

	ctx := log.WithTraceID(context.Background(), "trace-abc-123")
	if err := svc.Evaluate(ctx, a.ID().Uint64()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(runRepo.saved) != 1 {
		t.Fatalf("saved runs = %d, want input snapshot update", len(runRepo.saved))
	}
	if got := runRepo.saved[0].TraceID(); got != "trace-abc-123" {
		t.Fatalf("first saved trace_id = %q, want trace-abc-123", got)
	}
}

func TestEvaluatePersistsInputSnapshotRefBeforeExecuting(t *testing.T) {
	t.Parallel()

	a := splitPhaseAssessment(t)
	evaluator := scaleEvaluatorForAssessment(a)
	runRepo := &stubRunRepo{}
	svc := newSplitPhaseTestService(
		&fakeAssessmentRepo{assessment: a},
		modelInputResolver{model: &evaluationinput.ModelSnapshot{Code: "SCALE-1", Version: "1.0.0"}},
		&splitPhaseCapture{},
		withTestEvaluator(evaluator),
		WithRunRepository(runRepo),
	)

	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if evaluator.calls != 1 {
		t.Fatalf("evaluator calls = %d, want 1", evaluator.calls)
	}
	if len(runRepo.saved) != 1 {
		t.Fatalf("saved runs = %d, want one input snapshot update", len(runRepo.saved))
	}
	if got := runRepo.saved[0].InputSnapshotRef(); !evaluationinput.IsIdentityRef(got) {
		t.Fatalf("saved input_snapshot_ref = %q, want isn:v1 identity ref", got)
	}
}

func TestEvaluateRejectsInputSnapshotDriftAcrossRetries(t *testing.T) {
	t.Parallel()

	a := splitPhaseAssessment(t)
	if err := a.MarkAsFailed("first attempt failed"); err != nil {
		t.Fatal(err)
	}
	previous := evalrun.Reconstruct(evalrun.ReconstructInput{
		RunID:            evalrun.ID("7001:1"),
		AssessmentID:     a.ID().Uint64(),
		Attempt:          evalrun.Attempt{Number: 1, Status: evalrun.StatusFailed},
		Failure:          &evalrun.Failure{Kind: evalrun.FailureKindCalculation, Retryable: true},
		InputSnapshotRef: "isn:v1:previous-attempt-digest",
	})
	runRepo := &stubRunRepo{latest: &previous}
	evaluator := scaleEvaluatorForAssessment(a)
	svc := newSplitPhaseTestService(
		&fakeAssessmentRepo{assessment: a},
		modelInputResolver{model: &evaluationinput.ModelSnapshot{Code: "SCALE-1", Version: "1.0.0"}},
		&splitPhaseCapture{},
		withTestEvaluator(evaluator),
		WithRunRepository(runRepo),
	)

	err := svc.Evaluate(context.Background(), a.ID().Uint64())
	if err == nil {
		t.Fatal("expected input snapshot drift error")
	}
	if evaluator.calls != 0 {
		t.Fatalf("evaluator calls = %d, want 0 after drift rejection", evaluator.calls)
	}
	last := runRepo.saved[len(runRepo.saved)-1]
	if last.Attempt().Status != evalrun.StatusFailed {
		t.Fatalf("last saved status = %s, want failed", last.Attempt().Status)
	}
	failure := last.Failure()
	if failure == nil || failure.Kind != evalrun.FailureKindValidation || failure.Retryable {
		t.Fatalf("failure = %#v, want terminal validation failure", failure)
	}
}

func TestEvaluateReturnsInputSnapshotPersistenceErrorBeforeExecuting(t *testing.T) {
	t.Parallel()

	persistErr := errors.New("input snapshot ref save failed")
	a := splitPhaseAssessment(t)
	evaluator := scaleEvaluatorForAssessment(a)
	runRepo := &stubRunRepo{saveErrs: []error{persistErr}}
	svc := newSplitPhaseTestService(
		&fakeAssessmentRepo{assessment: a},
		modelInputResolver{model: &evaluationinput.ModelSnapshot{Code: "SCALE-1", Version: "1.0.0"}},
		&splitPhaseCapture{},
		withTestEvaluator(evaluator),
		WithRunRepository(runRepo),
	)

	err := svc.Evaluate(context.Background(), a.ID().Uint64())
	if !errors.Is(err, persistErr) {
		t.Fatalf("Evaluate error = %v, want input snapshot persistence error", err)
	}
	if evaluator.calls != 0 {
		t.Fatalf("evaluator calls = %d, want 0 after input snapshot persist failure", evaluator.calls)
	}
	if len(runRepo.saved) != 1 {
		t.Fatalf("saved runs = %d, want failed input snapshot update", len(runRepo.saved))
	}
	if got := runRepo.saved[len(runRepo.saved)-1].InputSnapshotRef(); !evaluationinput.IsIdentityRef(got) {
		t.Fatalf("last saved input_snapshot_ref = %q, want isn:v1 identity ref", got)
	}
	if got := runRepo.saved[len(runRepo.saved)-1].Attempt().Status; got != evalrun.StatusRunning {
		t.Fatalf("last saved status = %s, want running", got)
	}
}
