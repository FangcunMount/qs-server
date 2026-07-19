package process

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/messaging"
	bootstrap "github.com/FangcunMount/qs-server/internal/apiserver/bootstrap"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	resiliencesubsystem "github.com/FangcunMount/qs-server/internal/apiserver/resilience/subsystem"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	eventtransport "github.com/FangcunMount/qs-server/internal/pkg/eventing/transport"
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
	buildResilience       func(*cacheplanebootstrap.RuntimeBundle) (*resiliencesubsystem.Subsystem, error)
	buildContainerOptions func(containerOptionsInput) container.ContainerOptions
}

type eventSubsystemResourceDeps struct {
	newSubsystem           func(eventsubsystem.Options) (*eventsubsystem.Subsystem, error)
	subscriberFactory      eventsubsystem.SubscriberFactory
	buildSubscriberFactory func(*gorm.DB) (eventsubsystem.SubscriberFactory, error)
	consumers              map[string]eventsubsystem.ConsumerOptions
	mongo                  eventsubsystem.ProfileOptions
	assessment             eventsubsystem.ProfileOptions
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
	fallbackMode eventruntime.PublishMode
	enabled      bool
	provider     string
	newPublisher func() (messaging.Publisher, error)
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
	if s.config.MessagingOptions != nil && s.config.MessagingOptions.Enabled {
		buildSubscriberFactory = func(mysqlDB *gorm.DB) (eventsubsystem.SubscriberFactory, error) {
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
			options, err := eventtransport.NewSubscriberOptions(0, s.config.MessagingOptions.Delivery.EffectiveMaxAttempts(), eventtransport.FailedMessageHandler(recorder))
			if err != nil {
				return nil, err
			}
			config := eventtransport.SubscriberConfig{
				Provider: s.config.MessagingOptions.Provider, NSQLookupdAddr: s.config.MessagingOptions.NSQLookupdAddr, RabbitMQURL: s.config.MessagingOptions.RabbitMQURL,
			}
			return func() (messaging.Subscriber, error) { return eventtransport.NewSubscriber(config, options) }, nil
		}
	}
	mongoProfile, assessmentProfile := buildEventProfileOptions(s.config)
	return eventSubsystemResourceDeps{
		newSubsystem:           eventsubsystem.New,
		buildSubscriberFactory: buildSubscriberFactory,
		consumers:              buildEventConsumerOptions(s.config),
		mongo:                  mongoProfile,
		assessment:             assessmentProfile,
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
		deps.enabled = s.config.MessagingOptions.Enabled
		deps.provider = s.config.MessagingOptions.Provider
		deps.newPublisher = s.config.MessagingOptions.NewPublisher
	}
	return deps
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
	mqPublisher, publishMode := createMQPublisher(deps.mqPublisher)
	eventCatalog, err := loadEventCatalog(deps.loadEventCatalog)
	if err != nil {
		return resourceOutput{}, err
	}
	events, err := buildResourceEventSubsystem(mysqlDB, mongoDB, cacheSubsystem, eventCatalog, mqPublisher, publishMode, resilience, deps.eventSubsystem)
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
			mqPublisher: mqPublisher,
			publishMode: publishMode,
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
	cacheSubsystem *cachebootstrap.Subsystem,
	catalog *eventcatalog.Catalog,
	mqPublisher messaging.Publisher,
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
	return deps.newSubsystem(eventsubsystem.Options{
		MySQLDB: mysqlDB, MongoDB: mongoDB, OpsRedis: opsRedis,
		Catalog: catalog, MQPublisher: mqPublisher, PublisherMode: publishMode,
		MySQLLimiter: mysqlLimiter, MongoLimiter: mongoLimiter,
		Mongo: deps.mongo, Assessment: deps.assessment,
		SubscriberFactory: subscriberFactory, Consumers: deps.consumers,
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

func createMQPublisher(deps mqPublisherStageDeps) (messaging.Publisher, eventruntime.PublishMode) {
	if !deps.enabled || deps.newPublisher == nil {
		return nil, deps.fallbackMode
	}

	publisher, err := deps.newPublisher()
	if err != nil {
		logger.L(context.Background()).Warnw("Failed to create MQ publisher, falling back to logging mode",
			"component", "apiserver",
			"error", err.Error(),
		)
		return nil, deps.fallbackMode
	}
	logger.L(context.Background()).Infow("MQ publisher created successfully",
		"component", "apiserver",
		"provider", deps.provider,
	)
	return publisher, eventruntime.PublishModeMQ
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
