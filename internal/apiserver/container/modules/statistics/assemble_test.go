package statistics_test

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gorm.io/gorm"
)

type lockRunnerStub struct{}

func (lockRunnerStub) Run(ctx context.Context, _ locklease.WorkloadID, _ string, _ time.Duration, body func(context.Context) error) (locklease.RunResult, error) {
	return locklease.RunResult{Acquired: true}, body(ctx)
}

func TestNewRequiresMySQLDB(t *testing.T) {
	t.Parallel()

	if _, err := statistics.New(statistics.Deps{}); err == nil {
		t.Fatal("New() error = nil, want missing MySQL error")
	}
}

func TestNewBuildsServicesWithoutExposingCache(t *testing.T) {
	t.Parallel()

	client, err := mongo.Connect(context.Background(), options.Client())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	module, err := statistics.New(statistics.Deps{MySQLDB: &gorm.DB{}, MongoDB: client.Database("test"), LockRunner: lockRunnerStub{}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if module.ReadService == nil || module.SyncService == nil {
		t.Fatalf("statistics services not initialized: %#v", module)
	}
}
