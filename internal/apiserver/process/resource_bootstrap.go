package process

import (
	"context"
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
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/bootstrap"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

type resourceStageDeps struct {
	dbManager             *bootstrap.DatabaseManager
	database              databaseResourceDeps
	redisRuntime          redisRuntimeStageDeps
	mqPublisher           mqPublisherStageDeps
	eventSubsystem        eventSubsystemResourceDeps
	loadEventCatalog      func() (*eventcatalog.Catalog, error)
	prepareMQChannels     func(*eventcatalog.Catalog) error
	buildResilience       func(*cacheplanebootstrap.RuntimeBundle) (*resiliencesubsystem.Subsystem, error)
	buildContainerOptions func(containerOptionsInput) container.ContainerOptions
}

type eventSubsystemResourceDeps struct {
	newSubsystem              func(eventsubsystem.Options) (*eventsubsystem.Subsystem, error)
	subscriberFactory         eventsubsystem.SubscriberFactory
	buildSubscriberFactory    func(*gorm.DB) (eventsubsystem.SubscriberFactory, error)
	sdkSubscriberFactory      eventsubsystem.SDKSubscriberFactory
	buildSDKSubscriberFactory func(*gorm.DB) (eventsubsystem.SDKSubscriberFactory, error)
	consumers                 map[string]eventsubsystem.ConsumerOptions
	mongo                     eventsubsystem.ProfileOptions
	assessment                eventsubsystem.ProfileOptions
	wirePublisher             eventruntime.WirePublisher
}

type wirePublisherResource interface {
	eventruntime.WirePublisher
	Close() error
}

type databaseResourceDeps struct {
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
	fallbackMode     eventruntime.PublishMode
	enabled          bool
	provider         string
	newWirePublisher func() (wirePublisherResource, error)
}

func (s *server) buildResourceStageDeps() resourceStageDeps {
	if s == nil {
		return resourceStageDeps{}
	}

	dbManager := s.buildDatabaseManager()
	deps := resourceStageDeps{
		dbManager:             dbManager,
		database:              buildDatabaseDeps(dbManager),
		redisRuntime:          s.buildRedisRuntimeDeps(dbManager),
		mqPublisher:           s.buildMQPublisherDeps(),
		eventSubsystem:        s.buildEventSubsystemResourceDeps(),
		loadEventCatalog:      loadDefaultEventCatalog,
		prepareMQChannels:     s.buildMQChannelPreparer(),
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
	var buildSubscriberFactory func(*gorm.DB) (eventsubsystem.SubscriberFactory, error)
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
		if s.config.MessagingOptions.Provider == "nsq" {
			buildSDKSubscriberFactory = func(mysqlDB *gorm.DB) (eventsubsystem.SDKSubscriberFactory, error) {
				recorder, err := recorderFor(mysqlDB)
				if err != nil {
					return nil, err
				}
				config := eventtransport.SubscriberConfig{
					Provider: "nsq", NSQLookupdAddr: s.config.MessagingOptions.NSQLookupdAddr,
				}
				return func() (eventsubsystem.SDKSubscriber, error) {
					return eventtransport.NewSDKDeliverySubscriber(config, 0,
						s.config.MessagingOptions.Delivery.EffectiveMaxAttempts(), eventtransport.SDKFailedHandoffHandler(recorder))
				}, nil
			}
		}
	}
	mongoProfile, assessmentProfile := buildEventProfileOptions(s.config)
	return eventSubsystemResourceDeps{
		newSubsystem:              configuredEventSubsystem(s.config),
		buildSubscriberFactory:    buildSubscriberFactory,
		buildSDKSubscriberFactory: buildSDKSubscriberFactory,
		consumers:                 buildEventConsumerOptions(s.config),
		mongo:                     mongoProfile,
		assessment:                assessmentProfile,
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
		if options.Provider == "nsq" {
			deps.newWirePublisher = func() (wirePublisherResource, error) {
				return messagingruntime.NewSDKNSQWirePublisher(options.NSQAddr)
			}
		}
	}
	return deps
}

func (s *server) buildMQChannelPreparer() func(*eventcatalog.Catalog) error {
	if s == nil || s.config == nil || s.config.MessagingOptions == nil {
		return nil
	}
	options := s.config.MessagingOptions
	if !options.Enabled || options.Provider != "nsq" {
		return nil
	}
	consumerOptions := buildEventConsumerOptions(s.config)
	return func(catalog *eventcatalog.Catalog) error {
		registry, err := eventcatalog.NewEffectiveRegistry(catalog, eventcatalog.DefaultSpecs())
		if err != nil {
			return fmt.Errorf("resolve NSQ channel catalog: %w", err)
		}
		channels := make([]messagingruntime.DurableChannel, 0)
		for _, subscription := range catalog.TopicSubscriptions() {
			channels = append(channels, messagingruntime.DurableChannel{Topic: subscription.TopicName, Channel: options.PrimaryWorkerChannel})
		}
		for _, event := range registry.Snapshot() {
			for _, consumer := range event.AdditionalConsumers {
				if configured, ok := consumerOptions[consumer.ID]; ok {
					if !configured.Enabled {
						continue
					}
					if configured.Channel != "" {
						consumer.Channel = configured.Channel
					}
				}
				channels = append(channels, messagingruntime.DurableChannel{Topic: event.Topic, Channel: consumer.Channel})
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return messagingruntime.EnsureNSQChannels(ctx, options.NSQDHTTPEndpoints, channels)
	}
}

func (s *server) buildContainerOptionsBuilder() func(containerOptionsInput) container.ContainerOptions {
	if s == nil || s.config == nil {
		return nil
	}
	return s.buildContainerOptions
}

func prepareResources(deps resourceStageDeps) (resourceOutput, error) {
	mysqlDB, mongoDB, err := initializeDatabaseConnections(deps.database)
	if err != nil {
		return resourceOutput{}, err
	}
	redisCache, redisRuntime, cacheSubsystem := initializeRedisRuntime(deps.redisRuntime)
	var resilience *resiliencesubsystem.Subsystem
	if deps.buildResilience != nil {
		resilience, err = deps.buildResilience(redisRuntime)
		if err != nil {
			return resourceOutput{}, err
		}
	}
	actionAuditStore, actionAuditRunner := buildActionAuditRuntime(mysqlDB, redisRuntime)
	var wirePublisher wirePublisherResource
	var closePublisher func() error
	publishMode := deps.mqPublisher.fallbackMode
	if deps.mqPublisher.enabled {
		if deps.mqPublisher.provider != "nsq" || deps.mqPublisher.newWirePublisher == nil {
			return resourceOutput{}, fmt.Errorf("NSQ wire publisher factory is required when messaging is enabled")
		}
		wirePublisher, err = createNSQWirePublisher(deps.mqPublisher)
		if err != nil {
			return resourceOutput{}, err
		}
		closePublisher = wirePublisher.Close
		publishMode = eventruntime.PublishModeMQ
	}
	prepared := false
	defer func() {
		if !prepared && closePublisher != nil {
			_ = closePublisher()
		}
	}()
	eventCatalog, err := loadEventCatalog(deps.loadEventCatalog)
	if err != nil {
		return resourceOutput{}, err
	}
	if deps.prepareMQChannels != nil {
		if err := deps.prepareMQChannels(eventCatalog); err != nil {
			return resourceOutput{}, fmt.Errorf("prepare MQ channels before serving: %w", err)
		}
	}
	deps.eventSubsystem.wirePublisher = wirePublisher
	events, err := buildResourceEventSubsystem(mysqlDB, mongoDB, cacheSubsystem, eventCatalog, publishMode, resilience, deps.eventSubsystem)
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
	prepared = true
	return output, nil
}

func buildResourceEventSubsystem(
	mysqlDB *gorm.DB,
	mongoDB *mongo.Database,
	cacheSubsystem *cachebootstrap.Subsystem,
	catalog *eventcatalog.Catalog,
	publishMode eventruntime.PublishMode,
	resilience *resiliencesubsystem.Subsystem,
	deps eventSubsystemResourceDeps,
) (*eventsubsystem.Subsystem, error) {
	if deps.newSubsystem == nil {
		return nil, fmt.Errorf("event subsystem constructor is not configured")
	}
	var opsRedis redis.UniversalClient
	if cacheSubsystem != nil {
		opsRedis = cacheSubsystem.Client(redisruntime.FamilyOps)
	}
	var mysqlLimiter, mongoLimiter backpressure.Acquirer
	if resilience != nil {
		mysqlLimiter = resilience.Backpressure("mysql")
		mongoLimiter = resilience.Backpressure("mongo")
	}
	subscriberFactory := deps.subscriberFactory
	if deps.buildSubscriberFactory != nil {
		var err error
		subscriberFactory, err = deps.buildSubscriberFactory(mysqlDB)
		if err != nil {
			return nil, err
		}
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
		MySQLDB: mysqlDB, MongoDB: mongoDB, OpsRedis: opsRedis,
		Catalog: catalog, WirePublisher: deps.wirePublisher, PublisherMode: publishMode,
		MySQLLimiter: mysqlLimiter, MongoLimiter: mongoLimiter,
		Mongo: deps.mongo, Assessment: deps.assessment,
		SubscriberFactory: subscriberFactory, SDKSubscriberFactory: sdkSubscriberFactory, Consumers: deps.consumers,
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

func createNSQWirePublisher(deps mqPublisherStageDeps) (wirePublisherResource, error) {
	if deps.newWirePublisher == nil {
		return nil, fmt.Errorf("NSQ wire publisher factory is required")
	}
	publisher, err := deps.newWirePublisher()
	if err != nil {
		return nil, fmt.Errorf("create NSQ wire publisher: %w", err)
	}
	if publisher == nil {
		return nil, fmt.Errorf("NSQ wire publisher factory returned nil")
	}
	logger.L(context.Background()).Infow("MQ publisher created successfully",
		"component", "apiserver", "provider", "nsq")
	return publisher, nil
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
