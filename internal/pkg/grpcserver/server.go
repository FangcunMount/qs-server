package grpcserver

import (
	"context"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	"github.com/FangcunMount/component-base/pkg/log"
)

// Server GRPC 服务器结构体
type Server struct {
	*grpc.Server
	config   *Config
	services []Service
	secure   bool
}

// Service GRPC 服务接口
type Service interface {
	RegisterService(*grpc.Server)
}

// NewServer 创建新的 GRPC 服务器
func NewServer(config *Config) (*Server, error) {
	// 创建 GRPC 服务器选项
	var serverOpts []grpc.ServerOption

	// 添加拦截器链
	serverOpts = append(serverOpts, grpc.ChainUnaryInterceptor(
		RecoveryInterceptor(),  // 恢复拦截器，防止 panic
		RequestIDInterceptor(), // 请求ID拦截器
		LoggingInterceptor(),   // 日志拦截器
	))

	// 添加消息大小限制
	if config.MaxMsgSize > 0 {
		serverOpts = append(serverOpts,
			grpc.MaxRecvMsgSize(config.MaxMsgSize),
			grpc.MaxSendMsgSize(config.MaxMsgSize),
		)
	}

	// 添加连接管理选项
	if config.MaxConnectionAge > 0 {
		serverOpts = append(serverOpts, grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionAge:      config.MaxConnectionAge,
			MaxConnectionAgeGrace: config.MaxConnectionAgeGrace,
		}))
	}

	// 如果配置了 TLS 且不是不安全模式，添加 TLS 选项
	secure := false
	if !config.Insecure && config.TLSCertFile != "" && config.TLSKeyFile != "" {
		creds, err := credentials.NewServerTLSFromFile(config.TLSCertFile, config.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS credentials: %v", err)
		}
		serverOpts = append(serverOpts, grpc.Creds(creds))
		secure = true
	}

	// 创建 GRPC 服务器
	grpcServer := grpc.NewServer(serverOpts...)

	// 注册健康检查服务
	if config.EnableHealthCheck {
		healthServer := health.NewServer()
		healthpb.RegisterHealthServer(grpcServer, healthServer)
	}

	// 注册反射服务，用于服务发现
	if config.EnableReflection {
		reflection.Register(grpcServer)
	}

	return &Server{
		Server:   grpcServer,
		config:   config,
		services: make([]Service, 0),
		secure:   secure,
	}, nil
}

// RegisterService 注册 GRPC 服务
func (s *Server) RegisterService(service Service) {
	service.RegisterService(s.Server)
	s.services = append(s.services, service)
}

// Run 启动 GRPC 服务器
func (s *Server) Run() error {
	address := fmt.Sprintf("%s:%d", s.config.BindAddress, s.config.BindPort)

	// 创建 TCP 监听器
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %v", address, err)
	}

	// 打印服务器信息
	scheme := "http"
	if s.secure {
		scheme = "https"
	}
	log.Infof("Starting GRPC Server on %s://%s (max message size: %d)", scheme, address, s.config.MaxMsgSize)

	// 启动服务器
	return s.Serve(lis)
}

// RunWithContext 使用上下文启动 GRPC 服务器
func (s *Server) RunWithContext(ctx context.Context) error {
	errCh := make(chan error)
	go func() {
		errCh <- s.Run()
	}()

	select {
	case <-ctx.Done():
		s.Close()
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// Close 优雅关闭 GRPC 服务器
func (s *Server) Close() {
	const timeout = 5 * time.Second
	ch := make(chan struct{})

	go func() {
		// 优雅停止
		s.GracefulStop()
		close(ch)
	}()

	// 等待优雅停止或超时
	select {
	case <-ch:
		log.Info("GRPC server stopped gracefully")
	case <-time.After(timeout):
		log.Info("GRPC server forced to stop after timeout")
		s.Stop()
	}
}

// Address 返回服务器地址
func (s *Server) Address() string {
	return fmt.Sprintf("%s:%d", s.config.BindAddress, s.config.BindPort)
}

// Config 返回服务器配置
func (s *Server) Config() *Config {
	return s.config
}
