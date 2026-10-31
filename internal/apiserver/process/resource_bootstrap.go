package process

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	bootstrap "github.com/FangcunMount/qs-server/internal/apiserver/bootstrap"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	resiliencesubsystem "github.com/FangcunMount/qs-server/internal/apiserver/resilience/subsystem"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	eventtransport "github.com/FangcunMount/qs-server/internal/pkg/eventing/transport"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/bootstrap"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

type resourceStageDeps struct {
	validateMessaging     func() error
	dbManager             *bootstrap.DatabaseManager
	database              databaseResourceDeps
	redisRuntime          redisRuntimeStageDeps
	mqPublisher           mqPublisherStageDeps
	eventSubsystem        eventSubsystemResourceDeps
	loadEventCatalog      func() (*eventcatalog.Catalog, error)
	buildResilience       func(*cacheplanebootstrap.RuntimeBundle) (*resiliencesubsystem.Subsystem, error)
	buildContainerOptions func(containerOptionsInput) container.ContainerOptions
}

type eventSubsystemResourceDeps struct {
	newSubsystem              func(eventsubsystem.Options) (*eventsubsystem.Subsystem, error)
	sdkSubscriberFactory      eventsubsystem.SDKSubscriberFactory
	buildSDKSubscriberFactory func(*gorm.DB) (eventsubsystem.SDKSubscriberFactory, error)
	consumers                 map[string]eventsubsystem.ConsumerOptions
	wirePublisher             eventruntime.WirePublisher
}

type wirePublisherResource interface {
	eventruntime.WirePublisher
	Close() error
}

type databaseResourceDeps struct {
	close      func() error
	initialize func() error
	getMySQL   func() (*gorm.DB, error)
	getMongo   func() (*mongo.Database, error)
}

type redisRuntimeStageDeps struct {
	getClient      func() (redis.UniversalClient, error)
	buildRuntime   func() *cacheplanebootstrap.RuntimeBundle
	buildSubsystem func(*cacheplanebootstrap.RuntimeBundle) *cachebootstrap.Subsystem
}

type mqPublisherStageDeps struct {
	fallbackMode       eventruntime.PublishMode
	enabled            bool
	provider           string
	newNormalPublisher func(*eventcatalog.Catalog) (wirePublisherResource, error)
}

func (s *server) buildResourceStageDeps() resourceStageDeps {
	if s == nil {
		return resourceStageDeps{}
	}

	dbManager := s.buildDatabaseManager()
	deps := resourceStageDeps{
		validateMessaging:     func() error { return validateFinalMessaging(s.config) },
		dbManager:             dbManager,
		database:              buildDatabaseDeps(dbManager),
		redisRuntime:          s.buildRedisRuntimeDeps(dbManager),
		mqPublisher:           s.buildMQPublisherDeps(),
		eventSubsystem:        s.buildEventSubsystemResourceDeps(),
		loadEventCatalog:      loadDefaultEventCatalog,
		buildResilience:       s.buildResilienceDeps(),
		buildContainerOptions: s.buildContainerOptionsBuilder(),
	}
	return deps
}

func (s *server) buildResilienceDeps() func(*cacheplanebootstrap.RuntimeBundle) (*resiliencesubsystem.Subsystem, error) {
	if s == nil || s.config == nil {
		return nil
	}
	return s.buildResilienceSubsystem
}

func (s *server) buildEventSubsystemResourceDeps() eventSubsystemResourceDeps {
	if s == nil || s.config == nil {
		return eventSubsystemResourceDeps{}
	}
	var buildSDKSubscriberFactory func(*gorm.DB) (eventsubsystem.SDKSubscriberFactory, error)
	if s.config.MessagingOptions != nil && s.config.MessagingOptions.Enabled {
		recorderFor := func(mysqlDB *gorm.DB) (*eventtransport.SQLDeadLetterRecorder, error) {
			if mysqlDB == nil {
				return nil, fmt.Errorf("event delivery dead-letter database is not configured")
			}
			sqlDB, err := mysqlDB.DB()
			if err != nil {
				return nil, fmt.Errorf("resolve event delivery dead-letter database: %w", err)
			}
			recorder, err := eventtransport.NewSQLDeadLetterRecorder(sqlDB)
			if err != nil {
				return nil, err
			}
			return recorder, nil
		}
		if validateFinalMessaging(s.config) == nil {
			buildSDKSubscriberFactory = func(mysqlDB *gorm.DB) (eventsubsystem.SDKSubscriberFactory, error) {
				recorder, err := recorderFor(mysqlDB)
				if err != nil {
					return nil, err
				}
				config := eventtransport.SubscriberConfig{
					RocketMQ: s.config.MessagingOptions.RocketMQ, Provider: "rocketmq",
				}
				return func() (eventsubsystem.SDKSubscriber, error) {
					return eventtransport.NewSDKDeliverySubscriber(config, 0,
						s.config.MessagingOptions.Delivery.EffectiveMaxAttempts(), eventtransport.SDKRocketMQFailureHandler(recorder))
				}, nil
			}
		}
	}
	return eventSubsystemResourceDeps{
		newSubsystem:              configuredEventSubsystem(s.config),
		buildSDKSubscriberFactory: buildSDKSubscriberFactory,
		consumers:                 buildEventConsumerOptions(s.config),
	}
}

func (s *server) buildDatabaseManager() *bootstrap.DatabaseManager {
	if s == nil || s.config == nil {
		return nil
	}
	return bootstrap.NewDatabaseManager(s.config)
}

func buildDatabaseDeps(dbManager *bootstrap.DatabaseManager) databaseResourceDeps {
	if dbManager == nil {
		return databaseResourceDeps{}
	}

	return databaseResourceDeps{
		initialize: dbManager.Initialize,
		close:      dbManager.Close,
		getMySQL:   dbManager.GetMySQLDB,
		getMongo:   dbManager.GetMongoDB,
	}
}

func (s *server) buildRedisRuntimeDeps(dbManager *bootstrap.DatabaseManager) redisRuntimeStageDeps {
	if dbManager == nil || s == nil || s.config == nil {
		return redisRuntimeStageDeps{}
	}

	return redisRuntimeStageDeps{
		getClient: dbManager.GetRedisClient,
		buildRuntime: func() *cacheplanebootstrap.RuntimeBundle {
			return cacheplanebootstrap.BuildRuntime(context.Background(), cacheplanebootstrap.Options{
				Component:      "apiserver",
				RuntimeOptions: s.config.RedisRuntime,
				Resolver:       dbManager,
			})
		},
		buildSubsystem: func(runtimeBundle *cacheplanebootstrap.RuntimeBundle) *cachebootstrap.Subsystem {
			subsystem := cachebootstrap.NewSubsystemFromRuntime(runtimeBundle, s.buildContainerCacheOptions())
			subsystem.BindPolicyReloader(s.cachePolicyCandidateLoader(subsystem.EffectiveRegistry()))
			return subsystem
		},
	}
}

func (s *server) buildMQPublisherDeps() mqPublisherStageDeps {
	if s == nil || s.config == nil {
		return mqPublisherStageDeps{}
	}

	deps := mqPublisherStageDeps{
		fallbackMode: eventruntime.PublishModeFromEnv(s.config.GenericServerRunOptions.Mode),
	}
	if s.config.MessagingOptions != nil {
		options := s.config.MessagingOptions
		deps.enabled = options.Enabled
		deps.provider = options.Provider
		if validateFinalMessaging(s.config) == nil {
			deps.newNormalPublisher = func(catalog *eventcatalog.Catalog) (wirePublisherResource, error) {
				topics, err := eventruntime.BestEffortTopics(catalog)
				if err != nil {
					return nil, err
				}
				binding, ok := options.RocketMQ.Transactions["qs.mysql"]
				if !ok {
					return nil, fmt.Errorf("NORMAL host publisher requires explicit qs.mysql binding")
				}
				limits, err := binding.ProtocolLimits("qs.mysql")
				if err != nil {
					return nil, err
				}
				p, err := messagingruntime.NewNormalWirePublisher(options, topics, limits.Sends)
				if err != nil {
					return nil, err
				}
				if err = p.Start(); err != nil {
					return nil, errors.Join(err, (&normalWireResource{NormalWirePublisher: p}).Close())
				}
				return &normalWireResource{NormalWirePublisher: p}, nil
			}
		}
	}
	return deps
}

func (s *server) buildContainerOptionsBuilder() func(containerOptionsInput) container.ContainerOptions {
	if s == nil || s.config == nil {
		return nil
	}
	return s.buildContainerOptions
}

func prepareResources(deps resourceStageDeps) (result resourceOutput, resultErr error) {
	if deps.validateMessaging != nil {
		if err := deps.validateMessaging(); err != nil {
			return resourceOutput{}, err
		}
	}

	var closePublisher func() error
	var events *eventsubsystem.Subsystem
	var cacheSubsystem *cachebootstrap.Subsystem
	defer func() {
		if resultErr == nil {
			return
		}
		// These constructors can own started NORMAL clients even before the resource
		// stage is published. Do not discard their drain errors or close DB early.
		cleanup := processLifecycleDeps{resource: resourceLifecycleDeps{closePublisher: closePublisher, closeDatabase: deps.database.close}}
		cleanup.container.containerCleanup = func() error {
			ctx := context.Background()
			if events != nil {
				if err := events.CloseContext(ctx); err != nil {
					return err
				}
			}
			if cacheSubsystem != nil {
				return cacheSubsystem.Close()
			}
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		resultErr = errors.Join(resultErr, newProcessShutdown(cleanup).Close(ctx))
	}()

	mysqlDB, mongoDB, err := initializeDatabaseConnections(deps.database)
	if err != nil {
		return resourceOutput{}, err
	}
	redisCache, redisRuntime, constructedCache := initializeRedisRuntime(deps.redisRuntime)
	cacheSubsystem = constructedCache
	var resilience *resiliencesubsystem.Subsystem
	if deps.buildResilience != nil {
		resilience, err = deps.buildResilience(redisRuntime)
		if err != nil {
			return resourceOutput{}, err
		}
	}
	actionAuditStore, actionAuditRunner := buildActionAuditRuntime(mysqlDB, redisRuntime)
	var wirePublisher wirePublisherResource
	publishMode := deps.mqPublisher.fallbackMode
	if deps.mqPublisher.enabled {
		if deps.mqPublisher.provider != "rocketmq" || deps.mqPublisher.newNormalPublisher == nil {
			return resourceOutput{}, fmt.Errorf("final RocketMQ NORMAL publisher factory is required")
		}
		publishMode = eventruntime.PublishModeMQ
	}

	eventCatalog, err := loadEventCatalog(deps.loadEventCatalog)
	if err != nil {
		return resourceOutput{}, err
	}
	if deps.mqPublisher.enabled && deps.mqPublisher.newNormalPublisher != nil {
		wirePublisher, err = deps.mqPublisher.newNormalPublisher(eventCatalog)
		if err != nil {
			return resourceOutput{}, err
		}
		if wirePublisher == nil {
			return resourceOutput{}, fmt.Errorf("NORMAL publisher factory returned nil")
		}
		closePublisher = wirePublisher.Close
	}

	deps.eventSubsystem.wirePublisher = wirePublisher
	events, err = buildResourceEventSubsystem(mysqlDB, mongoDB, eventCatalog, publishMode, deps.eventSubsystem)
	if err != nil {
		return resourceOutput{}, err
	}

	output := resourceOutput{
		handles: resourceHandles{
			dbManager:  deps.dbManager,
			mysqlDB:    mysqlDB,
			mongoDB:    mongoDB,
			redisCache: redisCache,
		},
		messaging: messagingOutput{
			wirePublisher:  wirePublisher,
			closePublisher: closePublisher,
			publishMode:    publishMode,
		},
		cacheRuntime: cacheRuntimeOutput{
			redisRuntime:   redisRuntime,
			cacheSubsystem: cacheSubsystem,
		},
	}
	if deps.buildContainerOptions != nil {
		containerOptions := deps.buildContainerOptions(containerOptionsInput{
			cacheSubsystem:    cacheSubsystem,
			resilience:        resilience,
			eventSubsystem:    events,
			actionAuditStore:  actionAuditStore,
			actionAuditRunner: actionAuditRunner,
		})
		output.containerInput = containerBootstrapInput{containerOptions: containerOptions}
	}
	return output, nil
}

func buildResourceEventSubsystem(
	mysqlDB *gorm.DB,
	mongoDB *mongo.Database,
	catalog *eventcatalog.Catalog,
	publishMode eventruntime.PublishMode,
	deps eventSubsystemResourceDeps,
) (*eventsubsystem.Subsystem, error) {
	if deps.newSubsystem == nil {
		return nil, fmt.Errorf("event subsystem constructor is not configured")
	}

	sdkSubscriberFactory := deps.sdkSubscriberFactory
	if deps.buildSDKSubscriberFactory != nil {
		var err error
		sdkSubscriberFactory, err = deps.buildSDKSubscriberFactory(mysqlDB)
		if err != nil {
			return nil, err
		}
	}
	return deps.newSubsystem(eventsubsystem.Options{
		MySQLDB: mysqlDB, MongoDB: mongoDB,
		Catalog: catalog, WirePublisher: deps.wirePublisher, PublisherMode: publishMode,
		SDKSubscriberFactory: sdkSubscriberFactory, Consumers: deps.consumers,
	})
}

func initializeDatabaseConnections(deps databaseResourceDeps) (*gorm.DB, *mongo.Database, error) {
	if deps.initialize == nil {
		return nil, nil, nil
	}
	if err := deps.initialize(); err != nil {
		return nil, nil, err
	}
	mysqlDB, err := deps.getMySQL()
	if err != nil {
		return nil, nil, err
	}
	mongoDB, err := deps.getMongo()
	if err != nil {
		return nil, nil, err
	}
	return mysqlDB, mongoDB, nil
}

func initializeRedisRuntime(deps redisRuntimeStageDeps) (redis.UniversalClient, *cacheplanebootstrap.RuntimeBundle, *cachebootstrap.Subsystem) {
	var redisCache redis.UniversalClient
	if deps.getClient != nil {
		client, err := deps.getClient()
		if err != nil {
			logger.L(context.Background()).Warnw("Cache Redis not available",
				"component", "apiserver",
				"error", err.Error(),
			)
		}
		redisCache = client
	}
	var redisRuntime *cacheplanebootstrap.RuntimeBundle
	if deps.buildRuntime != nil {
		redisRuntime = deps.buildRuntime()
	}
	if deps.buildSubsystem == nil {
		return redisCache, redisRuntime, nil
	}
	return redisCache, redisRuntime, deps.buildSubsystem(redisRuntime)
}

func loadDefaultEventCatalog() (*eventcatalog.Catalog, error) {
	cfg, err := eventcatalog.Load("configs/events.yaml")
	if err != nil {
		return nil, err
	}
	return eventcatalog.NewCatalog(cfg), nil
}

func loadEventCatalog(load func() (*eventcatalog.Catalog, error)) (*eventcatalog.Catalog, error) {
	if load == nil {
		return eventcatalog.NewCatalog(nil), nil
	}
	return load()
}
