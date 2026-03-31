package apiserver

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/component-base/pkg/shutdown"
	"github.com/FangcunMount/component-base/pkg/shutdown/shutdownmanagers/posixsignal"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	scaleCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	infraIAM "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	infraMongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	mysqlbp "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	grpcpkg "github.com/FangcunMount/qs-server/internal/pkg/grpc"
	genericapiserver "github.com/FangcunMount/qs-server/internal/pkg/server"
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
	// Container 主容器
	container *container.Container
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
		grpcServer:       nil, // 延迟初始化
		config:           cfg,
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
			Questionnaire:    s.config.Cache.TTL.Questionnaire,
			AssessmentDetail: s.config.Cache.TTL.AssessmentDetail,
			AssessmentStatus: s.config.Cache.TTL.AssessmentStatus,
			Testee:           s.config.Cache.TTL.Testee,
			Plan:             s.config.Cache.TTL.Plan,
			Negative:         s.config.Cache.TTL.Negative,
		}
		cacheTTLJitter = s.config.Cache.TTLJitterRatio
	}
	var statsWarmupCfg *scaleCache.StatisticsWarmupConfig
	if s.config.Cache != nil && s.config.Cache.StatisticsWarmup != nil && s.config.Cache.StatisticsWarmup.Enable {
		statsWarmupCfg = &scaleCache.StatisticsWarmupConfig{
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
				Namespace:              s.config.Cache.Namespace,
				CompressPayload:        s.config.Cache.CompressPayload,
			},
			PlanEntryBaseURL: s.config.Plan.EntryBaseURL,
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

	// 初始化小程序码生成服务（从配置读取 wechat_app_id，然后从 IAM 查询）
	if s.config.WeChatOptions != nil {
		s.container.InitQRCodeService(s.config.WeChatOptions)
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

	// 添加关闭回调
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		// 清理容器资源
		if s.container != nil {
			s.container.Cleanup()
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

// startStatisticsSyncScheduler 启动统计同步定时任务（Redis -> MySQL）
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

	startTicker := func(name string, interval time.Duration, fn func(context.Context) error) {
		if interval <= 0 {
			log.Warnf("skip statistics sync %s: interval <= 0", name)
			return
		}
		go func() {
			// 统一初始延迟
			if opts.InitialDelay > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(opts.InitialDelay):
				}
			}
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			for {
				if err := fn(context.Background()); err != nil {
					log.Warnf("statistics sync %s failed: %v", name, err)
				}

				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	}

	syncSvc := s.container.StatisticsModule.SyncService
	for _, orgID := range opts.OrgIDs {
		scheduledOrgID := orgID
		startTicker(fmt.Sprintf("daily(org=%d)", scheduledOrgID), opts.DailyInterval, func(ctx context.Context) error {
			return syncSvc.SyncDailyStatistics(ctx, scheduledOrgID)
		})
		startTicker(fmt.Sprintf("accumulated(org=%d)", scheduledOrgID), opts.AccumulatedInterval, func(ctx context.Context) error {
			return syncSvc.SyncAccumulatedStatistics(ctx, scheduledOrgID)
		})
		startTicker(fmt.Sprintf("plan(org=%d)", scheduledOrgID), opts.PlanInterval, func(ctx context.Context) error {
			return syncSvc.SyncPlanStatistics(ctx, scheduledOrgID)
		})
	}

	log.Infof("statistics sync scheduler started (org_ids=%v, daily=%s, accum=%s, plan=%s, initial_delay=%s)",
		opts.OrgIDs, opts.DailyInterval, opts.AccumulatedInterval, opts.PlanInterval, opts.InitialDelay)
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
