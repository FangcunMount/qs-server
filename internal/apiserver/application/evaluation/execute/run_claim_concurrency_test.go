package execute

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

type blockingEvaluator struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	calls   atomic.Int32
}

func (e *blockingEvaluator) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityScaleDefault
}
func (e *blockingEvaluator) Key() evaluation.ExecutionIdentity {
	return e.ExecutionIdentity()
}
func (e *blockingEvaluator) Execute(_ context.Context, input ExecutionInput) (*domainoutcome.Execution, error) {
	e.calls.Add(1)
	e.once.Do(func() { close(e.entered) })
	<-e.release
	return executionForAssessment(input.Assessment, "claimed"), nil
}

func TestEvaluateConcurrentWorkersExecuteEvaluatorOnce(t *testing.T) {
	a := splitPhaseAssessment(t)
	repo := &stubRunRepo{}
	evaluator := &blockingEvaluator{entered: make(chan struct{}), release: make(chan struct{})}
	capture := &splitPhaseCapture{}
	svc := newSplitPhaseTestService(
		&fakeAssessmentRepo{assessment: a},
		stubInputResolver{},
		capture,
		withTestEvaluator(evaluator),
		WithRunRepository(repo),
		WithRunLease(time.Minute),
	)

	firstDone := make(chan error, 1)
	go func() { firstDone <- svc.Evaluate(context.Background(), a.ID().Uint64()) }()
	select {
	case <-evaluator.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("first worker did not enter evaluator")
	}

	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("duplicate worker: %v", err)
	}
	if calls := evaluator.calls.Load(); calls != 1 {
		t.Fatalf("evaluator calls = %d, want 1", calls)
	}

	close(evaluator.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("claim owner: %v", err)
	}
	if calls := evaluator.calls.Load(); calls != 1 {
		t.Fatalf("evaluator calls after completion = %d, want 1", calls)
	}
}
