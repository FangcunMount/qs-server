package process

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/component-base/pkg/processruntime"
	"github.com/FangcunMount/component-base/pkg/shutdown"
	bootstrap "github.com/FangcunMount/qs-server/internal/apiserver/bootstrap"
	grpcpkg "github.com/FangcunMount/qs-server/internal/pkg/grpc"
	genericapiserver "github.com/FangcunMount/qs-server/internal/pkg/server"
)

func TestPrepareRunLifecycleRunsHooksInRegistrationOrder(t *testing.T) {
	t.Parallel()

	var order []string
	lifecycle := processruntime.Lifecycle{}
	lifecycle.AddShutdownHook("first", func() error {
		order = append(order, "first")
		return nil
	})
	lifecycle.AddShutdownHook("second", func() error {
		order = append(order, "second")
		return nil
	})

	runPrepareRunShutdownHooks(lifecycle)

	want := []string{"first", "second"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("shutdown hook order = %#v, want %#v", order, want)
	}
}

func TestPrepareRunLifecycleContinuesAfterHookError(t *testing.T) {
	t.Parallel()

	var order []string
	lifecycle := processruntime.Lifecycle{}
	lifecycle.AddShutdownHook("first", func() error {
		order = append(order, "first")
		return errors.New("boom")
	})
	lifecycle.AddShutdownHook("second", func() error {
		order = append(order, "second")
		return nil
	})

	runPrepareRunShutdownHooks(lifecycle)

	want := []string{"first", "second"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("shutdown hook order = %#v, want %#v", order, want)
	}
}

func TestRunProcessLifecycleDepsRunsInExpectedOrder(t *testing.T) {
	t.Parallel()

	var order []string
	runProcessLifecycleDeps(processLifecycleDeps{
		container: containerLifecycleDeps{
			containerCleanup: func() error {
				order = append(order, "container")
				return nil
			},
			stopAuthzSync: func() error {
				order = append(order, "authz")
				return nil
			},
		},
		resource: resourceLifecycleDeps{
			closeDatabase: func() error {
				order = append(order, "database")
				return nil
			},
		},
		transport: transportLifecycleDeps{
			closeHTTP: func() {
				order = append(order, "http")
			},
			closeGRPC: func() {
				order = append(order, "grpc")
			},
		},
	})

	want := []string{"container", "authz", "database", "http", "grpc"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("lifecycle dep order = %#v, want %#v", order, want)
	}
}

type fakeSubscriber struct {
	order *[]string
}

func (s *fakeSubscriber) Subscribe(string, string, messaging.Handler) error { return nil }

func (s *fakeSubscriber) SubscribeWithMiddleware(string, string, messaging.Handler, ...messaging.Middleware) error {
	return nil
}

func (s *fakeSubscriber) Stop() {
	if s.order != nil {
		*s.order = append(*s.order, "authz-stop")
	}
}

func (s *fakeSubscriber) Close() error {
	if s.order != nil {
		*s.order = append(*s.order, "authz-close")
	}
	return nil
}

func TestBuildProcessLifecycleDepsUsesStageOutputs(t *testing.T) {
	t.Parallel()

	var order []string
	subscriber := &fakeSubscriber{order: &order}

	deps := buildLifecycleDeps(
		resourceOutput{
			handles: resourceHandles{dbManager: &bootstrap.DatabaseManager{}},
		},
		containerOutput{},
		integrationOutput{
			authzVersionSubscriber: subscriber,
		},
		transportOutput{
			httpServer: &genericapiserver.GenericAPIServer{},
			grpcServer: &grpcpkg.Server{},
		},
		runtimeOutput{},
	)

	if deps.resource.closeDatabase == nil {
		t.Fatal("closeDatabase = nil, want value")
	}
	if deps.container.stopAuthzSync == nil {
		t.Fatal("stopAuthzSync = nil, want value")
	}
	if deps.transport.closeHTTP == nil {
		t.Fatal("closeHTTP = nil, want value")
	}
	if deps.transport.closeGRPC == nil {
		t.Fatal("closeGRPC = nil, want value")
	}

	if err := deps.container.stopAuthzSync(); err != nil {
		t.Fatalf("stopAuthzSync() error = %v", err)
	}

	want := []string{"authz-stop", "authz-close"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("authz shutdown order = %#v, want %#v", order, want)
	}
}

func TestRunPreparedServerStartsShutdownBeforeServing(t *testing.T) {
	t.Parallel()

	order := make(chan string, 3)
	httpDone := make(chan struct{})
	grpcDone := make(chan struct{})
	got := runPreparedServer(preparedServerRunDeps{
		startShutdown: func() error {
			order <- "shutdown"
			return nil
		},
		transports: preparedServerTransports{
			runHTTP: func() error {
				defer close(httpDone)
				order <- "http"
				return errors.New("http boom")
			},
			runGRPC: func() error {
				defer close(grpcDone)
				order <- "grpc"
				return nil
			},
		},
	})

	if got == nil || got.Error() != "http boom" {
		t.Fatalf("runPreparedServer() error = %v, want http boom", got)
	}

	waitDone := func(name string, done <-chan struct{}) {
		t.Helper()
		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("%s service did not finish", name)
		}
	}

	waitDone("http", httpDone)
	waitDone("grpc", grpcDone)

	first := <-order
	if first != "shutdown" {
		t.Fatalf("first event = %q, want shutdown", first)
	}
}

func TestPreparedAPIServerBuildRunDepsUsesPreparedFields(t *testing.T) {
	t.Parallel()

	prepared := preparedServer{
		startShutdown: shutdown.New().Start,
		grpcServer:    &grpcpkg.Server{},
	}

	deps := prepared.buildPreparedServerRunDeps()
	if deps.startShutdown == nil {
		t.Fatal("startShutdown = nil, want value")
	}
	if deps.transports.runGRPC == nil {
		t.Fatal("runGRPC = nil, want value")
	}
}
