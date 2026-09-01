package process

import (
	"reflect"
	"testing"

	bootstrap "github.com/FangcunMount/qs-server/internal/collection-server/bootstrap"
	genericapiserver "github.com/FangcunMount/qs-server/internal/pkg/server"
)

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

	want := []string{"http", "grpc", "database", "iam", "container"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("lifecycle order = %#v, want %#v", order, want)
	}
}

func TestBuildCollectionLifecycleDepsUsesStageOutputs(t *testing.T) {
	t.Parallel()

	deps := buildLifecycleDeps(
		resourceOutput{handles: resourceHandles{dbManager: &bootstrap.DatabaseManager{}}},
		containerOutput{},
		integrationOutput{},
		transportOutput{httpServer: &genericapiserver.GenericAPIServer{}},
	)
	if deps.closeDatabase == nil {
		t.Fatal("closeDatabase = nil, want value")
	}
	if deps.closeHTTP == nil {
		t.Fatal("closeHTTP = nil, want value")
	}
}
