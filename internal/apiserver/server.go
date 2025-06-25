package apiserver

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/config"
	genericapiserver "github.com/yshujie/questionnaire-scale/internal/pkg/server"
	"github.com/yshujie/questionnaire-scale/pkg/log"
	"github.com/yshujie/questionnaire-scale/pkg/shutdown"
	"github.com/yshujie/questionnaire-scale/pkg/shutdown/shutdownmanagers/posixsignal"
)

// apiServer 定义了 API 服务器的基本结构
type apiServer struct {
	// 优雅关闭管理器
	gs *shutdown.GracefulShutdown
	// 通用 API 服务器
	genericAPIServer *genericapiserver.GenericAPIServer
	// 数据库管理器
	dbManager *DatabaseManager
}

// preparedAPIServer 定义了准备运行的 API 服务器
type preparedAPIServer struct {
	*apiServer
}

// createAPIServer 创建 API 服务器实例
func createAPIServer(cfg *config.Config) (*apiServer, error) {
	// 创建一个 GracefulShutdown 实例
	gs := shutdown.New()
	gs.AddShutdownManager(posixsignal.NewPosixSignalManager())

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

	// 创建数据库管理器
	dbManager := NewDatabaseManager(cfg)

	// 创建 API 服务器实例
	server := &apiServer{
		gs:               gs,
		genericAPIServer: genericServer,
		dbManager:        dbManager,
	}

	return server, nil
}

// PrepareRun 准备运行 API 服务器
func (s *apiServer) PrepareRun() preparedAPIServer {
	// 初始化数据库连接
	if err := s.dbManager.Initialize(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// 初始化路由
	initRouter(s.genericAPIServer.Engine, s.dbManager)

	// 添加关闭回调
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		// 关闭数据库连接
		if s.dbManager != nil {
			if err := s.dbManager.Close(); err != nil {
				log.Errorf("Failed to close database connections: %v", err)
			}
		}

		// 关闭 HTTP 服务器
		s.genericAPIServer.Close()

		log.Info("HTTP server shutdown complete")
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

	log.Info("Starting HTTP REST API server...")
	return s.genericAPIServer.Run()
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
