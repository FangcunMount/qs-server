package collection

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/shutdown"
	"github.com/FangcunMount/component-base/pkg/shutdown/shutdownmanagers/posixsignal"
	"github.com/FangcunMount/qs-server/internal/collection-server/config"
	"github.com/FangcunMount/qs-server/internal/collection-server/container"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	genericapiserver "github.com/FangcunMount/qs-server/internal/pkg/server"
	"github.com/gin-gonic/gin"
)

// collectionServer 定义了 Collection 服务器的基本结构
type collectionServer struct {
	// 优雅关闭管理器
	gs *shutdown.GracefulShutdown
	// 通用 API 服务器
	genericAPIServer *genericapiserver.GenericAPIServer
	// 配置
	config *config.Config
	// 数据库管理器
	dbManager *DatabaseManager
	// Container 主容器
	container *container.Container
	// gRPC 客户端管理器
	grpcManager *grpcclient.Manager
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
	}

	return server, nil
}

// PrepareRun 准备运行 Collection 服务器
func (s *collectionServer) PrepareRun() preparedCollectionServer {
	// 1. 初始化数据库管理器（Redis）
	s.dbManager = NewDatabaseManager(s.config)
	if err := s.dbManager.Initialize(); err != nil {
		log.Fatalf("Failed to initialize database manager: %v", err)
	}
	cacheRedis, err := s.dbManager.GetRedisClient()
	if err != nil {
		log.Warnf("Cache Redis not available: %v", err)
	}
	storeRedis, err := s.dbManager.GetStoreRedisClient()
	if err != nil {
		log.Warnf("Store Redis not available: %v", err)
	}

	// 2. 创建 gRPC 客户端管理器
	s.grpcManager, err = CreateGRPCClientManager(
		s.config.GRPCClient.Endpoint,
		s.config.GRPCClient.Timeout,
		s.config.GRPCClient.Insecure,
		s.config.GRPCClient.TLSCertFile,
		s.config.GRPCClient.TLSKeyFile,
		s.config.GRPCClient.TLSCAFile,
		s.config.GRPCClient.TLSServerName,
		s.config.GRPCClient.MaxInflight,
	)
	if err != nil {
		log.Fatalf("Failed to create gRPC client manager: %v", err)
	}
	log.Infof("✅ gRPC client manager initialized (endpoint: %s)", s.config.GRPCClient.Endpoint)

	// 3. 创建容器
	s.container = container.NewContainer(
		s.config.Options,
		cacheRedis,
		storeRedis,
	)

	// 4. 初始化 IAM 模块（优先）
	ctx := context.Background()
	iamModule, err := container.NewIAMModule(ctx, s.config.IAMOptions)
	if err != nil {
		log.Fatalf("Failed to initialize IAM module: %v", err)
	}
	s.container.IAMModule = iamModule
	log.Info("✅ IAM module initialized")

	// 5. 通过 GRPCClientRegistry 注入 gRPC 客户端到容器
	grpcRegistry := NewGRPCClientRegistry(s.grpcManager, s.container)
	if err := grpcRegistry.RegisterClients(); err != nil {
		log.Fatalf("Failed to register gRPC clients: %v", err)
	}

	// 6. 初始化容器中的所有组件
	if err := s.container.Initialize(); err != nil {
		log.Fatalf("Failed to initialize container: %v", err)
	}
	log.Infof("Router registering with middlewares: %v", s.config.GenericServerRunOptions.Middlewares)

	// 7. 安装全局并发限制中间件（避免过载）
	if s.config.Concurrency != nil && s.config.Concurrency.MaxConcurrency > 0 {
		s.genericAPIServer.Engine.Use(concurrencyLimitMiddleware(s.config.Concurrency.MaxConcurrency))
		log.Infof("Installed concurrency limiter: max=%d", s.config.Concurrency.MaxConcurrency)
	}

	// 7. 创建并初始化路由器
	NewRouter(s.container).RegisterRoutes(s.genericAPIServer.Engine)

	log.Info("🏗️  Collection Server initialized successfully!")

	// 添加关闭回调
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		if s.dbManager != nil {
			_ = s.dbManager.Close()
		}
		if s.grpcManager != nil {
			_ = s.grpcManager.Close()
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
