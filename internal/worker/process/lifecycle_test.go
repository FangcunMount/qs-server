package process

import (
	"reflect"
	"testing"

	"github.com/FangcunMount/component-base/pkg/messaging"
	bootstrap "github.com/FangcunMount/qs-server/internal/worker/bootstrap"
	observability "github.com/FangcunMount/qs-server/internal/worker/observability"
)

type workerFakeSubscriber struct {
	order *[]string
}

func (s *workerFakeSubscriber) Subscribe(string, string, messaging.Handler) error { return nil }

func (s *workerFakeSubscriber) SubscribeWithMiddleware(string, string, messaging.Handler, ...messaging.Middleware) error {
	return nil
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
	return nil
}

func TestRunWorkerLifecycleRunsInExpectedOrder(t *testing.T) {
	t.Parallel()

	var order []string
	runWorkerLifecycle(lifecycleDeps{
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

	want := []string{"subscriber", "grpc", "database", "metrics", "container"}
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
