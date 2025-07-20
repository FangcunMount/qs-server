package evaluation

import (
	"github.com/yshujie/questionnaire-scale/internal/evaluation-server/config"
	"github.com/yshujie/questionnaire-scale/internal/evaluation-server/container"
	genericapiserver "github.com/yshujie/questionnaire-scale/internal/pkg/server"
	"github.com/yshujie/questionnaire-scale/pkg/log"
	"github.com/yshujie/questionnaire-scale/pkg/shutdown"
	"github.com/yshujie/questionnaire-scale/pkg/shutdown/shutdownmanagers/posixsignal"
)

// evaluationServer 定义了 Evaluation 服务器的基本结构
type evaluationServer struct {
	// 优雅关闭管理器
	gs *shutdown.GracefulShutdown
	// 通用 API 服务器（仅用于健康检查）
	genericAPIServer *genericapiserver.GenericAPIServer
	// 配置
	config *config.Config
	// Container 主容器
	container *container.Container
}

// preparedEvaluationServer 定义了准备运行的 Evaluation 服务器
type preparedEvaluationServer struct {
	*evaluationServer
}

// createEvaluationServer 创建 Evaluation 服务器实例
func createEvaluationServer(cfg *config.Config) (*evaluationServer, error) {
	// 创建一个 GracefulShutdown 实例
	gs := shutdown.New()
	gs.AddShutdownManager(posixsignal.NewPosixSignalManager())

	// 创建通用服务器（仅用于健康检查）
	genericServer, err := buildGenericServer(cfg)
	if err != nil {
		log.Fatalf("Failed to build generic server: %v", err)
		return nil, err
	}

	// 创建 Evaluation 服务器实例
	server := &evaluationServer{
		gs:               gs,
		genericAPIServer: genericServer,
		config:           cfg,
	}

	return server, nil
}

// PrepareRun 准备运行 Evaluation 服务器
func (s *evaluationServer) PrepareRun() preparedEvaluationServer {
	// 创建容器
	pubsubConfig := s.config.MessageQueue.ToPubSubConfig()
	s.container = container.NewContainer(s.config.GRPCClient, s.config.MessageQueue, s.config.Concurrency, pubsubConfig)

	// 初始化容器中的所有组件
	if err := s.container.Initialize(); err != nil {
		log.Fatalf("Failed to initialize container: %v", err)
	}

	// 创建并初始化路由器（仅健康检查）
	NewRouter(s.container).RegisterRoutes(s.genericAPIServer.Engine)

	log.Info("🏗️  Evaluation Server initialized successfully!")
	log.Info("   📦 Domain: scoring, evaluation, report-generation")
	log.Info("   🔌 Ports: message-queue-subscriber, grpc-client")
	log.Info("   🔧 Adapters: grpc-client, message-queue")
	log.Info("   📋 Application Services: scoring_service, evaluation_service, report_generator")

	// 添加关闭回调
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		// 清理容器资源
		if s.container != nil {
			s.container.Cleanup()
		}

		// 关闭 HTTP 服务器
		s.genericAPIServer.Close()

		log.Info("🏗️  Evaluation Server shutdown complete")
		return nil
	}))

	return preparedEvaluationServer{s}
}

// Run 运行 Evaluation 服务器
func (s preparedEvaluationServer) Run() error {
	// 启动关闭管理器
	if err := s.gs.Start(); err != nil {
		log.Fatalf("start shutdown manager failed: %s", err.Error())
	}

	log.Info("🚀 Starting Evaluation Server...")
	log.Info("   📨 Message queue subscriber starting...")
	log.Info("   🌐 HTTP health check server starting...")

	// 创建一个 channel 用于接收错误
	errChan := make(chan error, 2)

	// 启动消息队列订阅者（在 goroutine 中运行）
	go func() {
		if err := s.container.StartMessageSubscriber(); err != nil {
			log.Errorf("Message subscriber failed: %v", err)
			errChan <- err
		}
	}()

	// 启动 HTTP 健康检查服务器（在 goroutine 中运行）
	go func() {
		if err := s.genericAPIServer.Run(); err != nil {
			log.Errorf("HTTP server failed: %v", err)
			errChan <- err
		}
	}()

	log.Info("✅ Evaluation Server started successfully")
	log.Info("   📨 Message queue subscriber is running")
	log.Info("   🌐 HTTP health check server is running")

	// 等待任一服务出错
	return <-errChan
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
