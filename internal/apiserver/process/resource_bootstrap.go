package process

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/messaging"
	bootstrap "github.com/FangcunMount/qs-server/internal/apiserver/bootstrap"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachebootstrap"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	infraIAM "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	infraMongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	mysqlbp "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

type resourceStageDeps struct {
	dbManager             *bootstrap.DatabaseManager
	database              databaseResourceDeps
	redisRuntime          redisRuntimeStageDeps
	mqPublisher           mqPublisherStageDeps
	applyBackpressure     func()
	buildContainerOptions func(containerOptionsInput) container.ContainerOptions
}

type databaseResourceDeps struct {
	initialize func() error
	getMySQL   func() (*gorm.DB, error)
	getMongo   func() (*mongo.Database, error)
}

type redisRuntimeStageDeps struct {
	getClient      func() (redis.UniversalClient, error)
	buildSubsystem func() *cachebootstrap.Subsystem
}

type mqPublisherStageDeps struct {
	fallbackMode eventconfig.PublishMode
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
		applyBackpressure:     s.buildBackpressureDeps(),
		buildContainerOptions: s.buildContainerOptionsBuilder(),
	}
	return deps
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
		buildSubsystem: func() *cachebootstrap.Subsystem {
			return cachebootstrap.NewSubsystem("apiserver", dbManager, s.config.RedisRuntime, s.buildContainerCacheOptions())
		},
	}
}

func (s *server) buildMQPublisherDeps() mqPublisherStageDeps {
	if s == nil || s.config == nil {
		return mqPublisherStageDeps{}
	}

	deps := mqPublisherStageDeps{
		fallbackMode: eventconfig.PublishModeFromEnv(s.config.GenericServerRunOptions.Mode),
	}
	if s.config.MessagingOptions != nil {
		deps.enabled = s.config.MessagingOptions.Enabled
		deps.provider = s.config.MessagingOptions.Provider
		deps.newPublisher = s.config.MessagingOptions.NewPublisher
	}
	return deps
}

func (s *server) buildBackpressureDeps() func() {
	if s == nil || s.config == nil {
		return nil
	}
	return s.configureBackpressure
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
	if deps.applyBackpressure != nil {
		deps.applyBackpressure()
	}
	redisCache, cacheSubsystem := initializeRedisRuntime(deps.redisRuntime)
	mqPublisher, publishMode := createMQPublisher(deps.mqPublisher)

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
			cacheSubsystem: cacheSubsystem,
		},
	}
	if deps.buildContainerOptions != nil {
		output.containerInput = containerBootstrapInput{containerOptions: deps.buildContainerOptions(containerOptionsInput{
			mqPublisher:    mqPublisher,
			publishMode:    publishMode,
			cacheSubsystem: cacheSubsystem,
		})}
	}
	return output, nil
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

func (s *server) configureBackpressure() {
	if s == nil || s.config == nil || s.config.Backpressure == nil {
		return
	}
	if bp := s.config.Backpressure.MySQL; bp != nil && bp.Enabled {
		mysqlbp.SetLimiter(backpressure.NewLimiter(bp.MaxInflight, time.Duration(bp.TimeoutMs)*time.Millisecond))
	}
	if bp := s.config.Backpressure.Mongo; bp != nil && bp.Enabled {
		infraMongo.SetLimiter(backpressure.NewLimiter(bp.MaxInflight, time.Duration(bp.TimeoutMs)*time.Millisecond))
	}
	if bp := s.config.Backpressure.IAM; bp != nil && bp.Enabled {
		infraIAM.SetLimiter(backpressure.NewLimiter(bp.MaxInflight, time.Duration(bp.TimeoutMs)*time.Millisecond))
	}
}

func initializeRedisRuntime(deps redisRuntimeStageDeps) (redis.UniversalClient, *cachebootstrap.Subsystem) {
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
	if deps.buildSubsystem == nil {
		return redisCache, nil
	}
	return redisCache, deps.buildSubsystem()
}

func createMQPublisher(deps mqPublisherStageDeps) (messaging.Publisher, eventconfig.PublishMode) {
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
	return publisher, eventconfig.PublishModeMQ
}
