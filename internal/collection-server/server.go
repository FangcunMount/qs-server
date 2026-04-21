package collection

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/component-base/pkg/shutdown"
	"github.com/FangcunMount/component-base/pkg/shutdown/shutdownmanagers/posixsignal"
	"github.com/FangcunMount/qs-server/internal/collection-server/config"
	"github.com/FangcunMount/qs-server/internal/collection-server/container"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	iamauth "github.com/FangcunMount/qs-server/internal/pkg/iamauth"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	genericapiserver "github.com/FangcunMount/qs-server/internal/pkg/server"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/credentials"
)

// collectionServer 定义了 Collection 服务器的基本结构
type collectionServer struct {
	// 优雅关闭管理器
	gs *shutdown.GracefulShutdown
	// 通用 API 服务器
	genericAPIServer *genericapiserver.GenericAPIServer
	// 配置
	config *config.Config
	// 数据库/Redis manager
	dbManager *DatabaseManager
	// 共享 Redis runtime / family status
	familyStatus *cacheobservability.FamilyStatusRegistry
	redisRuntime *redisplane.Runtime
	opsHandle    *redisplane.Handle
	lockHandle   *redisplane.Handle
	lockManager  *redislock.Manager
	// Container 主容器
	container *container.Container
	// gRPC 客户端管理器
	grpcManager *grpcclient.Manager
	// IAM authz_version 同步订阅者
	authzVersionSubscriber messaging.Subscriber
}

// preparedCollectionServer 定义了准备运行的 Collection 服务器
type preparedCollectionServer struct {
	*collectionServer
}

// createCollectionServer 创建 Collection 服务器实例
func createCollectionServer(cfg *config.Config) (*collectionServer, error) {
	// 创建一个 GracefulShutdown 实例
	gs := shutdown.New()
	gs.AddShutdownManager(posixsignal.NewPosixSignalManager())
	log.Info("🔔 Graceful shutdown manager registered (POSIX signals)")

	// 创建通用服务器
	genericServer, err := buildGenericServer(cfg)
	if err != nil {
		log.Fatalf("Failed to build generic server: %v", err)
		return nil, err
	}
	log.Infof("✅ Generic server built (HTTP %s:%d, HTTPS %s:%d)",
		cfg.InsecureServing.BindAddress, cfg.InsecureServing.BindPort,
		cfg.SecureServing.BindAddress, cfg.SecureServing.BindPort)

	// 创建 Collection 服务器实例
	server := &collectionServer{
		gs:               gs,
		genericAPIServer: genericServer,
		config:           cfg,
		familyStatus:     cacheobservability.NewFamilyStatusRegistry("collection-server"),
	}

	return server, nil
}

// PrepareRun 准备运行 Collection 服务器
func (s *collectionServer) PrepareRun() preparedCollectionServer {
	var err error

	// 1. 初始化 Redis runtime
	s.dbManager = NewDatabaseManager(s.config)
	if err = s.dbManager.Initialize(); err != nil {
		log.Fatalf("Failed to initialize collection-server database manager: %v", err)
	}
	s.redisRuntime = redisplane.NewRuntime(
		"collection-server",
		s.dbManager,
		redisplane.CatalogFromOptions(s.config.RedisRuntime, map[redisplane.Family]redisplane.Route{
			redisplane.FamilyOps: {
				RedisProfile:         "ops_runtime",
				NamespaceSuffix:      "ops:runtime",
				AllowFallbackDefault: true,
			},
			redisplane.FamilyLock: {
				RedisProfile:         "lock_cache",
				NamespaceSuffix:      "cache:lock",
				AllowFallbackDefault: true,
			},
		}),
		s.familyStatus,
	)
	s.opsHandle = s.redisRuntime.Handle(context.Background(), redisplane.FamilyOps)
	s.lockHandle = s.redisRuntime.Handle(context.Background(), redisplane.FamilyLock)
	s.lockManager = redislock.NewManager("collection-server", "lock_lease", s.lockHandle)

	// 2. 创建容器
	s.container = container.NewContainer(s.config.Options, s.opsHandle, s.lockManager, s.familyStatus)

	// 3. 初始化 IAM 模块（须在 gRPC 连 apiserver 之前，以便挂载 ServiceAuth PerRPC）
	ctx := context.Background()
	iamModule, err := container.NewIAMModule(ctx, s.config.IAMOptions)
	if err != nil {
		log.Fatalf("Failed to initialize IAM module: %v", err)
	}
	s.container.IAMModule = iamModule
	log.Info("✅ IAM module initialized")

	var perRPC credentials.PerRPCCredentials
	if h := iamModule.ServiceAuthHelper(); h != nil {
		perRPC = h
	}

	// 4. 创建 gRPC 客户端管理器（可选 PerRPC 服务 JWT）
	s.grpcManager, err = CreateGRPCClientManager(
		s.config.GRPCClient.Endpoint,
		s.config.GRPCClient.Timeout,
		s.config.GRPCClient.Insecure,
		s.config.GRPCClient.TLSCertFile,
		s.config.GRPCClient.TLSKeyFile,
		s.config.GRPCClient.TLSCAFile,
		s.config.GRPCClient.TLSServerName,
		s.config.GRPCClient.MaxInflight,
		perRPC,
	)
	if err != nil {
		log.Fatalf("Failed to create gRPC client manager: %v", err)
	}
	log.Infof("✅ gRPC client manager initialized (endpoint: %s)", s.config.GRPCClient.Endpoint)

	// 5. 通过 GRPCClientRegistry 注入 gRPC 客户端到容器
	grpcRegistry := NewGRPCClientRegistry(s.grpcManager, s.container)
	if err := grpcRegistry.RegisterClients(); err != nil {
		log.Fatalf("Failed to register gRPC clients: %v", err)
	}

	// 6. 初始化容器中的所有组件
	if err := s.container.Initialize(); err != nil {
		log.Fatalf("Failed to initialize container: %v", err)
	}
	s.startAuthzVersionSync()
	log.Infof("Router registering with middlewares: %v", s.config.GenericServerRunOptions.Middlewares)

	// 7. 安装全局并发限制中间件（避免过载）
	if s.config.Concurrency != nil && s.config.Concurrency.MaxConcurrency > 0 {
		s.genericAPIServer.Engine.Use(concurrencyLimitMiddleware(s.config.Concurrency.MaxConcurrency))
		log.Infof("Installed concurrency limiter: max=%d", s.config.Concurrency.MaxConcurrency)
	}

	// 8. 创建并初始化路由器
	NewRouter(s.container).RegisterRoutes(s.genericAPIServer.Engine)

	log.Info("🏗️  Collection Server initialized successfully!")

	// 添加关闭回调
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		if s.grpcManager != nil {
			_ = s.grpcManager.Close()
		}
		if s.dbManager != nil {
			_ = s.dbManager.Close()
		}
		if s.authzVersionSubscriber != nil {
			s.authzVersionSubscriber.Stop()
			_ = s.authzVersionSubscriber.Close()
		}

		// 关闭 IAM 模块
		if s.container.IAMModule != nil {
			_ = s.container.IAMModule.Close()
		}

		// 清理容器资源
		if s.container != nil {
			s.container.Cleanup()
		}

		// 关闭 HTTP 服务器
		s.genericAPIServer.Close()

		log.Info("🏗️  Collection Server shutdown complete")
		return nil
	}))

	return preparedCollectionServer{s}
}

func (s *collectionServer) startAuthzVersionSync() {
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
		log.Warnf("Failed to create collection authz version subscriber: %v", err)
		return
	}
	channelPrefix := authzSync.ChannelPrefix
	if channelPrefix == "" {
		channelPrefix = "qs-authz-sync"
	}
	channel := iamauth.DefaultVersionSyncChannel(channelPrefix + "-collection")
	if err := iamauth.SubscribeVersionChanges(context.Background(), subscriber, authzSync.Topic, channel, loader); err != nil {
		_ = subscriber.Close()
		log.Warnf("Failed to subscribe collection authz version sync: %v", err)
		return
	}
	s.authzVersionSubscriber = subscriber
}

// Run 运行 Collection 服务器
func (s preparedCollectionServer) Run() error {
	// 启动关闭管理器
	if err := s.gs.Start(); err != nil {
		log.Fatalf("start shutdown manager failed: %s", err.Error())
	}
	log.Info("🚦 Shutdown manager started, servers coming online")

	log.Info("🚀 Starting Collection Server HTTP REST API server...")
	return s.genericAPIServer.Run()
}

// concurrencyLimitMiddleware 使用带缓冲通道实现全局并发限制
func concurrencyLimitMiddleware(max int) gin.HandlerFunc {
	sem := make(chan struct{}, max)
	return func(c *gin.Context) {
		sem <- struct{}{}
		defer func() { <-sem }()
		c.Next()
	}
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
