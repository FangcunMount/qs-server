package process

import (
	"reflect"
	"testing"

	"github.com/FangcunMount/component-base/pkg/messaging"
	bootstrap "github.com/FangcunMount/qs-server/internal/collection-server/bootstrap"
	genericapiserver "github.com/FangcunMount/qs-server/internal/pkg/server"
)

type collectionFakeSubscriber struct {
	order *[]string
}

func (s *collectionFakeSubscriber) Subscribe(string, string, messaging.Handler) error { return nil }

func (s *collectionFakeSubscriber) SubscribeWithMiddleware(string, string, messaging.Handler, ...messaging.Middleware) error {
	return nil
}

func (s *collectionFakeSubscriber) Stop() {
	if s.order != nil {
		*s.order = append(*s.order, "authz-stop")
	}
}

func (s *collectionFakeSubscriber) Close() error {
	if s.order != nil {
		*s.order = append(*s.order, "authz-close")
	}
	return nil
}

func TestRunCollectionLifecycleRunsInExpectedOrder(t *testing.T) {
	t.Parallel()

	var order []string
	runCollectionLifecycle(lifecycleDeps{
		closeGRPCManager: func() error {
			order = append(order, "grpc")
			return nil
		},
		closeDatabase: func() error {
			order = append(order, "database")
			return nil
		},
		stopAuthzSync: func() error {
			order = append(order, "authz")
			return nil
		},
		closeIAM: func() error {
			order = append(order, "iam")
			return nil
		},
		cleanupContainer: func() error {
			order = append(order, "container")
			return nil
		},
		closeHTTP: func() {
			order = append(order, "http")
		},
	})

	want := []string{"grpc", "database", "authz", "iam", "container", "http"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("lifecycle order = %#v, want %#v", order, want)
	}
}

func TestBuildCollectionLifecycleDepsUsesStageOutputs(t *testing.T) {
	t.Parallel()

	var order []string
	subscriber := &collectionFakeSubscriber{order: &order}

	deps := buildLifecycleDeps(
		resourceOutput{handles: resourceHandles{dbManager: &bootstrap.DatabaseManager{}}},
		containerOutput{},
		integrationOutput{iamSync: iamSyncOutput{authzVersionSubscriber: subscriber}},
		transportOutput{httpServer: &genericapiserver.GenericAPIServer{}},
	)
	if deps.closeDatabase == nil {
		t.Fatal("closeDatabase = nil, want value")
	}
	if deps.stopAuthzSync == nil {
		t.Fatal("stopAuthzSync = nil, want value")
	}
	if deps.closeHTTP == nil {
		t.Fatal("closeHTTP = nil, want value")
	}

	if err := deps.stopAuthzSync(); err != nil {
		t.Fatalf("stopAuthzSync() error = %v", err)
	}
	want := []string{"authz-stop", "authz-close"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("authz shutdown order = %#v, want %#v", order, want)
	}
}
