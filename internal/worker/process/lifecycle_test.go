package process

import (
	"errors"
	"reflect"
	"testing"

	bootstrap "github.com/FangcunMount/qs-server/internal/worker/bootstrap"
	observability "github.com/FangcunMount/qs-server/internal/worker/observability"
)

type workerFakeCloser struct{ closed bool }

func (c *workerFakeCloser) Close() error { c.closed = true; return nil }

func TestSubscriberFailureDoesNotCloseItsHeldEvidenceStore(t *testing.T) {
	want := errors.New("subscriber not drained")
	store, publisher := &workerFakeCloser{}, &workerFakeCloser{}
	deps := buildLifecycleDeps(resourceOutput{}, containerOutput{}, integrationOutput{}, runtimeOutput{messaging: messagingRuntimeOutput{
		subscriber: &workerFakeSubscriber{err: want}, holdStore: store, publisher: publisher,
	}})
	if err := deps.stopSubscriber(); !errors.Is(err, want) {
		t.Fatal("subscriber error lost", err)
	}
	if store.closed || publisher.closed {
		t.Fatal("dependencies closed before subscriber drained")
	}
}

type workerFakeSubscriber struct {
	order *[]string
	err   error
}

func (s *workerFakeSubscriber) Stop() {
	if s.order != nil {
		*s.order = append(*s.order, "subscriber-stop")
	}
}

func (s *workerFakeSubscriber) Close() error {
	if s.order != nil {
		*s.order = append(*s.order, "subscriber-close")
	}
	return s.err
}

func TestRunWorkerLifecycleRunsInExpectedOrder(t *testing.T) {
	t.Parallel()

	var order []string
	err := runWorkerLifecycle(lifecycleDeps{
		stopSubscriber: func() error {
			order = append(order, "subscriber")
			return nil
		},
		closeGRPCManager: func() error {
			order = append(order, "grpc")
			return nil
		},
		closeDatabase: func() error {
			order = append(order, "database")
			return nil
		},
		shutdownMetrics: func() error {
			order = append(order, "metrics")
			return nil
		},
		cleanupContainer: func() error {
			order = append(order, "container")
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"subscriber", "container", "grpc", "database", "metrics"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("lifecycle order = %#v, want %#v", order, want)
	}
}

func TestBuildWorkerLifecycleDepsUsesStageOutputs(t *testing.T) {
	t.Parallel()

	var order []string
	subscriber := &workerFakeSubscriber{order: &order}

	deps := buildLifecycleDeps(
		resourceOutput{handles: resourceHandles{dbManager: &bootstrap.DatabaseManager{}}},
		containerOutput{},
		integrationOutput{},
		runtimeOutput{
			messaging:     messagingRuntimeOutput{subscriber: subscriber},
			observability: observabilityOutput{metricsServer: &observability.MetricsServer{}},
		},
	)
	if deps.stopSubscriber == nil {
		t.Fatal("stopSubscriber = nil, want value")
	}
	if deps.closeDatabase == nil {
		t.Fatal("closeDatabase = nil, want value")
	}
	if deps.shutdownMetrics == nil {
		t.Fatal("shutdownMetrics = nil, want value")
	}
	if err := deps.stopSubscriber(); err != nil {
		t.Fatalf("stopSubscriber() error = %v", err)
	}
	want := []string{"subscriber-stop", "subscriber-close"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("subscriber shutdown order = %#v, want %#v", order, want)
	}
}
