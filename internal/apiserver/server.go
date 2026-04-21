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
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	domainoperator "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	infraIAM "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	infraMongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	mysqlbp "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	grpcpkg "github.com/FangcunMount/qs-server/internal/pkg/grpc"
	iamauth "github.com/FangcunMount/qs-server/internal/pkg/iamauth"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	genericapiserver "github.com/FangcunMount/qs-server/internal/pkg/server"
	redis "github.com/redis/go-redis/v9"
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
	// 初始化数据库连接
	if err := s.dbManager.Initialize(); err != nil {
		logger.L(context.Background()).Errorw("Failed to initialize database",
			"component", "apiserver",
			"error", err.Error(),
		)
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// 获取 MySQL 数据库连接
	mysqlDB, err := s.dbManager.GetMySQLDB()
	if err != nil {
		logger.L(context.Background()).Errorw("Failed to get MySQL connection",
			"component", "apiserver",
			"error", err.Error(),
		)
		log.Fatalf("Failed to get MySQL connection: %v", err)
	}

	// 获取 MongoDB 数据库链接
	mongoDB, err := s.dbManager.GetMongoDB()
	if err != nil {
		logger.L(context.Background()).Errorw("Failed to get MongoDB connection",
			"component", "apiserver",
			"error", err.Error(),
		)
		log.Fatalf("Failed to get MongoDB connection: %v", err)
	}

	// 初始化下游背压（MySQL/Mongo/IAM）
	if s.config.Backpressure != nil {
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

	// 获取 Redis 客户端（cache/store）
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
	handles := s.redisRuntime.ResolveAll(context.Background())
	staticHandle := handles[redisplane.FamilyStatic]
	objectHandle := handles[redisplane.FamilyObject]
	queryHandle := handles[redisplane.FamilyQuery]
	metaHandle := handles[redisplane.FamilyMeta]
	sdkHandle := handles[redisplane.FamilySDK]
	lockHandle := handles[redisplane.FamilyLock]
	metaRedisCache := redisHandleClient(metaHandle)
	_ = redisHandleClient(lockHandle)
	if s.config.Cache != nil && s.config.Cache.Warmup != nil && s.config.Cache.Warmup.Hotset != nil && s.config.Cache.Warmup.Hotset.Enable && metaRedisCache == nil {
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

	// 创建消息队列 publisher（用于事件发布）
	var mqPublisher messaging.Publisher
	publishMode := eventconfig.PublishModeFromEnv(s.config.GenericServerRunOptions.Mode)
	if s.config.MessagingOptions != nil && s.config.MessagingOptions.Enabled {
		mqPublisher, err = s.config.MessagingOptions.NewPublisher()
		if err != nil {
			logger.L(context.Background()).Warnw("Failed to create MQ publisher, falling back to logging mode",
				"component", "apiserver",
				"error", err.Error(),
			)
			mqPublisher = nil
		} else {
			logger.L(context.Background()).Infow("MQ publisher created successfully",
				"component", "apiserver",
				"provider", s.config.MessagingOptions.Provider,
			)
			// 明确开启 MQ 发布模式（避免因 server.mode=release 被误判为 logging）
			publishMode = eventconfig.PublishModeMQ
		}
	}

	// 创建六边形架构容器（使用 MQ 模式）
	var cacheTTLOpts container.ContainerCacheTTLOptions
	var cacheTTLJitter float64
	if s.config.Cache != nil && s.config.Cache.TTL != nil {
		cacheTTLOpts = container.ContainerCacheTTLOptions{
			Scale:            s.config.Cache.TTL.Scale,
			ScaleList:        s.config.Cache.TTL.ScaleList,
			Questionnaire:    s.config.Cache.TTL.Questionnaire,
			AssessmentDetail: s.config.Cache.TTL.AssessmentDetail,
			AssessmentList:   s.config.Cache.TTL.AssessmentList,
			Testee:           s.config.Cache.TTL.Testee,
			Plan:             s.config.Cache.TTL.Plan,
			Negative:         s.config.Cache.TTL.Negative,
		}
		cacheTTLJitter = s.config.Cache.TTLJitterRatio
	}
	var statsWarmupCfg *cachegov.StatisticsWarmupConfig
	if s.config.Cache != nil && s.config.Cache.StatisticsWarmup != nil && s.config.Cache.StatisticsWarmup.Enable {
		statsWarmupCfg = &cachegov.StatisticsWarmupConfig{
			OrgIDs:             s.config.Cache.StatisticsWarmup.OrgIDs,
			QuestionnaireCodes: s.config.Cache.StatisticsWarmup.QuestionnaireCodes,
			PlanIDs:            s.config.Cache.StatisticsWarmup.PlanIDs,
		}
	}
	s.container = container.NewContainerWithOptions(
		mysqlDB, mongoDB, redisCache,
		container.ContainerOptions{
			MQPublisher:   mqPublisher,
			PublisherMode: publishMode,
			Cache: container.ContainerCacheOptions{
				DisableEvaluationCache: s.config.Cache != nil && s.config.Cache.DisableEvaluationCache,
				DisableStatisticsCache: s.config.Cache != nil && s.config.Cache.DisableStatisticsCache,
				TTL:                    cacheTTLOpts,
				TTLJitterRatio:         cacheTTLJitter,
				StatisticsWarmup:       statsWarmupCfg,
				Warmup: container.ContainerWarmupOptions{
					Enable:        s.config.Cache != nil && s.config.Cache.Warmup != nil && s.config.Cache.Warmup.Enable,
					StartupStatic: s.config.Cache != nil && s.config.Cache.Warmup != nil && s.config.Cache.Warmup.Startup != nil && s.config.Cache.Warmup.Startup.Static,
					StartupQuery:  s.config.Cache != nil && s.config.Cache.Warmup != nil && s.config.Cache.Warmup.Startup != nil && s.config.Cache.Warmup.Startup.Query,
					HotsetEnable:  s.config.Cache != nil && s.config.Cache.Warmup != nil && s.config.Cache.Warmup.Hotset != nil && s.config.Cache.Warmup.Hotset.Enable,
					HotsetTopN: func() int64 {
						if s.config.Cache == nil || s.config.Cache.Warmup == nil || s.config.Cache.Warmup.Hotset == nil {
							return 0
						}
						return s.config.Cache.Warmup.Hotset.TopN
					}(),
					MaxItemsPerKind: func() int64 {
						if s.config.Cache == nil || s.config.Cache.Warmup == nil || s.config.Cache.Warmup.Hotset == nil {
							return 0
						}
						return s.config.Cache.Warmup.Hotset.MaxItemsPerKind
					}(),
				},
				CompressPayload: s.config.Cache.CompressPayload,
				Static: container.ContainerCacheFamilyOptions{
					NegativeTTL:    s.config.Cache.Static.NegativeTTL,
					TTLJitterRatio: s.config.Cache.Static.TTLJitterRatio,
					Compress:       s.config.Cache.Static.Compress,
					Singleflight:   s.config.Cache.Static.Singleflight,
					Negative:       s.config.Cache.Static.Negative,
				},
				Object: container.ContainerCacheFamilyOptions{
					NegativeTTL:    s.config.Cache.Object.NegativeTTL,
					TTLJitterRatio: s.config.Cache.Object.TTLJitterRatio,
					Compress:       s.config.Cache.Object.Compress,
					Singleflight:   s.config.Cache.Object.Singleflight,
					Negative:       s.config.Cache.Object.Negative,
				},
				Query: container.ContainerCacheFamilyOptions{
					TTL:            s.config.Cache.Query.TTL,
					NegativeTTL:    s.config.Cache.Query.NegativeTTL,
					TTLJitterRatio: s.config.Cache.Query.TTLJitterRatio,
					Compress:       s.config.Cache.Query.Compress,
					Singleflight:   s.config.Cache.Query.Singleflight,
					Negative:       s.config.Cache.Query.Negative,
				},
				Meta: container.ContainerCacheFamilyOptions{},
				SDK: container.ContainerCacheFamilyOptions{
					NegativeTTL:    s.config.Cache.SDK.NegativeTTL,
					TTLJitterRatio: s.config.Cache.SDK.TTLJitterRatio,
					Compress:       s.config.Cache.SDK.Compress,
					Singleflight:   s.config.Cache.SDK.Singleflight,
					Negative:       s.config.Cache.SDK.Negative,
				},
				Lock: container.ContainerCacheFamilyOptions{
					NegativeTTL:    s.config.Cache.Lock.NegativeTTL,
					TTLJitterRatio: s.config.Cache.Lock.TTLJitterRatio,
					Compress:       s.config.Cache.Lock.Compress,
					Singleflight:   s.config.Cache.Lock.Singleflight,
					Negative:       s.config.Cache.Lock.Negative,
				},
			},
			StaticRedisHandle: staticHandle,
			ObjectRedisHandle: objectHandle,
			QueryRedisHandle:  queryHandle,
			MetaRedisHandle:   metaHandle,
			SDKRedisHandle:    sdkHandle,
			LockRedisHandle:   lockHandle,
			PlanEntryBaseURL:  s.config.Plan.EntryBaseURL,
			StatisticsRepairWindowDays: func() int {
				if s.config.StatisticsSync == nil {
					return 0
				}
				return s.config.StatisticsSync.RepairWindowDays
			}(),
		},
	)
	// 初始化 IAM 模块（优先）
	ctx := context.Background()
	iamModule, err := container.NewIAMModule(ctx, s.config.IAMOptions)
	if err != nil {
		logger.L(context.Background()).Errorw("Failed to initialize IAM module",
			"component", "apiserver",
			"error", err.Error(),
		)
		log.Fatalf("Failed to initialize IAM module: %v", err)
	}
	s.container.IAMModule = iamModule

	// 初始化容器中的所有组件
	if err := s.container.Initialize(); err != nil {
		log.Fatalf("Failed to initialize hexagonal architecture container: %v", err)
	}
	if s.container != nil {
		s.container.CacheGovernanceStatusService = cachegov.NewStatusService("apiserver", s.familyStatus, s.container.HotsetInspector(), s.container.WarmupCoordinator)
		if s.container.StatisticsModule != nil && s.container.StatisticsModule.Handler != nil {
			s.container.StatisticsModule.Handler.SetCacheGovernanceStatusService(s.container.CacheGovernanceStatusService)
		}
	}

	s.startAuthzVersionSync()

	// 初始化小程序码生成服务（从配置读取 wechat_app_id，然后从 IAM 查询）
	if s.config.WeChatOptions != nil {
		s.container.InitQRCodeService(s.config.WeChatOptions)
		s.container.InitMiniProgramTaskNotificationService(s.config.WeChatOptions)
		// 将 QRCodeService 注入到各个模块
		if s.container.QRCodeService != nil {
			// 注入到 EvaluationModule
			if s.container.EvaluationModule != nil {
				s.container.EvaluationModule.SetQRCodeService(s.container.QRCodeService)
			}
			// 注入到 SurveyModule（问卷）
			if s.container.SurveyModule != nil {
				s.container.SurveyModule.SetQRCodeService(s.container.QRCodeService)
			}
			// 注入到 ScaleModule（量表）
			if s.container.ScaleModule != nil {
				s.container.ScaleModule.SetQRCodeService(s.container.QRCodeService)
			}
			// 注入到 ActorModule（测评入口二维码）
			if s.container.ActorModule != nil {
				s.container.ActorModule.SetQRCodeService(s.container.QRCodeService)
			}
		}
	}

	// 现在创建 GRPC 服务器（IAM Module 已初始化）
	s.grpcServer, err = buildGRPCServer(s.config, s.container)
	if err != nil {
		log.Fatalf("Failed to build GRPC server: %v", err)
	}

	// 创建并初始化路由器
	NewRouter(s.container, s.config.RateLimit).RegisterRoutes(s.genericAPIServer.Engine)

	// 注册 GRPC 服务
	if err := NewGRPCRegistry(s.grpcServer, s.container).RegisterServices(); err != nil {
		log.Fatalf("Failed to register GRPC services: %v", err)
	}

	log.Info("🏗️  Hexagonal Architecture initialized successfully!")
	log.Info("   📦 Domain: questionnaire, user")
	log.Info("   🔌 Ports: storage, document")
	log.Info("   🔧 Adapters: mysql, mongodb, http, grpc")
	log.Info("   📋 Application Services: questionnaire_service, user_service")

	if mongoDB != nil {
		log.Info("   🗄️  Storage Mode: MySQL + MongoDB (Hybrid)")
	} else {
		log.Info("   🗄️  Storage Mode: MySQL Only")
	}

	// 异步预热缓存（不阻塞服务启动）
	go func() {
		ctx := context.Background()
		if err := s.container.WarmupCache(ctx); err != nil {
			logger.L(ctx).Warnw("Cache warmup failed", "error", err)
		} else {
			logger.L(ctx).Infow("Cache warmup completed")
		}
	}()

	// 启动统计同步定时任务（Redis -> MySQL），最终一致
	s.startStatisticsSyncScheduler()
	s.startMongoOutboxRelay()
	s.startAssessmentOutboxRelay()
	s.startBehaviorPendingReconcile()

	// 添加关闭回调
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		// 清理容器资源
		if s.container != nil {
			s.container.Cleanup()
		}

		if s.authzVersionSubscriber != nil {
			s.authzVersionSubscriber.Stop()
			if err := s.authzVersionSubscriber.Close(); err != nil {
				log.Errorf("Failed to close IAM authz version subscriber: %v", err)
			}
		}

		// 关闭数据库连接
		if s.dbManager != nil {
			if err := s.dbManager.Close(); err != nil {
				log.Errorf("Failed to close database connections: %v", err)
			}
		}

		// 关闭 HTTP 服务器
		s.genericAPIServer.Close()

		// 关闭 GRPC 服务器
		s.grpcServer.Close()

		log.Info("🏗️  Hexagonal Architecture server shutdown complete")
		return nil
	}))

	return preparedAPIServer{s}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
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

// startStatisticsSyncScheduler 启动统计同步定时任务（夜间批处理）。
func (s *apiServer) startStatisticsSyncScheduler() {
	opts := s.config.StatisticsSync
	if opts == nil || !opts.Enable {
		log.Infof("statistics sync scheduler disabled")
		return
	}
	if s.container == nil || s.container.StatisticsModule == nil || s.container.StatisticsModule.SyncService == nil {
		log.Warnf("statistics sync scheduler not started (module or sync service unavailable)")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		cancel()
		return nil
	}))

	runAt, err := parseStatisticsSyncRunAt(opts.RunAt)
	if err != nil {
		log.Warnf("statistics sync scheduler disabled: invalid run_at %q: %v", opts.RunAt, err)
		return
	}

	syncSvc := s.container.StatisticsModule.SyncService
	go func() {
		for {
			now := time.Now().In(time.Local)
			nextRun := nextStatisticsSyncRun(now, runAt.hour, runAt.minute)
			timer := time.NewTimer(time.Until(nextRun))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}

			for _, orgID := range opts.OrgIDs {
				orgCtx := context.Background()
				start, end := statisticsSyncRepairWindow(time.Now().In(time.Local), opts.RepairWindowDays)
				dailyOpts := statisticsApp.SyncDailyOptions{StartDate: &start, EndDate: &end}
				if err := syncSvc.SyncDailyStatistics(orgCtx, orgID, dailyOpts); err != nil {
					log.Warnf("statistics nightly daily sync failed (org=%d): %v", orgID, err)
					continue
				}
				if err := syncSvc.SyncAccumulatedStatistics(orgCtx, orgID); err != nil {
					log.Warnf("statistics nightly accumulated sync failed (org=%d): %v", orgID, err)
					continue
				}
				if err := syncSvc.SyncPlanStatistics(orgCtx, orgID); err != nil {
					log.Warnf("statistics nightly plan sync failed (org=%d): %v", orgID, err)
					continue
				}
				if s.container != nil && s.container.WarmupCoordinator != nil {
					if err := s.container.WarmupCoordinator.HandleStatisticsSync(orgCtx, orgID); err != nil {
						log.Warnf("statistics nightly cache warmup failed (org=%d): %v", orgID, err)
					}
				}
			}
		}
	}()

	log.Infof("statistics sync scheduler started (org_ids=%v, run_at=%s, repair_window_days=%d)",
		opts.OrgIDs, opts.RunAt, opts.RepairWindowDays)
}

type statisticsSyncClock struct {
	hour   int
	minute int
}

func parseStatisticsSyncRunAt(raw string) (statisticsSyncClock, error) {
	parsed, err := time.ParseInLocation("15:04", raw, time.Local)
	if err != nil {
		return statisticsSyncClock{}, err
	}
	return statisticsSyncClock{hour: parsed.Hour(), minute: parsed.Minute()}, nil
}

func nextStatisticsSyncRun(now time.Time, hour, minute int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func statisticsSyncRepairWindow(now time.Time, repairWindowDays int) (time.Time, time.Time) {
	if repairWindowDays <= 0 {
		repairWindowDays = 7
	}
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return todayStart.AddDate(0, 0, -repairWindowDays), todayStart
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

// startBehaviorPendingReconcile 启动 behavior pending 事件归因重试任务。
func (s *apiServer) startBehaviorPendingReconcile() {
	if s.container == nil || s.container.StatisticsModule == nil {
		return
	}

	projector := s.container.StatisticsModule.BehaviorProjectorService
	if projector == nil {
		return
	}

	const (
		interval = 10 * time.Second
		limit    = 100
	)

	ctx, cancel := context.WithCancel(context.Background())
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		cancel()
		return nil
	}))

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			if _, err := projector.ReconcilePendingBehaviorEvents(ctx, limit); err != nil {
				log.Warnf("behavior pending reconcile failed: %v", err)
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	log.Infof("behavior pending reconcile started (interval=%s, limit=%d)", interval, limit)
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
