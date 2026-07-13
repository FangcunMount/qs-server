package process

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/messaging"
	bootstrap "github.com/FangcunMount/qs-server/internal/apiserver/bootstrap"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/bootstrap"
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
	buildBackpressure     func() container.BackpressureOptions
	buildContainerOptions func(containerOptionsInput) container.ContainerOptions
}

type eventSubsystemResourceDeps struct {
	newSubsystem      func(eventsubsystem.Options) (*eventsubsystem.Subsystem, error)
	subscriberFactory eventsubsystem.SubscriberFactory
	consumers         map[string]eventsubsystem.ConsumerOptions
	mongo             eventsubsystem.ProfileOptions
	assessment        eventsubsystem.ProfileOptions
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
		buildBackpressure:     s.buildBackpressureDeps(),
		buildContainerOptions: s.buildContainerOptionsBuilder(),
	}
	return deps
}

func (s *server) buildEventSubsystemResourceDeps() eventSubsystemResourceDeps {
	if s == nil || s.config == nil {
		return eventSubsystemResourceDeps{}
	}
	var subscriberFactory eventsubsystem.SubscriberFactory
	if s.config.MessagingOptions != nil && s.config.MessagingOptions.Enabled {
		subscriberFactory = s.config.MessagingOptions.NewSubscriber
	}
	mongoProfile, assessmentProfile := buildEventProfileOptions(s.config)
	return eventSubsystemResourceDeps{
		newSubsystem:      eventsubsystem.New,
		subscriberFactory: subscriberFactory,
		consumers:         buildEventConsumerOptions(s.config),
		mongo:             mongoProfile,
		assessment:        assessmentProfile,
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
				LockName:       "lock_lease",
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

func (s *server) buildBackpressureDeps() func() container.BackpressureOptions {
	if s == nil || s.config == nil {
		return nil
	}
	return s.buildBackpressureOptions
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
	var backpressureOptions container.BackpressureOptions
	if deps.buildBackpressure != nil {
		backpressureOptions = deps.buildBackpressure()
	}
	redisCache, redisRuntime, cacheSubsystem := initializeRedisRuntime(deps.redisRuntime)
	mqPublisher, publishMode := createMQPublisher(deps.mqPublisher)
	eventCatalog, err := loadEventCatalog(deps.loadEventCatalog)
	if err != nil {
		return resourceOutput{}, err
	}
	events, err := buildResourceEventSubsystem(mysqlDB, mongoDB, cacheSubsystem, eventCatalog, mqPublisher, publishMode, backpressureOptions, deps.eventSubsystem)
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
			cacheSubsystem: cacheSubsystem,
			backpressure:   backpressureOptions,
			eventSubsystem: events,
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
	backpressureOptions container.BackpressureOptions,
	deps eventSubsystemResourceDeps,
) (*eventsubsystem.Subsystem, error) {
	if deps.newSubsystem == nil {
		return nil, fmt.Errorf("event subsystem constructor is not configured")
	}
	var opsRedis redis.UniversalClient
	if cacheSubsystem != nil {
		opsRedis = cacheSubsystem.Client(redisruntime.FamilyOps)
	}
	return deps.newSubsystem(eventsubsystem.Options{
		MySQLDB: mysqlDB, MongoDB: mongoDB, OpsRedis: opsRedis,
		Catalog: catalog, MQPublisher: mqPublisher, PublisherMode: publishMode,
		MySQLLimiter: backpressureOptions.MySQL, MongoLimiter: backpressureOptions.Mongo,
		Mongo: deps.mongo, Assessment: deps.assessment,
		SubscriberFactory: deps.subscriberFactory, Consumers: deps.consumers,
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

func (s *server) buildBackpressureOptions() container.BackpressureOptions {
	if s == nil || s.config == nil || s.config.Backpressure == nil {
		return container.BackpressureOptions{}
	}
	options := container.BackpressureOptions{}
	if bp := s.config.Backpressure.MySQL; bp != nil && bp.Enabled {
		options.MySQL = newDependencyBackpressureLimiter("mysql", bp.MaxInflight, bp.TimeoutMs)
	}
	if bp := s.config.Backpressure.Mongo; bp != nil && bp.Enabled {
		options.Mongo = newDependencyBackpressureLimiter("mongo", bp.MaxInflight, bp.TimeoutMs)
	}
	if bp := s.config.Backpressure.IAM; bp != nil && bp.Enabled {
		options.IAM = newDependencyBackpressureLimiter("iam", bp.MaxInflight, bp.TimeoutMs)
	}
	return options
}

func newDependencyBackpressureLimiter(dependency string, maxInflight int, timeoutMs int) *backpressure.Limiter {
	return backpressure.NewLimiterWithOptions(maxInflight, time.Duration(timeoutMs)*time.Millisecond, backpressure.Options{
		Component:  "apiserver",
		Dependency: dependency,
	})
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
