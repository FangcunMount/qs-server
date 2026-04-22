package apiserver

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/component-base/pkg/shutdown"
	"github.com/FangcunMount/component-base/pkg/shutdown/shutdownmanagers/posixsignal"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	domainoperator "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	infraIAM "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	infraMongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	runtimescheduler "github.com/FangcunMount/qs-server/internal/apiserver/runtime/scheduler"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	mysqlbp "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	grpcpkg "github.com/FangcunMount/qs-server/internal/pkg/grpc"
	iamauth "github.com/FangcunMount/qs-server/internal/pkg/iamauth"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	genericapiserver "github.com/FangcunMount/qs-server/internal/pkg/server"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// apiServer 定义了 API 服务器的基本结构（六边形架构版本）
type apiServer struct {
	// 优雅关闭管理器
	gs *shutdown.GracefulShutdown
	// 通用 API 服务器
	genericAPIServer *genericapiserver.GenericAPIServer
	// GRPC 服务器
	grpcServer *grpcpkg.Server
	// 数据库管理器
	dbManager *DatabaseManager
	// Redis 客户端（供内建调度器等后台任务复用）
	redisCache redis.UniversalClient
	// 共享 Redis family runtime
	redisRuntime *redisplane.Runtime
	// lock/lease Redis runtime handle
	lockHandle *redisplane.Handle
	// apiserver 内建 scheduler 共享锁管理器
	lockManager *redislock.Manager
	// Container 主容器
	container *container.Container
	// Redis family 状态注册表
	familyStatus *cacheobservability.FamilyStatusRegistry
	// IAM authz_version 同步订阅者
	authzVersionSubscriber messaging.Subscriber
	// 配置
	config *config.Config
}

// preparedAPIServer 定义了准备运行的 API 服务器
type preparedAPIServer struct {
	*apiServer
}

type prepareRedisHandles struct {
	static *redisplane.Handle
	object *redisplane.Handle
	query  *redisplane.Handle
	meta   *redisplane.Handle
	sdk    *redisplane.Handle
	lock   *redisplane.Handle
}

type prepareResources struct {
	mysqlDB      *gorm.DB
	mongoDB      *mongo.Database
	redisCache   redis.UniversalClient
	redisHandles prepareRedisHandles
	mqPublisher  messaging.Publisher
	publishMode  eventconfig.PublishMode
}

// createAPIServer 创建 API 服务器实例（六边形架构版本）
func createAPIServer(cfg *config.Config) (*apiServer, error) {
	// 创建一个 GracefulShutdown 实例
	gs := shutdown.New()
	gs.AddShutdownManager(posixsignal.NewPosixSignalManager())

	// 创建  服务器
	genericServer, err := buildGenericServer(cfg)
	if err != nil {
		logger.L(context.Background()).Errorw("Failed to build generic server",
			"component", "apiserver",
			"error", err.Error(),
		)
		log.Fatalf("Failed to build generic server: %v", err)
		return nil, err
	}

	// 创建数据库管理器
	dbManager := NewDatabaseManager(cfg)

	// 创建 API 服务器实例（gRPC Server 在 PrepareRun 中创建，因为需要 IAM SDK）
	server := &apiServer{
		gs:               gs,
		genericAPIServer: genericServer,
		dbManager:        dbManager,
		redisCache:       nil,
		grpcServer:       nil, // 延迟初始化
		config:           cfg,
		familyStatus:     cacheobservability.NewFamilyStatusRegistry("apiserver"),
	}

	return server, nil
}

// PrepareRun 准备运行 API 服务器（六边形架构版本）
func (s *apiServer) PrepareRun() preparedAPIServer {
	resources, err := s.prepareResources()
	if err != nil {
		s.fatalPrepareRun("prepare resources", err)
	}

	s.container = container.NewContainerWithOptions(
		resources.mysqlDB,
		resources.mongoDB,
		resources.redisCache,
		s.buildContainerOptions(resources),
	)
	if err := s.initializeContainer(); err != nil {
		s.fatalPrepareRun("initialize container", err)
	}
	if err := s.initializeWeChatServices(); err != nil {
		s.fatalPrepareRun("initialize wechat services", err)
	}
	if err := s.initializeTransports(); err != nil {
		s.fatalPrepareRun("initialize transports", err)
	}
	s.logInitialization(resources.mongoDB != nil)
	s.startWarmup()
	s.startSchedulers()
	s.startMongoOutboxRelay()
	s.startAssessmentOutboxRelay()
	s.registerShutdownCallback()

	return preparedAPIServer{s}
}

func (s *apiServer) prepareResources() (*prepareResources, error) {
	mysqlDB, mongoDB, err := s.initializeDatabaseConnections()
	if err != nil {
		return nil, err
	}
	s.configureBackpressure()
	redisCache, handles := s.initializeRedisRuntime()
	mqPublisher, publishMode := s.createMQPublisher()
	return &prepareResources{
		mysqlDB:      mysqlDB,
		mongoDB:      mongoDB,
		redisCache:   redisCache,
		redisHandles: handles,
		mqPublisher:  mqPublisher,
		publishMode:  publishMode,
	}, nil
}

func (s *apiServer) initializeDatabaseConnections() (*gorm.DB, *mongo.Database, error) {
	if err := s.dbManager.Initialize(); err != nil {
		return nil, nil, err
	}
	mysqlDB, err := s.dbManager.GetMySQLDB()
	if err != nil {
		return nil, nil, err
	}
	mongoDB, err := s.dbManager.GetMongoDB()
	if err != nil {
		return nil, nil, err
	}
	return mysqlDB, mongoDB, nil
}

func (s *apiServer) configureBackpressure() {
	if s.config.Backpressure == nil {
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

func (s *apiServer) initializeRedisRuntime() (redis.UniversalClient, prepareRedisHandles) {
	redisCache, err := s.dbManager.GetRedisClient()
	if err != nil {
		logger.L(context.Background()).Warnw("Cache Redis not available",
			"component", "apiserver",
			"error", err.Error(),
		)
	}
	s.redisCache = redisCache

	runtimeCatalog := redisplane.CatalogFromOptions(s.config.RedisRuntime, nil)
	s.redisRuntime = redisplane.NewRuntime("apiserver", s.dbManager, runtimeCatalog, s.familyStatus)
	resolved := s.redisRuntime.ResolveAll(context.Background())
	handles := prepareRedisHandles{
		static: resolved[redisplane.FamilyStatic],
		object: resolved[redisplane.FamilyObject],
		query:  resolved[redisplane.FamilyQuery],
		meta:   resolved[redisplane.FamilyMeta],
		sdk:    resolved[redisplane.FamilySDK],
		lock:   resolved[redisplane.FamilyLock],
	}
	s.lockHandle = handles.lock
	s.lockManager = redislock.NewManager("apiserver", "lock_lease", handles.lock)
	s.warnMetaCacheAvailability(handles.meta)
	return redisCache, handles
}

func (s *apiServer) warnMetaCacheAvailability(metaHandle *redisplane.Handle) {
	metaRedisCache := redisHandleClient(metaHandle)
	if s.config.Cache != nil &&
		s.config.Cache.Warmup != nil &&
		s.config.Cache.Warmup.Hotset != nil &&
		s.config.Cache.Warmup.Hotset.Enable &&
		metaRedisCache == nil {
		logger.L(context.Background()).Warnw("meta_cache unavailable while hotset governance is enabled; hotset recording and hot-target warmup will degrade",
			"component", "apiserver",
			"family", string(redisplane.FamilyMeta),
			"profile", metaHandleProfile(metaHandle),
		)
	}
	if metaRedisCache == nil {
		logger.L(context.Background()).Warnw("meta_cache unavailable; version-token query caches will run uncached where required",
			"component", "apiserver",
			"family", string(redisplane.FamilyMeta),
			"profile", metaHandleProfile(metaHandle),
		)
	}
}

func (s *apiServer) createMQPublisher() (messaging.Publisher, eventconfig.PublishMode) {
	publishMode := eventconfig.PublishModeFromEnv(s.config.GenericServerRunOptions.Mode)
	if s.config.MessagingOptions == nil || !s.config.MessagingOptions.Enabled {
		return nil, publishMode
	}

	publisher, err := s.config.MessagingOptions.NewPublisher()
	if err != nil {
		logger.L(context.Background()).Warnw("Failed to create MQ publisher, falling back to logging mode",
			"component", "apiserver",
			"error", err.Error(),
		)
		return nil, publishMode
	}
	logger.L(context.Background()).Infow("MQ publisher created successfully",
		"component", "apiserver",
		"provider", s.config.MessagingOptions.Provider,
	)
	return publisher, eventconfig.PublishModeMQ
}

func (s *apiServer) buildContainerOptions(resources *prepareResources) container.ContainerOptions {
	return container.ContainerOptions{
		MQPublisher:                resources.mqPublisher,
		PublisherMode:              resources.publishMode,
		Cache:                      s.buildContainerCacheOptions(),
		StaticRedisHandle:          resources.redisHandles.static,
		ObjectRedisHandle:          resources.redisHandles.object,
		QueryRedisHandle:           resources.redisHandles.query,
		MetaRedisHandle:            resources.redisHandles.meta,
		SDKRedisHandle:             resources.redisHandles.sdk,
		LockRedisHandle:            resources.redisHandles.lock,
		PlanEntryBaseURL:           s.config.Plan.EntryBaseURL,
		StatisticsRepairWindowDays: statisticsRepairWindowDays(s.config),
	}
}

func (s *apiServer) buildContainerCacheOptions() container.ContainerCacheOptions {
	cacheCfg := s.config.Cache
	if cacheCfg == nil {
		return container.ContainerCacheOptions{}
	}

	var ttl container.ContainerCacheTTLOptions
	if cacheCfg.TTL != nil {
		ttl = container.ContainerCacheTTLOptions{
			Scale:            cacheCfg.TTL.Scale,
			ScaleList:        cacheCfg.TTL.ScaleList,
			Questionnaire:    cacheCfg.TTL.Questionnaire,
			AssessmentDetail: cacheCfg.TTL.AssessmentDetail,
			AssessmentList:   cacheCfg.TTL.AssessmentList,
			Testee:           cacheCfg.TTL.Testee,
			Plan:             cacheCfg.TTL.Plan,
			Negative:         cacheCfg.TTL.Negative,
		}
	}

	return container.ContainerCacheOptions{
		DisableEvaluationCache: cacheCfg.DisableEvaluationCache,
		DisableStatisticsCache: cacheCfg.DisableStatisticsCache,
		TTL:                    ttl,
		TTLJitterRatio:         cacheCfg.TTLJitterRatio,
		StatisticsWarmup:       buildStatisticsWarmupConfig(cacheCfg),
		Warmup:                 buildWarmupOptions(cacheCfg),
		CompressPayload:        cacheCfg.CompressPayload,
		Static:                 buildCacheFamilyOptions(cacheCfg.Static),
		Object:                 buildCacheFamilyOptions(cacheCfg.Object),
		Query:                  buildQueryFamilyOptions(cacheCfg.Query),
		Meta:                   container.ContainerCacheFamilyOptions{},
		SDK:                    buildCacheFamilyOptions(cacheCfg.SDK),
		Lock:                   buildCacheFamilyOptions(cacheCfg.Lock),
	}
}

func buildStatisticsWarmupConfig(cacheCfg *apiserveroptions.CacheOptions) *cachegov.StatisticsWarmupConfig {
	if cacheCfg == nil || cacheCfg.StatisticsWarmup == nil || !cacheCfg.StatisticsWarmup.Enable {
		return nil
	}
	return &cachegov.StatisticsWarmupConfig{
		OrgIDs:             cacheCfg.StatisticsWarmup.OrgIDs,
		QuestionnaireCodes: cacheCfg.StatisticsWarmup.QuestionnaireCodes,
		PlanIDs:            cacheCfg.StatisticsWarmup.PlanIDs,
	}
}

func buildWarmupOptions(cacheCfg *apiserveroptions.CacheOptions) container.ContainerWarmupOptions {
	if cacheCfg == nil || cacheCfg.Warmup == nil {
		return container.ContainerWarmupOptions{}
	}
	options := container.ContainerWarmupOptions{
		Enable: cacheCfg.Warmup.Enable,
	}
	if cacheCfg.Warmup.Startup != nil {
		options.StartupStatic = cacheCfg.Warmup.Startup.Static
		options.StartupQuery = cacheCfg.Warmup.Startup.Query
	}
	if cacheCfg.Warmup.Hotset != nil {
		options.HotsetEnable = cacheCfg.Warmup.Hotset.Enable
		options.HotsetTopN = cacheCfg.Warmup.Hotset.TopN
		options.MaxItemsPerKind = cacheCfg.Warmup.Hotset.MaxItemsPerKind
	}
	return options
}

func buildCacheFamilyOptions(family *apiserveroptions.CacheFamilyOptions) container.ContainerCacheFamilyOptions {
	if family == nil {
		return container.ContainerCacheFamilyOptions{}
	}
	return container.ContainerCacheFamilyOptions{
		NegativeTTL:    family.NegativeTTL,
		TTLJitterRatio: family.TTLJitterRatio,
		Compress:       family.Compress,
		Singleflight:   family.Singleflight,
		Negative:       family.Negative,
	}
}

func buildQueryFamilyOptions(family *apiserveroptions.CacheFamilyOptions) container.ContainerCacheFamilyOptions {
	options := buildCacheFamilyOptions(family)
	if family != nil {
		options.TTL = family.TTL
	}
	return options
}

func statisticsRepairWindowDays(cfg *config.Config) int {
	if cfg.StatisticsSync == nil {
		return 0
	}
	return cfg.StatisticsSync.RepairWindowDays
}

func (s *apiServer) initializeContainer() error {
	ctx := context.Background()
	iamModule, err := container.NewIAMModule(ctx, s.config.IAMOptions)
	if err != nil {
		return err
	}
	s.container.IAMModule = iamModule
	if err := s.container.Initialize(); err != nil {
		return err
	}
	if s.container != nil {
		s.container.CacheGovernanceStatusService = cachegov.NewStatusService("apiserver", s.familyStatus, s.container.HotsetInspector(), s.container.WarmupCoordinator)
		if s.container.StatisticsModule != nil && s.container.StatisticsModule.Handler != nil {
			s.container.StatisticsModule.Handler.SetCacheGovernanceStatusService(s.container.CacheGovernanceStatusService)
		}
	}
	s.startAuthzVersionSync()
	return nil
}

func (s *apiServer) initializeWeChatServices() error {
	if s.config.WeChatOptions == nil || s.container == nil {
		return nil
	}
	s.container.InitQRCodeService(s.config.WeChatOptions)
	s.container.InitMiniProgramTaskNotificationService(s.config.WeChatOptions)
	if s.container.QRCodeService == nil {
		return nil
	}
	if s.container.EvaluationModule != nil {
		s.container.EvaluationModule.SetQRCodeService(s.container.QRCodeService)
	}
	if s.container.SurveyModule != nil {
		s.container.SurveyModule.SetQRCodeService(s.container.QRCodeService)
	}
	if s.container.ScaleModule != nil {
		s.container.ScaleModule.SetQRCodeService(s.container.QRCodeService)
	}
	if s.container.ActorModule != nil {
		s.container.ActorModule.SetQRCodeService(s.container.QRCodeService)
	}
	return nil
}

func (s *apiServer) initializeTransports() error {
	var err error
	s.grpcServer, err = buildGRPCServer(s.config, s.container)
	if err != nil {
		return err
	}
	NewRouter(s.container, s.config.RateLimit).RegisterRoutes(s.genericAPIServer.Engine)
	return NewGRPCRegistry(s.grpcServer, s.container).RegisterServices()
}

func (s *apiServer) logInitialization(hasMongo bool) {
	log.Info("🏗️  Hexagonal Architecture initialized successfully!")
	log.Info("   📦 Domain: questionnaire, user")
	log.Info("   🔌 Ports: storage, document")
	log.Info("   🔧 Adapters: mysql, mongodb, http, grpc")
	log.Info("   📋 Application Services: questionnaire_service, user_service")
	if hasMongo {
		log.Info("   🗄️  Storage Mode: MySQL + MongoDB (Hybrid)")
		return
	}
	log.Info("   🗄️  Storage Mode: MySQL Only")
}

func (s *apiServer) startWarmup() {
	go func() {
		ctx := context.Background()
		if err := s.container.WarmupCache(ctx); err != nil {
			logger.L(ctx).Warnw("Cache warmup failed", "error", err)
		} else {
			logger.L(ctx).Infow("Cache warmup completed")
		}
	}()
}

func (s *apiServer) registerShutdownCallback() {
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		if s.container != nil {
			if err := s.container.Cleanup(); err != nil {
				log.Errorf("Failed to cleanup container resources: %v", err)
			}
		}
		if s.authzVersionSubscriber != nil {
			s.authzVersionSubscriber.Stop()
			if err := s.authzVersionSubscriber.Close(); err != nil {
				log.Errorf("Failed to close IAM authz version subscriber: %v", err)
			}
		}
		if s.dbManager != nil {
			if err := s.dbManager.Close(); err != nil {
				log.Errorf("Failed to close database connections: %v", err)
			}
		}
		s.genericAPIServer.Close()
		s.grpcServer.Close()
		log.Info("🏗️  Hexagonal Architecture server shutdown complete")
		return nil
	}))
}

func (s *apiServer) fatalPrepareRun(action string, err error) {
	logger.L(context.Background()).Errorw("Failed to prepare api server",
		"component", "apiserver",
		"action", action,
		"error", err.Error(),
	)
	log.Fatalf("Failed to %s: %v", action, err)
}

func redisHandleClient(handle *redisplane.Handle) redis.UniversalClient {
	if handle == nil {
		return nil
	}
	return handle.Client
}

func (s *apiServer) startAuthzVersionSync() {
	if s == nil || s.container == nil || s.container.IAMModule == nil {
		return
	}
	loader := s.container.IAMModule.AuthzSnapshotLoader()
	authzSync := s.config.IAMOptions.AuthzSync
	if loader == nil || authzSync == nil || !authzSync.Enabled {
		return
	}

	subscriber, err := authzSync.NewSubscriber()
	if err != nil {
		logger.L(context.Background()).Warnw("Failed to create authz version subscriber",
			"component", "apiserver",
			"error", err.Error(),
		)
		return
	}

	channelPrefix := authzSync.ChannelPrefix
	if channelPrefix == "" {
		channelPrefix = "qs-authz-sync"
	}
	channel := iamauth.DefaultVersionSyncChannel(channelPrefix + "-apiserver")
	if err := iamauth.SubscribeVersionChanges(context.Background(), subscriber, authzSync.Topic, channel, loader); err != nil {
		_ = subscriber.Close()
		logger.L(context.Background()).Warnw("Failed to subscribe IAM authz version sync",
			"component", "apiserver",
			"error", err.Error(),
			"channel", channel,
			"topic", authzSync.Topic,
		)
		return
	}
	s.authzVersionSubscriber = subscriber
}

// Run 运行 API 服务器
func (s preparedAPIServer) Run() error {
	// 启动关闭管理器
	if err := s.gs.Start(); err != nil {
		log.Fatalf("start shutdown manager failed: %s", err.Error())
	}

	// 创建一个 channel 用于接收错误
	errChan := make(chan error, 2)

	// 启动 HTTP 服务器
	go func() {
		if err := s.genericAPIServer.Run(); err != nil {
			log.Errorf("Failed to run HTTP server: %v", err)
			errChan <- err
		}
	}()
	log.Info("🚀 Starting Hexagonal Architecture HTTP REST API server...")

	// 启动 GRPC 服务器
	go func() {
		if err := s.grpcServer.Run(); err != nil {
			log.Errorf("Failed to run GRPC server: %v", err)
			errChan <- err
		}
	}()
	log.Info("🚀 Starting Hexagonal Architecture GRPC server...")

	// 等待任一服务出错
	return <-errChan
}

// startSchedulers 启动 apiserver 内建调度器。
func (s *apiServer) startSchedulers() {
	if s == nil || s.gs == nil || s.container == nil {
		return
	}

	var (
		lockBuilder       *rediskey.Builder
		planCommand       planApp.PlanCommandService
		statisticsSyncSvc statisticsApp.StatisticsSyncService
		behaviorProjector statisticsApp.BehaviorProjectorService
	)
	if s.lockHandle != nil {
		lockBuilder = s.lockHandle.Builder
	}
	if s.container.PlanModule != nil {
		planCommand = s.container.PlanModule.CommandService
	}
	if s.container.StatisticsModule != nil {
		statisticsSyncSvc = s.container.StatisticsModule.SyncService
		behaviorProjector = s.container.StatisticsModule.BehaviorProjectorService
	}

	manager := runtimescheduler.NewManager(
		runtimescheduler.NewPlanRunner(
			s.config.PlanScheduler,
			s.lockManager,
			planCommand,
			lockBuilder,
		),
		runtimescheduler.NewStatisticsSyncRunner(
			s.config.StatisticsSync,
			statisticsSyncSvc,
			s.container.WarmupCoordinator,
			s.lockManager,
			lockBuilder,
		),
		runtimescheduler.NewBehaviorPendingReconcileRunner(
			s.config.BehaviorPendingReconcile,
			behaviorProjector,
			s.lockManager,
			lockBuilder,
		),
	)
	if manager.Len() == 0 {
		log.Infof("no built-in apiserver schedulers enabled")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		cancel()
		return nil
	}))

	manager.Start(ctx)
	log.Infof("apiserver scheduler manager started (runner_count=%d)", manager.Len())
}

// startMongoOutboxRelay 启动 Mongo outbox relay（answersheet/report success events）。
func (s *apiServer) startMongoOutboxRelay() {
	if s.container == nil || s.container.SurveyModule == nil || s.container.SurveyModule.AnswerSheet == nil {
		return
	}

	relay := s.container.SurveyModule.AnswerSheet.SubmittedEventRelay
	if relay == nil {
		return
	}

	const interval = 2 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		cancel()
		return nil
	}))

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			if err := relay.DispatchDue(ctx); err != nil {
				log.Warnf("answersheet submitted outbox relay failed: %v", err)
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	log.Infof("mongo outbox relay started (interval=%s)", interval)
}

// startAssessmentOutboxRelay 启动 MySQL outbox relay（assessment submitted/failed）。
func (s *apiServer) startAssessmentOutboxRelay() {
	if s.container == nil || s.container.EvaluationModule == nil {
		return
	}

	relay := s.container.EvaluationModule.AssessmentOutboxRelay
	if relay == nil {
		return
	}

	const interval = 2 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		cancel()
		return nil
	}))

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			if err := relay.DispatchDue(ctx); err != nil {
				log.Warnf("assessment outbox relay failed: %v", err)
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	log.Infof("assessment outbox relay started (interval=%s)", interval)
}

// buildGenericServer 构建通用服务器
func buildGenericServer(cfg *config.Config) (*genericapiserver.GenericAPIServer, error) {
	// 构建通用配置
	genericConfig, err := buildGenericConfig(cfg)
	if err != nil {
		return nil, err
	}

	// 完成通用配置并创建实例
	genericServer, err := genericConfig.Complete().New()
	if err != nil {
		return nil, err
	}

	return genericServer, nil
}

// buildGenericConfig 构建通用配置
func buildGenericConfig(cfg *config.Config) (genericConfig *genericapiserver.Config, lastErr error) {
	genericConfig = genericapiserver.NewConfig()

	// 应用通用配置
	if lastErr = cfg.GenericServerRunOptions.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	// 应用安全配置
	if lastErr = cfg.SecureServing.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	// 应用不安全配置
	if lastErr = cfg.InsecureServing.ApplyTo(genericConfig); lastErr != nil {
		return
	}
	return
}

// buildGRPCServer 构建 GRPC 服务器（使用 component-base 提供的能力）
func buildGRPCServer(cfg *config.Config, container *container.Container) (*grpcpkg.Server, error) {
	// 创建 GRPC 配置
	grpcConfig := grpcpkg.NewConfig()

	// 应用配置选项
	if err := applyGRPCOptions(cfg, grpcConfig); err != nil {
		return nil, err
	}

	if loader := container.IAMModule.AuthzSnapshotLoader(); loader != nil {
		var operatorRepo domainoperator.Repository
		if container.ActorModule != nil {
			operatorRepo = container.ActorModule.OperatorRepo
		}
		// 授权快照拦截器只负责权限视图，不替代前面的 JWT 权威在线校验。
		grpcConfig.ExtraUnaryAfterAuth = append(grpcConfig.ExtraUnaryAfterAuth,
			NewAuthzSnapshotUnaryInterceptor(loader, operatorRepo))
		log.Info("gRPC server: IAM authorization snapshot interceptor enabled (after JWT auth)")
	}

	// 获取 SDK TokenVerifier（使用 SDK 的本地 JWKS 验签能力）
	tokenVerifier := container.IAMModule.SDKTokenVerifier()
	if tokenVerifier != nil {
		log.Info("gRPC server: TokenVerifier injected for authentication (local JWKS verification)")
	} else {
		log.Warn("gRPC server: TokenVerifier not available, authentication disabled")
	}

	// 完成配置并创建服务器
	return grpcConfig.Complete().New(tokenVerifier)
}

// applyGRPCOptions 应用 GRPC 选项到配置
func applyGRPCOptions(cfg *config.Config, grpcConfig *grpcpkg.Config) error {
	opts := cfg.GRPCOptions

	// 应用基本配置
	grpcConfig.BindAddress = opts.BindAddress
	grpcConfig.BindPort = opts.BindPort
	grpcConfig.Insecure = opts.Insecure

	// 应用 TLS 配置
	grpcConfig.TLSCertFile = opts.TLSCertFile
	grpcConfig.TLSKeyFile = opts.TLSKeyFile

	// 应用消息和连接配置
	grpcConfig.MaxMsgSize = opts.MaxMsgSize
	grpcConfig.MaxConnectionAge = opts.MaxConnectionAge
	grpcConfig.MaxConnectionAgeGrace = opts.MaxConnectionAgeGrace

	// 应用 mTLS 配置
	if opts.MTLS != nil {
		grpcConfig.MTLS.Enabled = opts.MTLS.Enabled
		grpcConfig.MTLS.CAFile = opts.MTLS.CAFile
		grpcConfig.MTLS.RequireClientCert = opts.MTLS.RequireClientCert
		grpcConfig.MTLS.AllowedCNs = opts.MTLS.AllowedCNs
		grpcConfig.MTLS.AllowedOUs = opts.MTLS.AllowedOUs
		grpcConfig.MTLS.MinTLSVersion = opts.MTLS.MinTLSVersion
	}

	// 应用认证配置
	if opts.Auth != nil {
		grpcConfig.Auth.Enabled = opts.Auth.Enabled
	}
	if cfg.IAMOptions != nil && cfg.IAMOptions.JWT != nil {
		grpcConfig.Auth.ForceRemoteVerification = cfg.IAMOptions.JWT.ForceRemoteVerification
	}

	// 应用 ACL 配置
	if opts.ACL != nil {
		grpcConfig.ACL.Enabled = opts.ACL.Enabled
	}

	// 应用审计配置
	if opts.Audit != nil {
		grpcConfig.Audit.Enabled = opts.Audit.Enabled
	}

	// 应用功能开关
	grpcConfig.EnableReflection = opts.EnableReflection
	grpcConfig.EnableHealthCheck = opts.EnableHealthCheck

	return nil
}

func metaHandleProfile(handle *redisplane.Handle) string {
	if handle == nil {
		return ""
	}
	return handle.Profile
}
