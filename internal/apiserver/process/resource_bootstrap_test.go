package process

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachebootstrap"
	apiserverconfig "github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

type fakePublisher struct{}

func (*fakePublisher) Publish(_ context.Context, _ string, _ []byte) error { return nil }

func (*fakePublisher) PublishMessage(_ context.Context, _ string, _ *messaging.Message) error {
	return nil
}

func (*fakePublisher) Close() error { return nil }

func TestPrepareResourcesBuildsStageOutputFromDeps(t *testing.T) {
	var mysqlDB gorm.DB
	var mongoDB mongo.Database
	var redisClient redis.UniversalClient
	subsystem := &cachebootstrap.Subsystem{}
	publisher := &fakePublisher{}

	var backpressureConfigured bool
	var buildOptionsInput containerOptionsInput
	wantOptions := container.ContainerOptions{PlanEntryBaseURL: "https://entry.example"}

	got, err := prepareResources(resourceStageDeps{
		database: databaseResourceDeps{
			initialize: func() error { return nil },
			getMySQL:   func() (*gorm.DB, error) { return &mysqlDB, nil },
			getMongo:   func() (*mongo.Database, error) { return &mongoDB, nil },
		},
		redisRuntime: redisRuntimeStageDeps{
			getClient:      func() (redis.UniversalClient, error) { return redisClient, nil },
			buildSubsystem: func() *cachebootstrap.Subsystem { return subsystem },
		},
		mqPublisher: mqPublisherStageDeps{
			fallbackMode: eventconfig.PublishModeLogging,
			enabled:      true,
			provider:     "stub",
			newPublisher: func() (messaging.Publisher, error) { return publisher, nil },
		},
		applyBackpressure: func() { backpressureConfigured = true },
		buildContainerOptions: func(output containerOptionsInput) container.ContainerOptions {
			buildOptionsInput = output
			return wantOptions
		},
	})
	if err != nil {
		t.Fatalf("prepareResources() error = %v", err)
	}

	if !backpressureConfigured {
		t.Fatal("applyBackpressure was not called")
	}
	if got.handles.mysqlDB != &mysqlDB || got.handles.mongoDB != &mongoDB {
		t.Fatalf("database output mismatch: %+v", got)
	}
	if got.handles.redisCache != redisClient {
		t.Fatalf("redisCache = %#v, want %#v", got.handles.redisCache, redisClient)
	}
	if got.cacheRuntime.cacheSubsystem != subsystem {
		t.Fatalf("cacheSubsystem = %#v, want %#v", got.cacheRuntime.cacheSubsystem, subsystem)
	}
	if got.messaging.mqPublisher != publisher {
		t.Fatalf("mqPublisher = %#v, want %#v", got.messaging.mqPublisher, publisher)
	}
	if got.messaging.publishMode != eventconfig.PublishModeMQ {
		t.Fatalf("publishMode = %q, want %q", got.messaging.publishMode, eventconfig.PublishModeMQ)
	}
	if got.containerInput.containerOptions != wantOptions {
		t.Fatalf("containerOptions = %#v, want %#v", got.containerInput.containerOptions, wantOptions)
	}
	if buildOptionsInput.cacheSubsystem != subsystem || buildOptionsInput.mqPublisher != publisher {
		t.Fatalf("buildContainerOptions input mismatch: %#v", buildOptionsInput)
	}
}

func TestInitializeRedisRuntimeReturnsSubsystemWhenRedisUnavailable(t *testing.T) {
	subsystem := &cachebootstrap.Subsystem{}

	client, gotSubsystem := initializeRedisRuntime(redisRuntimeStageDeps{
		getClient:      func() (redis.UniversalClient, error) { return nil, errors.New("redis unavailable") },
		buildSubsystem: func() *cachebootstrap.Subsystem { return subsystem },
	})
	if client != nil {
		t.Fatalf("redis client = %#v, want nil when redis is unavailable", client)
	}
	if gotSubsystem != subsystem {
		t.Fatalf("cache subsystem = %#v, want %#v", gotSubsystem, subsystem)
	}
}

func TestCreateMQPublisherFallsBackToLoggingModeOnPublisherError(t *testing.T) {
	publisher, mode := createMQPublisher(mqPublisherStageDeps{
		fallbackMode: eventconfig.PublishModeLogging,
		enabled:      true,
		provider:     "unsupported",
		newPublisher: func() (messaging.Publisher, error) { return nil, errors.New("boom") },
	})

	if publisher != nil {
		t.Fatalf("publisher = %#v, want nil on fallback", publisher)
	}
	if mode != eventconfig.PublishModeLogging {
		t.Fatalf("publish mode = %q, want %q", mode, eventconfig.PublishModeLogging)
	}
}

func TestAPIServerBuildResourceStageDepsWithoutConfigOmitsConfigBoundBuilders(t *testing.T) {
	deps := (&server{}).buildResourceStageDeps()

	if deps.applyBackpressure != nil {
		t.Fatal("applyBackpressure != nil, want nil")
	}
	if deps.buildContainerOptions != nil {
		t.Fatal("buildContainerOptions != nil, want nil")
	}
}

func TestAPIServerBuildResourceStageDepsWithConfigIncludesConfigBoundBuilders(t *testing.T) {
	cfg, err := apiserverconfig.CreateConfigFromOptions(apiserveroptions.NewOptions())
	if err != nil {
		t.Fatalf("CreateConfigFromOptions() error = %v", err)
	}

	deps := (&server{config: cfg}).buildResourceStageDeps()

	if deps.applyBackpressure == nil {
		t.Fatal("applyBackpressure = nil, want callback")
	}
	if deps.buildContainerOptions == nil {
		t.Fatal("buildContainerOptions = nil, want builder")
	}
}
