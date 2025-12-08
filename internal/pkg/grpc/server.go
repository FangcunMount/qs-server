package grpc

import (
	"fmt"
	"net"

	basegrpc "github.com/FangcunMount/component-base/pkg/grpc/interceptors"
	basemtls "github.com/FangcunMount/component-base/pkg/grpc/mtls"
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/iam-contracts/pkg/sdk/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
)

// Server gRPC 服务器结构体
type Server struct {
	*grpc.Server
	config          *Config
	services        []Service
	mtlsCreds       *basemtls.ServerCredentials
	healthCheck     *health.Server
	tokenVerifier   *auth.TokenVerifier // SDK TokenVerifier（支持本地 JWKS 验签）
	authInterceptor *IAMAuthInterceptor
}

// Service gRPC 服务接口
type Service interface {
	RegisterService(*grpc.Server)
}

// NewServer 创建新的 gRPC 服务器（使用 component-base 提供的能力）
// tokenVerifier: SDK 的 TokenVerifier，支持本地 JWKS 验签和远程降级
func NewServer(config *Config, tokenVerifier *auth.TokenVerifier) (*Server, error) {
	var serverOpts []grpc.ServerOption
	var mtlsCreds *basemtls.ServerCredentials

	// 1. 构建拦截器链（使用 component-base 的拦截器）
	unaryInterceptors := buildUnaryInterceptors(config, tokenVerifier)
	serverOpts = append(serverOpts, grpc.ChainUnaryInterceptor(unaryInterceptors...))

	// 2. 配置消息大小限制
	if config.MaxMsgSize > 0 {
		serverOpts = append(serverOpts,
			grpc.MaxRecvMsgSize(config.MaxMsgSize),
			grpc.MaxSendMsgSize(config.MaxMsgSize),
		)
	}

	// 3. 配置连接管理
	if config.MaxConnectionAge > 0 {
		serverOpts = append(serverOpts, grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionAge:      config.MaxConnectionAge,
			MaxConnectionAgeGrace: config.MaxConnectionAgeGrace,
		}))
	}

	// 4. 配置 TLS/mTLS（使用 component-base/pkg/grpc/mtls）
	if !config.Insecure && config.TLSCertFile != "" && config.TLSKeyFile != "" {
		if config.MTLS.Enabled {
			// mTLS 双向认证
			mtlsConfig := config.MTLS.ToBaseMTLSConfig(config.TLSCertFile, config.TLSKeyFile)
			creds, err := basemtls.NewServerCredentials(mtlsConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to create mTLS credentials: %w", err)
			}
			mtlsCreds = creds
			serverOpts = append(serverOpts, creds.GRPCServerOption())
			log.Infof("gRPC server: mTLS enabled with CA: %s", config.MTLS.CAFile)
		} else {
			// 单向 TLS
			creds, err := credentials.NewServerTLSFromFile(config.TLSCertFile, config.TLSKeyFile)
			if err != nil {
				return nil, fmt.Errorf("failed to load TLS credentials: %w", err)
			}
			serverOpts = append(serverOpts, grpc.Creds(creds))
			log.Info("gRPC server: TLS enabled (one-way)")
		}
	} else {
		log.Warn("gRPC server: running in INSECURE mode (not recommended for production)")
	}

	// 5. 创建 gRPC 服务器
	grpcServer := grpc.NewServer(serverOpts...)

	// 6. 注册健康检查服务
	var healthCheck *health.Server
	if config.EnableHealthCheck {
		healthCheck = health.NewServer()
		healthpb.RegisterHealthServer(grpcServer, healthCheck)
		log.Info("gRPC server: health check service registered")
	}

	// 7. 注册反射服务
	if config.EnableReflection {
		reflection.Register(grpcServer)
		log.Info("gRPC server: reflection service registered")
	}

	// 创建 IAM 认证拦截器
	var authInterceptor *IAMAuthInterceptor
	if config.Auth.Enabled && tokenVerifier != nil {
		authInterceptor = NewIAMAuthInterceptor(tokenVerifier, &config.Auth)
	}

	return &Server{
		Server:          grpcServer,
		config:          config,
		services:        make([]Service, 0),
		mtlsCreds:       mtlsCreds,
		healthCheck:     healthCheck,
		tokenVerifier:   tokenVerifier,
		authInterceptor: authInterceptor,
	}, nil
}

// buildUnaryInterceptors 构建一元拦截器链（使用 component-base 提供的拦截器）
func buildUnaryInterceptors(config *Config, tokenVerifier *auth.TokenVerifier) []grpc.UnaryServerInterceptor {
	var interceptorChain []grpc.UnaryServerInterceptor

	// 1. Recovery（最外层，捕获所有 panic）
	interceptorChain = append(interceptorChain,
		basegrpc.RecoveryInterceptor())

	// 2. RequestID（生成请求 ID）
	interceptorChain = append(interceptorChain,
		basegrpc.RequestIDInterceptor(
			basegrpc.WithRequestIDGenerator(RequestIDGenerator()),
		))

	// 3. Logging（记录请求日志）
	interceptorChain = append(interceptorChain,
		basegrpc.LoggingInterceptor(NewComponentBaseLogger()))

	// 4. mTLS Identity（提取客户端身份）
	if config.MTLS.Enabled {
		interceptorChain = append(interceptorChain,
			basegrpc.MTLSInterceptor())
		log.Info("gRPC server: mTLS identity interceptor enabled")
	}

	// 5. IAM Authentication（IAM JWT 认证 - 使用 SDK TokenVerifier 本地验签）
	if config.Auth.Enabled && tokenVerifier != nil {
		authInterceptor := NewIAMAuthInterceptor(tokenVerifier, &config.Auth)
		interceptorChain = append(interceptorChain,
			authInterceptor.UnaryServerInterceptor())
		log.Info("gRPC server: IAM authentication interceptor enabled (local JWKS verification)")
	} else if config.Auth.Enabled {
		log.Warn("gRPC server: auth enabled but TokenVerifier not provided, skipping authentication")
	}

	// 6. ACL（权限控制 - 使用 component-base 的 ServiceACL）
	if config.ACL.Enabled {
		acl := loadACLConfig(config.ACL.ConfigFile, config.ACL.DefaultPolicy)
		interceptorChain = append(interceptorChain,
			basegrpc.ACLInterceptor(acl, basegrpc.WithACLLogger(NewComponentBaseLogger())))
		log.Infof("gRPC server: ACL interceptor enabled (default_policy=%s, config_file=%s)",
			config.ACL.DefaultPolicy, config.ACL.ConfigFile)
	}

	// 7. Audit（审计日志 - 使用 component-base 的 AuditInterceptor）
	if config.Audit.Enabled {
		auditLogger := basegrpc.NewDefaultAuditLogger(NewComponentBaseLogger())
		interceptorChain = append(interceptorChain,
			basegrpc.AuditInterceptor(auditLogger))
		log.Infof("gRPC server: Audit interceptor enabled (output=%s)", config.Audit.OutputPath)
	}

	return interceptorChain
}

// loadACLConfig 从配置文件加载 ACL 规则
// 如果文件不存在或解析失败，返回仅包含默认策略的 ACL
func loadACLConfig(configFile, defaultPolicy string) *basegrpc.ServiceACL {
	cfg := &basegrpc.ACLConfig{
		DefaultPolicy: defaultPolicy,
		Services:      make([]*basegrpc.ServicePermissions, 0),
	}

	if configFile != "" {
		// TODO: 从 YAML/JSON 文件加载 ACL 规则
		// 示例配置文件格式:
		// default_policy: deny
		// services:
		//   - service_name: collection-server
		//     enabled: true
		//     allowed_methods:
		//       - /qs.survey.v1.SurveyService/*
		//       - /qs.evaluation.v1.EvaluationService/*
		//     denied_methods:
		//       - /qs.admin.v1.AdminService/*
		log.Infof("gRPC ACL: loading config from %s (not yet implemented)", configFile)
	}

	return basegrpc.NewServiceACL(cfg)
}

// RegisterService 注册服务
func (s *Server) RegisterService(svc Service) {
	s.services = append(s.services, svc)
	svc.RegisterService(s.Server)
}

// Run 启动服务器
func (s *Server) Run() error {
	addr := fmt.Sprintf("%s:%d", s.config.BindAddress, s.config.BindPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	scheme := "grpc"
	if !s.config.Insecure {
		if s.config.MTLS.Enabled {
			scheme = "grpcs (mTLS)"
		} else {
			scheme = "grpcs"
		}
	}

	log.Infof("gRPC server listening on %s://%s", scheme, addr)
	log.Infof("  - Max message size: %d bytes", s.config.MaxMsgSize)
	log.Infof("  - Health check: %v", s.config.EnableHealthCheck)
	log.Infof("  - Reflection: %v", s.config.EnableReflection)

	return s.Serve(lis)
}

// Close 优雅关闭服务器
func (s *Server) Close() {
	if s.healthCheck != nil {
		s.healthCheck.Shutdown()
	}
	s.GracefulStop()
	log.Info("gRPC server stopped gracefully")
}

// Address 返回服务器地址
func (s *Server) Address() string {
	return fmt.Sprintf("%s:%d", s.config.BindAddress, s.config.BindPort)
}
