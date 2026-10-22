package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type blockingRunner struct {
	runnerLifecycle
	entered, release chan struct{}
	calls            atomic.Int32
}

func (*blockingRunner) Name() string { return "blocking" }
func (r *blockingRunner) Start(ctx context.Context) {
	r.launch(func() { r.calls.Add(1); close(r.entered); <-r.release })
}
func TestManagerWaitRequiresRealLoopCompletionAfterCancellation(t *testing.T) {
	r := &blockingRunner{entered: make(chan struct{}), release: make(chan struct{})}
	manager := NewManager(r)
	ctx, cancel := context.WithCancel(context.Background())
	manager.Start(ctx)
	select {
	case <-r.entered:
	case <-time.After(time.Second):
		t.Fatal("loop absent")
	}
	cancel()
	for i := 0; i < 8; i++ {
		manager.Start(ctx)
		if err := manager.Wait(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("false drain completion: %v", err)
		}
	}
	if r.calls.Load() != 1 {
		t.Fatal("duplicate loop created")
	}
	close(r.release)
	wait, cancelWait := context.WithTimeout(context.Background(), time.Second)
	defer cancelWait()
	if err := manager.Wait(wait); err != nil {
		t.Fatal(err)
	}
	if err := manager.Wait(ctx); err != nil {
		t.Fatal("finished loop lost to cancellation", err)
	}
}
func TestAllProductionRunnersExposeTheirRealCompletion(t *testing.T) {
	for _, r := range []Runner{&PlanRunner{}, &LeaseRecoveryRunner{}, &StatisticsSyncRunner{}, &AuthzRoleProjectionRunner{}, &EvaluationConsistencyAuditRunner{}, &ReportCatalogAuditRunner{}, &MongoConsistencyAuditRunner{}} {
		if err := NewManager(r).Wait(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDisabledTypedNilRunnersDoNotOwnShutdownWork(t *testing.T) {
	manager := NewManager((*PlanRunner)(nil), (*LeaseRecoveryRunner)(nil), (*StatisticsSyncRunner)(nil), (*AuthzRoleProjectionRunner)(nil), (*EvaluationConsistencyAuditRunner)(nil), (*ReportCatalogAuditRunner)(nil), (*MongoConsistencyAuditRunner)(nil))
	if manager.Len() != 0 {
		t.Fatal("disabled constructors became active runners")
	}
	manager.Start(context.Background())
	if err := manager.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}
