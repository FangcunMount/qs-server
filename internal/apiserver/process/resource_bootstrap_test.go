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
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/bootstrap"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventruntime"
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
	runtimeBundle := &cacheplanebootstrap.RuntimeBundle{Component: "apiserver"}
	subsystem := &cachebootstrap.Subsystem{}
	publisher := &fakePublisher{}
	catalog := eventcatalog.NewCatalog(nil)

	var backpressureConfigured bool
	var buildOptionsInput containerOptionsInput
	wantOptions := container.ContainerOptions{PlanEntryBaseURL: "https://entry.example"}
	backpressureOptions := container.BackpressureOptions{}

	got, err := prepareResources(resourceStageDeps{
		database: databaseResourceDeps{
			initialize: func() error { return nil },
			getMySQL:   func() (*gorm.DB, error) { return &mysqlDB, nil },
			getMongo:   func() (*mongo.Database, error) { return &mongoDB, nil },
		},
		redisRuntime: redisRuntimeStageDeps{
			getClient:    func() (redis.UniversalClient, error) { return redisClient, nil },
			buildRuntime: func() *cacheplanebootstrap.RuntimeBundle { return runtimeBundle },
			buildSubsystem: func(got *cacheplanebootstrap.RuntimeBundle) *cachebootstrap.Subsystem {
				if got != runtimeBundle {
					t.Fatalf("runtime bundle = %#v, want %#v", got, runtimeBundle)
				}
				return subsystem
			},
		},
		mqPublisher: mqPublisherStageDeps{
			fallbackMode: eventruntime.PublishModeLogging,
			enabled:      true,
			provider:     "stub",
			newPublisher: func() (messaging.Publisher, error) { return publisher, nil },
		},
		loadEventCatalog: func() (*eventcatalog.Catalog, error) { return catalog, nil },
		buildBackpressure: func() container.BackpressureOptions {
			backpressureConfigured = true
			return backpressureOptions
		},
		buildContainerOptions: func(output containerOptionsInput) container.ContainerOptions {
			buildOptionsInput = output
			return wantOptions
		},
	})
	if err != nil {
		t.Fatalf("prepareResources() error = %v", err)
	}

	if !backpressureConfigured {
		t.Fatal("buildBackpressure was not called")
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
	if got.cacheRuntime.redisRuntime != runtimeBundle {
		t.Fatalf("redis runtime = %#v, want %#v", got.cacheRuntime.redisRuntime, runtimeBundle)
	}
	if got.messaging.mqPublisher != publisher {
		t.Fatalf("mqPublisher = %#v, want %#v", got.messaging.mqPublisher, publisher)
	}
	if got.messaging.publishMode != eventruntime.PublishModeMQ {
		t.Fatalf("publishMode = %q, want %q", got.messaging.publishMode, eventruntime.PublishModeMQ)
	}
	if got.containerInput.containerOptions != wantOptions {
		t.Fatalf("containerOptions = %#v, want %#v", got.containerInput.containerOptions, wantOptions)
	}
	if buildOptionsInput.cacheSubsystem != subsystem || buildOptionsInput.mqPublisher != publisher || buildOptionsInput.eventCatalog != catalog || buildOptionsInput.backpressure != backpressureOptions {
		t.Fatalf("buildContainerOptions input mismatch: %#v", buildOptionsInput)
	}
}

func TestInitializeRedisRuntimeReturnsSubsystemWhenRedisUnavailable(t *testing.T) {
	subsystem := &cachebootstrap.Subsystem{}

	runtimeBundle := &cacheplanebootstrap.RuntimeBundle{Component: "apiserver"}
	client, gotRuntime, gotSubsystem := initializeRedisRuntime(redisRuntimeStageDeps{
		getClient: func() (redis.UniversalClient, error) { return nil, errors.New("redis unavailable") },
		buildRuntime: func() *cacheplanebootstrap.RuntimeBundle {
			return runtimeBundle
		},
		buildSubsystem: func(got *cacheplanebootstrap.RuntimeBundle) *cachebootstrap.Subsystem {
			if got != runtimeBundle {
				t.Fatalf("runtime bundle = %#v, want %#v", got, runtimeBundle)
			}
			return subsystem
		},
	})
	if client != nil {
		t.Fatalf("redis client = %#v, want nil when redis is unavailable", client)
	}
	if gotRuntime != runtimeBundle {
		t.Fatalf("redis runtime = %#v, want %#v", gotRuntime, runtimeBundle)
	}
	if gotSubsystem != subsystem {
		t.Fatalf("cache subsystem = %#v, want %#v", gotSubsystem, subsystem)
	}
}

func TestCreateMQPublisherFallsBackToLoggingModeOnPublisherError(t *testing.T) {
	publisher, mode := createMQPublisher(mqPublisherStageDeps{
		fallbackMode: eventruntime.PublishModeLogging,
		enabled:      true,
		provider:     "unsupported",
		newPublisher: func() (messaging.Publisher, error) { return nil, errors.New("boom") },
	})

	if publisher != nil {
		t.Fatalf("publisher = %#v, want nil on fallback", publisher)
	}
	if mode != eventruntime.PublishModeLogging {
		t.Fatalf("publish mode = %q, want %q", mode, eventruntime.PublishModeLogging)
	}
}

func TestAPIServerBuildResourceStageDepsWithoutConfigOmitsConfigBoundBuilders(t *testing.T) {
	deps := (&server{}).buildResourceStageDeps()

	if deps.buildBackpressure != nil {
		t.Fatal("buildBackpressure != nil, want nil")
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

	if deps.buildBackpressure == nil {
		t.Fatal("buildBackpressure = nil, want callback")
	}
	if deps.buildContainerOptions == nil {
		t.Fatal("buildContainerOptions = nil, want builder")
	}
}
