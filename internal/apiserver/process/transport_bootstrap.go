package process

import (
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"
	resttransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest"
	grpcpkg "github.com/FangcunMount/qs-server/internal/pkg/grpc"
	genericapiserver "github.com/FangcunMount/qs-server/internal/pkg/server"
)

type transportStageDeps struct {
	buildHTTPServer func() (*genericapiserver.GenericAPIServer, error)
	buildGRPCServer func() (*grpcpkg.Server, error)
	registerREST    func(*genericapiserver.GenericAPIServer)
	registerGRPC    func(*grpcpkg.Server) error
}

func (s *server) initializeTransports(containerOutput containerOutput) (transportOutput, error) {
	return bootstrapTransports(s.buildTransportStageDeps(containerOutput))
}

func (s *server) buildTransportStageDeps(containerOutput containerOutput) transportStageDeps {
	if s == nil || s.config == nil || containerOutput.container == nil {
		return transportStageDeps{}
	}

	grpcBootstrapDeps := containerOutput.container.BuildServerGRPCBootstrapDeps()
	return transportStageDeps{
		buildHTTPServer: func() (*genericapiserver.GenericAPIServer, error) {
			return buildGenericServer(s.config)
		},
		buildGRPCServer: func() (*grpcpkg.Server, error) {
			return buildGRPCServer(s.config, grpcBootstrapDeps)
		},
		registerREST: func(httpServer *genericapiserver.GenericAPIServer) {
			resttransport.NewRouter(containerOutput.container.BuildRESTDeps(s.config.RateLimit)).RegisterRoutes(httpServer.Engine)
		},
		registerGRPC: func(grpcServer *grpcpkg.Server) error {
			return grpctransport.NewRegistry(containerOutput.container.BuildGRPCDeps(grpcServer)).RegisterServices()
		},
	}
}

func bootstrapTransports(deps transportStageDeps) (transportOutput, error) {
	var output transportOutput
	if deps.buildHTTPServer != nil {
		httpServer, err := deps.buildHTTPServer()
		if err != nil {
			return transportOutput{}, err
		}
		output.httpServer = httpServer
	}
	if deps.buildGRPCServer != nil {
		grpcServer, err := deps.buildGRPCServer()
		if err != nil {
			return transportOutput{}, err
		}
		output.grpcServer = grpcServer
	}
	if deps.registerREST != nil && output.httpServer != nil {
		deps.registerREST(output.httpServer)
	}
	if deps.registerGRPC != nil && output.grpcServer != nil {
		if err := deps.registerGRPC(output.grpcServer); err != nil {
			return transportOutput{}, err
		}
	}
	return output, nil
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
func buildGRPCServer(cfg *config.Config, deps container.ServerGRPCBootstrapDeps) (*grpcpkg.Server, error) {
	// 创建 GRPC 配置
	grpcConfig := grpcpkg.NewConfig()

	// 应用配置选项
	if err := applyGRPCOptions(cfg, grpcConfig); err != nil {
		return nil, err
	}

	if loader := deps.AuthzSnapshotLoader; loader != nil {
		// 授权快照拦截器只负责权限视图，不替代前面的 JWT 权威在线校验。
		grpcConfig.ExtraUnaryAfterAuth = append(grpcConfig.ExtraUnaryAfterAuth,
			grpctransport.NewAuthzSnapshotUnaryInterceptor(loader, deps.ActiveOperatorRepo))
		log.Info("gRPC server: IAM authorization snapshot interceptor enabled (after JWT auth)")
	}

	// 获取 SDK TokenVerifier（使用 SDK 的本地 JWKS 验签能力）
	if deps.TokenVerifier != nil {
		log.Info("gRPC server: TokenVerifier injected for authentication (local JWKS verification)")
	} else {
		log.Warn("gRPC server: TokenVerifier not available, authentication disabled")
	}

	// 完成配置并创建服务器
	return grpcConfig.Complete().New(deps.TokenVerifier)
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
