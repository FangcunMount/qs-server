package process

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/subsystem"
	apiserverconfig "github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	resiliencesubsystem "github.com/FangcunMount/qs-server/internal/apiserver/resilience/subsystem"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	cacheplanebootstrap "github.com/FangcunMount/qs-server/internal/pkg/redisruntime/bootstrap"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

type fakePublisher struct{ onClose func() }

type fakeWirePublisher struct{ onClose func() }

func (*fakeWirePublisher) PublishWire(context.Context, string, []byte) error { return nil }
func (p *fakeWirePublisher) Close() error {
	if p.onClose != nil {
		p.onClose()
	}
	return nil
}

func (*fakePublisher) PublishWire(context.Context, string, []byte) error { return nil }

func TestAPISelectsNativeNSQWireFactory(t *testing.T) {
	cfg := &apiserverconfig.Config{Options: apiserveroptions.NewOptions()}
	cfg.MessagingOptions.Enabled = true
	cfg.MessagingOptions.Provider = "nsq"
	deps := (&server{config: cfg}).buildMQPublisherDeps()
	if deps.newWirePublisher == nil {
		t.Fatalf("NSQ should use the native wire factory: %+v", deps)
	}
	cfg.MessagingOptions.Provider = "rabbitmq"
	deps = (&server{config: cfg}).buildMQPublisherDeps()
	if deps.newWirePublisher != nil {
		t.Fatalf("retired RabbitMQ provider still has a publisher factory: %+v", deps)
	}
	if _, err := prepareResources(resourceStageDeps{mqPublisher: deps}); err == nil {
		t.Fatal("retired provider reached a usable publisher")
	}
}

func TestPrepareResourcesPassesNativeNSQWirePortAndClosesOnFailure(t *testing.T) {
	wire := &fakeWirePublisher{}
	var options eventsubsystem.Options
	got, err := prepareResources(resourceStageDeps{
		mqPublisher: mqPublisherStageDeps{
			enabled: true, provider: "nsq",
			newWirePublisher: func() (wirePublisherResource, error) { return wire, nil },
		},
		loadEventCatalog: func() (*eventcatalog.Catalog, error) { return eventcatalog.NewCatalog(nil), nil },
		eventSubsystem: eventSubsystemResourceDeps{
			newSubsystem: func(input eventsubsystem.Options) (*eventsubsystem.Subsystem, error) {
				options = input
				return &eventsubsystem.Subsystem{}, nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.messaging.wirePublisher != wire ||
		got.messaging.closePublisher == nil || got.messaging.publishMode != eventruntime.PublishModeMQ ||
		options.WirePublisher != wire {
		t.Fatalf("native NSQ publisher was not passed directly: messaging=%+v options=%+v", got.messaging, options)
	}
	closed := false
	wire.onClose = func() { closed = true }
	if err := got.messaging.closePublisher(); err != nil || !closed {
		t.Fatalf("native NSQ publisher close: err=%v closed=%t", err, closed)
	}

	closed = false
	_, err = prepareResources(resourceStageDeps{
		mqPublisher: mqPublisherStageDeps{
			enabled: true, provider: "nsq",
			newWirePublisher: func() (wirePublisherResource, error) { return wire, nil },
		},
		loadEventCatalog: func() (*eventcatalog.Catalog, error) { return nil, errors.New("catalog unavailable") },
	})
	if err == nil || !closed {
		t.Fatalf("startup failure leaked native NSQ publisher: err=%v closed=%t", err, closed)
	}
}

func TestAPIProjectionOnlySelectsSDKSubscriberForNSQ(t *testing.T) {
	for _, item := range []struct {
		provider string
		wantSDK  bool
	}{
		{provider: "nsq", wantSDK: true},
		{provider: "rabbitmq", wantSDK: false},
	} {
		t.Run(item.provider, func(t *testing.T) {
			cfg := &apiserverconfig.Config{Options: apiserveroptions.NewOptions()}
			cfg.MessagingOptions.Enabled = true
			cfg.MessagingOptions.Provider = item.provider
			deps := (&server{config: cfg}).buildEventSubsystemResourceDeps()
			if (deps.buildSDKSubscriberFactory != nil) != item.wantSDK {
				t.Fatalf("SDK subscriber factory for %s: got %t want %t", item.provider,
					deps.buildSDKSubscriberFactory != nil, item.wantSDK)
			}
		})
	}
}

func (*fakePublisher) Publish(_ context.Context, _ string, _ []byte) error { return nil }

func (*fakePublisher) PublishMessage(_ context.Context, _ string, _ *messaging.Message) error {
	return nil
}

func (p *fakePublisher) Close() error {
	if p.onClose != nil {
		p.onClose()
	}
	return nil
}

func TestPrepareResourcesClosesPublisherWhenEventSubsystemFails(t *testing.T) {
	closed := false
	_, err := prepareResources(resourceStageDeps{
		mqPublisher: mqPublisherStageDeps{
			enabled: true, provider: "nsq", newWirePublisher: func() (wirePublisherResource, error) {
				return &fakeWirePublisher{onClose: func() { closed = true }}, nil
			},
		},
		loadEventCatalog: func() (*eventcatalog.Catalog, error) { return eventcatalog.NewCatalog(nil), nil },
		eventSubsystem: eventSubsystemResourceDeps{
			newSubsystem: func(eventsubsystem.Options) (*eventsubsystem.Subsystem, error) {
				return nil, errors.New("controlled startup failure")
			},
		},
	})
	if err == nil || !closed {
		t.Fatalf("resource failure leaked MQ publisher: err=%v closed=%t", err, closed)
	}
}

func TestPrepareResourcesBuildsStageOutputFromDeps(t *testing.T) {
	var mysqlDB gorm.DB
	var mongoDB mongo.Database
	var redisClient redis.UniversalClient
	runtimeBundle := &cacheplanebootstrap.RuntimeBundle{Component: "apiserver"}
	subsystem := &cachebootstrap.Subsystem{}
	publisher := &fakeWirePublisher{}
	catalog := eventcatalog.NewCatalog(nil)
	events := &eventsubsystem.Subsystem{}

	var resilienceConfigured bool
	var buildOptionsInput containerOptionsInput
	var eventOptions eventsubsystem.Options
	wantOptions := container.ContainerOptions{PlanEntryBaseURL: "https://entry.example", EventSubsystem: events}
	resilience, err := resiliencesubsystem.New(resiliencesubsystem.Options{Backpressure: apiserveroptions.NewBackpressureOptions()})
	if err != nil {
		t.Fatal(err)
	}

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
			fallbackMode:     eventruntime.PublishModeLogging,
			enabled:          true,
			provider:         "nsq",
			newWirePublisher: func() (wirePublisherResource, error) { return publisher, nil },
		},
		eventSubsystem: eventSubsystemResourceDeps{
			newSubsystem: func(opts eventsubsystem.Options) (*eventsubsystem.Subsystem, error) {
				eventOptions = opts
				return events, nil
			},
		},
		loadEventCatalog: func() (*eventcatalog.Catalog, error) { return catalog, nil },
		buildResilience: func(got *cacheplanebootstrap.RuntimeBundle) (*resiliencesubsystem.Subsystem, error) {
			if got != runtimeBundle {
				t.Fatalf("resilience runtime = %#v, want %#v", got, runtimeBundle)
			}
			resilienceConfigured = true
			return resilience, nil
		},
		buildContainerOptions: func(output containerOptionsInput) container.ContainerOptions {
			buildOptionsInput = output
			return wantOptions
		},
	})
	if err != nil {
		t.Fatalf("prepareResources() error = %v", err)
	}

	if !resilienceConfigured {
		t.Fatal("buildResilience was not called")
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
	if got.messaging.wirePublisher != publisher {
		t.Fatalf("wirePublisher = %#v, want %#v", got.messaging.wirePublisher, publisher)
	}
	if got.messaging.publishMode != eventruntime.PublishModeMQ {
		t.Fatalf("publishMode = %q, want %q", got.messaging.publishMode, eventruntime.PublishModeMQ)
	}
	if !reflect.DeepEqual(got.containerInput.containerOptions, wantOptions) {
		t.Fatalf("containerOptions = %#v, want %#v", got.containerInput.containerOptions, wantOptions)
	}
	if buildOptionsInput.cacheSubsystem != subsystem || buildOptionsInput.eventSubsystem != events || buildOptionsInput.resilience != resilience {
		t.Fatalf("buildContainerOptions input mismatch: %#v", buildOptionsInput)
	}
	if eventOptions.MySQLDB != &mysqlDB || eventOptions.MongoDB != &mongoDB || eventOptions.Catalog != catalog || eventOptions.WirePublisher != publisher {
		t.Fatalf("event subsystem options mismatch: %#v", eventOptions)
	}
	if eventOptions.MySQLLimiter != resilience.Backpressure("mysql") || eventOptions.MongoLimiter != resilience.Backpressure("mongo") {
		t.Fatal("event subsystem did not receive shared resilience backpressure instances")
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

func TestPrepareResourcesFailsClosedWhenEnabledPublisherCannotStart(t *testing.T) {
	for _, deps := range []mqPublisherStageDeps{
		{fallbackMode: eventruntime.PublishModeLogging, enabled: true, provider: "unsupported"},
		{fallbackMode: eventruntime.PublishModeLogging, enabled: true, provider: "nsq",
			newWirePublisher: func() (wirePublisherResource, error) { return nil, errors.New("boom") }},
	} {
		output, err := prepareResources(resourceStageDeps{mqPublisher: deps})
		if err == nil || output.messaging.publishMode == eventruntime.PublishModeLogging {
			t.Fatalf("enabled publisher fell back to logging: output=%+v err=%v", output.messaging, err)
		}
	}
}

func TestPrepareResourcesPreservesFallbackModeWhenMessagingDisabled(t *testing.T) {
	for _, mode := range []eventruntime.PublishMode{eventruntime.PublishModeLogging, eventruntime.PublishModeNop} {
		got, err := prepareResources(resourceStageDeps{
			mqPublisher: mqPublisherStageDeps{fallbackMode: mode},
			eventSubsystem: eventSubsystemResourceDeps{
				newSubsystem: func(opts eventsubsystem.Options) (*eventsubsystem.Subsystem, error) {
					if opts.PublisherMode != mode || opts.WirePublisher != nil {
						t.Fatalf("disabled messaging options = %+v, want mode %q without publisher", opts, mode)
					}
					return &eventsubsystem.Subsystem{}, nil
				},
			},
		})
		if err != nil || got.messaging.publishMode != mode || got.messaging.closePublisher != nil {
			t.Fatalf("disabled messaging output = %+v, err = %v, want mode %q", got.messaging, err, mode)
		}
	}
}

func TestPrepareResourcesStopsBeforeEventSubsystemWhenChannelPreparationFails(t *testing.T) {
	closed := false
	built := false
	_, err := prepareResources(resourceStageDeps{
		mqPublisher: mqPublisherStageDeps{
			enabled: true, provider: "nsq", newWirePublisher: func() (wirePublisherResource, error) {
				return &fakeWirePublisher{onClose: func() { closed = true }}, nil
			},
		},
		loadEventCatalog: func() (*eventcatalog.Catalog, error) { return eventcatalog.NewCatalog(nil), nil },
		prepareMQChannels: func(*eventcatalog.Catalog) error {
			return errors.New("nsqd unavailable")
		},
		eventSubsystem: eventSubsystemResourceDeps{
			newSubsystem: func(eventsubsystem.Options) (*eventsubsystem.Subsystem, error) {
				built = true
				return &eventsubsystem.Subsystem{}, nil
			},
		},
	})
	if err == nil || !closed || built {
		t.Fatalf("unsafe bootstrap: err=%v, publisher closed=%t, event subsystem built=%t", err, closed, built)
	}
}

func TestAPIChannelPreparationCreatesWorkerAndProjectionChannels(t *testing.T) {
	created := make(map[string]bool)
	nsqd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		topic := r.URL.Query().Get("topic")
		switch r.URL.Path {
		case "/topic/create":
			created[topic] = true
		case "/channel/create":
			if !created[topic] {
				t.Errorf("channel %s was created before its topic", r.URL.Query().Get("channel"))
			}
			created[topic+"/"+r.URL.Query().Get("channel")] = true
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer nsqd.Close()
	options := apiserveroptions.NewOptions()
	options.MessagingOptions.Enabled = true
	options.MessagingOptions.NSQDHTTPEndpoints = []string{nsqd.URL}
	options.Eventing.Consumers.ModelCatalogHotRank.Channel = "custom-projection-channel"
	cfg, err := eventcatalog.Load("../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := (&server{config: &apiserverconfig.Config{Options: options}}).buildMQChannelPreparer()(eventcatalog.NewCatalog(cfg)); err != nil {
		t.Fatal(err)
	}
	for _, topic := range []string{"qs.survey.lifecycle", "qs.evaluation.lifecycle", "qs.plan.task"} {
		if !created[topic+"/qs-worker"] {
			t.Errorf("primary Worker channel missing for %s", topic)
		}
	}
	if !created["qs.evaluation.lifecycle/custom-projection-channel"] {
		t.Error("API projection channel missing before serving")
	}
}

func TestAPIServerBuildResourceStageDepsWithoutConfigOmitsConfigBoundBuilders(t *testing.T) {
	deps := (&server{}).buildResourceStageDeps()

	if deps.buildResilience != nil {
		t.Fatal("buildResilience != nil, want nil")
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

	if deps.buildResilience == nil {
		t.Fatal("buildResilience = nil, want callback")
	}
	if deps.buildContainerOptions == nil {
		t.Fatal("buildContainerOptions = nil, want builder")
	}
}
