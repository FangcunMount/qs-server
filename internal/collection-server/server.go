package collection

import (
	"github.com/yshujie/questionnaire-scale/internal/collection-server/config"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/container"
	genericapiserver "github.com/yshujie/questionnaire-scale/internal/pkg/server"
	"github.com/yshujie/questionnaire-scale/pkg/log"
	"github.com/yshujie/questionnaire-scale/pkg/shutdown"
	"github.com/yshujie/questionnaire-scale/pkg/shutdown/shutdownmanagers/posixsignal"
)

// collectionServer 定义了 Collection 服务器的基本结构
type collectionServer struct {
	// 优雅关闭管理器
	gs *shutdown.GracefulShutdown
	// 通用 API 服务器
	genericAPIServer *genericapiserver.GenericAPIServer
	// 配置
	config *config.Config
	// Container 主容器
	container *container.Container
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

	// 创建通用服务器
	genericServer, err := buildGenericServer(cfg)
	if err != nil {
		log.Fatalf("Failed to build generic server: %v", err)
		return nil, err
	}

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
	// 创建容器
	pubsubConfig := s.config.ToPubSubConfig()
	s.container = container.NewContainer(s.config.GRPCClient, pubsubConfig, s.config.Concurrency)

	// 初始化容器中的所有组件
	if err := s.container.Initialize(); err != nil {
		log.Fatalf("Failed to initialize container: %v", err)
	}

	// 创建并初始化路由器
	NewRouter(s.container).RegisterRoutes(s.genericAPIServer.Engine)

	log.Info("🏗️  Collection Server initialized successfully!")
	log.Info("   📦 Domain: validation")
	log.Info("   🔌 Ports: grpc-client, redis-publisher")
	log.Info("   🔧 Adapters: http, grpc-client, redis-publisher")
	log.Info("   📋 Application Services: validation_service, questionnaire_client, answersheet_client")

	// 添加关闭回调
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
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

	log.Info("🚀 Starting Collection Server HTTP REST API server...")
	return s.genericAPIServer.Run()
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
