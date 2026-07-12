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
	if got := runRepo.saved[0].InputSnapshotRef(); got != "model:SCALE-1@1.0.0" {
		t.Fatalf("saved input_snapshot_ref = %q, want model:SCALE-1@1.0.0", got)
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
	if got := runRepo.saved[len(runRepo.saved)-1].InputSnapshotRef(); got != "model:SCALE-1@1.0.0" {
		t.Fatalf("last saved input_snapshot_ref = %q", got)
	}
	if got := runRepo.saved[len(runRepo.saved)-1].Attempt().Status; got != evalrun.StatusRunning {
		t.Fatalf("last saved status = %s, want running", got)
	}
}
