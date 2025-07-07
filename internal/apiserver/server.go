package apiserver

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/config"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/container"
	"github.com/yshujie/questionnaire-scale/internal/pkg/grpcserver"
	genericapiserver "github.com/yshujie/questionnaire-scale/internal/pkg/server"
	"github.com/yshujie/questionnaire-scale/pkg/log"
	"github.com/yshujie/questionnaire-scale/pkg/shutdown"
	"github.com/yshujie/questionnaire-scale/pkg/shutdown/shutdownmanagers/posixsignal"
)

// apiServer 定义了 API 服务器的基本结构（六边形架构版本）
type apiServer struct {
	// 优雅关闭管理器
	gs *shutdown.GracefulShutdown
	// 通用 API 服务器
	genericAPIServer *genericapiserver.GenericAPIServer
	// GRPC 服务器
	grpcServer *grpcserver.Server
	// 数据库管理器
	dbManager *DatabaseManager
	// Container 主容器
	container *container.Container
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
		log.Fatalf("Failed to build generic server: %v", err)
		return nil, err
	}

	// 创建 GRPC 服务器
	grpcServer, err := buildGRPCServer(cfg)
	if err != nil {
		log.Fatalf("Failed to build GRPC server: %v", err)
		return nil, err
	}

	// 创建数据库管理器
	dbManager := NewDatabaseManager(cfg)

	// 创建 API 服务器实例
	server := &apiServer{
		gs:               gs,
		genericAPIServer: genericServer,
		dbManager:        dbManager,
		grpcServer:       grpcServer,
	}

	return server, nil
}

// PrepareRun 准备运行 API 服务器（六边形架构版本）
func (s *apiServer) PrepareRun() preparedAPIServer {
	// 初始化数据库连接
	if err := s.dbManager.Initialize(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// 获取 MySQL 数据库连接
	mysqlDB, err := s.dbManager.GetMySQLDB()
	if err != nil {
		log.Fatalf("Failed to get MySQL connection: %v", err)
	}

	// 获取 MongoDB 数据库链接
	mongoDB, err := s.dbManager.GetMongoDB()
	if err != nil {
		log.Fatalf("Failed to get MongoDB connection: %v", err)
	}

	// 创建六边形架构容器（自动发现版本）
	s.container = container.NewContainer(mysqlDB, mongoDB)

	// 初始化容器中的所有组件
	if err := s.container.Initialize(); err != nil {
		log.Fatalf("Failed to initialize hexagonal architecture container: %v", err)
	}

	// 创建并初始化路由器
	NewRouter(s.container).RegisterRoutes(s.genericAPIServer.Engine)

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

	if err := s.genericAPIServer.Run(); err != nil {
		log.Errorf("Failed to run HTTP server: %v", err)
		return err
	}
	log.Info("🚀 Starting Hexagonal Architecture HTTP REST API server...")

	// 启动 GRPC 服务器
	if err := s.grpcServer.Run(); err != nil {
		log.Errorf("Failed to run GRPC server: %v", err)
		return err
	}
	log.Info("🚀 Starting Hexagonal Architecture GRPC server...")

	return nil
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

// buildGRPCServer 构建 GRPC 服务器
func buildGRPCServer(cfg *config.Config) (*grpcserver.Server, error) {
	// 创建 GRPC 配置
	grpcConfig := grpcserver.NewConfig()

	// 应用配置选项
	if err := applyGRPCOptions(cfg, grpcConfig); err != nil {
		return nil, err
	}

	// 完成配置并创建服务器
	return grpcConfig.Complete().New()
}

// applyGRPCOptions 应用 GRPC 选项到配置
func applyGRPCOptions(cfg *config.Config, grpcConfig *grpcserver.Config) error {
	// 应用基本配置
	grpcConfig.BindAddress = cfg.GRPCOptions.BindAddress
	grpcConfig.BindPort = cfg.GRPCOptions.BindPort

	// 应用 TLS 配置
	if cfg.SecureServing != nil {
		grpcConfig.TLSCertFile = cfg.SecureServing.CertFile
		grpcConfig.TLSKeyFile = cfg.SecureServing.KeyFile
	}

	return nil
}
