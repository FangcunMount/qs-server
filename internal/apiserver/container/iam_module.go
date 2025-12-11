package container

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/iam-contracts/pkg/sdk/auth"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
)

// IAMModule IAM 集成模块
type IAMModule struct {
	client            *iam.Client
	tokenVerifier     *iam.TokenVerifier
	serviceAuthHelper *iam.ServiceAuthHelper
	identityService   *iam.IdentityService
	guardianshipSvc   *iam.GuardianshipService
}

// NewIAMModule 创建 IAM 模块
func NewIAMModule(ctx context.Context, opts *options.IAMOptions) (*IAMModule, error) {
	if opts == nil || !opts.Enabled {
		logger.L(context.Background()).Infow("IAM integration is disabled",
			"component", "iam_module",
		)
		return &IAMModule{}, nil
	}

	// 转换配置为 IAM 客户端格式
	clientOpts := convertIAMOptions(opts)

	// 创建 IAM 客户端
	client, err := iam.NewClient(ctx, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create IAM client: %w", err)
	}

	// 创建 Token 验证器（使用 SDK 的 JWKS 本地验签 + 远程降级）
	var tokenVerifier *iam.TokenVerifier
	if client.IsEnabled() {
		tokenVerifier, err = iam.NewTokenVerifier(ctx, client)
		if err != nil {
			logger.L(context.Background()).Warnw("Failed to create token verifier, will use remote verification only",
				"component", "iam_module",
				"error", err.Error(),
			)
			// 不返回错误，允许降级到远程验证
		}
	}

	// 创建服务间认证助手（如果配置了 ServiceAuth）
	var serviceAuthHelper *iam.ServiceAuthHelper
	if client.IsEnabled() && opts.ServiceAuth != nil && opts.ServiceAuth.ServiceID != "" {
		serviceAuthConfig := &iam.ServiceAuthConfig{
			ServiceID:      opts.ServiceAuth.ServiceID,
			TargetAudience: opts.ServiceAuth.TargetAudience,
			TokenTTL:       int64(opts.ServiceAuth.TokenTTL.Seconds()),
			RefreshBefore:  int64(opts.ServiceAuth.RefreshBefore.Seconds()),
		}
		serviceAuthHelper, err = iam.NewServiceAuthHelper(ctx, client, serviceAuthConfig)
		if err != nil {
			if errors.Is(err, iam.ErrServiceTokenNotSupported) {
				logger.L(context.Background()).Infow("IAM server does not support IssueServiceToken, service-to-service auth disabled",
					"component", "iam_module",
					"service_id", serviceAuthConfig.ServiceID,
					"target_audience", serviceAuthConfig.TargetAudience,
				)
			} else {
				logger.L(context.Background()).Warnw("Failed to create service auth helper, service-to-service auth will not be available",
					"component", "iam_module",
					"error", err.Error(),
				)
				// 不返回错误，允许继续运行
			}
		}
	}

	// 创建 Identity 服务
	var identityService *iam.IdentityService
	if client.IsEnabled() {
		identityService, err = iam.NewIdentityService(client)
		if err != nil {
			logger.L(context.Background()).Warnw("Failed to create identity service",
				"component", "iam_module",
				"error", err.Error(),
			)
		}
	}

	// 创建 Guardianship 服务
	var guardianshipSvc *iam.GuardianshipService
	if client.IsEnabled() {
		guardianshipSvc, err = iam.NewGuardianshipService(client)
		if err != nil {
			logger.L(context.Background()).Warnw("Failed to create guardianship service",
				"component", "iam_module",
				"error", err.Error(),
			)
		}
	}

	logger.L(context.Background()).Infow("IAM module initialized successfully",
		"component", "iam_module",
		"result", "success",
	)

	return &IAMModule{
		client:            client,
		tokenVerifier:     tokenVerifier,
		serviceAuthHelper: serviceAuthHelper,
		identityService:   identityService,
		guardianshipSvc:   guardianshipSvc,
	}, nil
}

// Client 返回 IAM 客户端
func (m *IAMModule) Client() *iam.Client {
	return m.client
}

// TokenVerifier 返回 Token 验证器包装（使用 SDK JWKS 本地验签）
func (m *IAMModule) TokenVerifier() *iam.TokenVerifier {
	return m.tokenVerifier
}

// SDKTokenVerifier 返回 SDK 的 TokenVerifier（用于 gRPC 拦截器等需要原生 SDK 类型的场景）
func (m *IAMModule) SDKTokenVerifier() *auth.TokenVerifier {
	if m.tokenVerifier == nil {
		return nil
	}
	return m.tokenVerifier.SDKVerifier()
}

// ServiceAuthHelper 返回服务间认证助手
// 用于 QS 服务以服务身份调用 IAM
func (m *IAMModule) ServiceAuthHelper() *iam.ServiceAuthHelper {
	return m.serviceAuthHelper
}

// IdentityService 返回身份服务
// 用于用户信息查询
func (m *IAMModule) IdentityService() *iam.IdentityService {
	return m.identityService
}

// GuardianshipService 返回监护关系服务
// 用于监护关系验证和查询
func (m *IAMModule) GuardianshipService() *iam.GuardianshipService {
	return m.guardianshipSvc
}

// IsEnabled 检查 IAM 模块是否启用
func (m *IAMModule) IsEnabled() bool {
	return m.client != nil && m.client.IsEnabled()
}

// Close 关闭 IAM 模块
func (m *IAMModule) Close() error {
	// 先关闭 ServiceAuthHelper（停止后台刷新）
	if m.serviceAuthHelper != nil {
		m.serviceAuthHelper.Stop()
	}
	// 关闭 TokenVerifier（停止 JWKS 后台刷新）
	if m.tokenVerifier != nil {
		m.tokenVerifier.Close()
	}
	// 最后关闭 Client
	if m.client != nil {
		return m.client.Close()
	}
	return nil
}

// HealthCheck 健康检查
func (m *IAMModule) HealthCheck(ctx context.Context) error {
	if !m.IsEnabled() {
		return nil
	}
	return m.client.HealthCheck(ctx)
}

// convertIAMOptions 转换配置选项
func convertIAMOptions(opts *options.IAMOptions) *iam.IAMOptions {
	if opts == nil {
		return nil
	}

	iamOpts := &iam.IAMOptions{
		Enabled:       opts.Enabled,
		GRPCEnabled:   opts.GRPCEnabled,
		JWKSEnabled:   opts.JWKSEnabled,
		EnableTracing: opts.EnableTracing,
		EnableMetrics: opts.EnableMetrics,
	}

	// GRPC 配置
	if opts.GRPC != nil {
		iamOpts.GRPC = &iam.GRPCOptions{
			Address:  opts.GRPC.Address,
			Timeout:  opts.GRPC.Timeout,
			RetryMax: opts.GRPC.RetryMax,
		}

		// TLS 配置
		if opts.GRPC.TLS != nil {
			iamOpts.GRPC.TLS = &iam.TLSOptions{
				Enabled:  opts.GRPC.TLS.Enabled,
				CAFile:   opts.GRPC.TLS.CAFile,
				CertFile: opts.GRPC.TLS.CertFile,
				KeyFile:  opts.GRPC.TLS.KeyFile,
			}
		}
	}

	// JWT 配置
	if opts.JWT != nil {
		iamOpts.JWT = &iam.JWTOptions{
			Issuer:         opts.JWT.Issuer,
			Audience:       opts.JWT.Audience,
			Algorithms:     opts.JWT.Algorithms,
			ClockSkew:      opts.JWT.ClockSkew,
			RequiredClaims: opts.JWT.RequiredClaims,
		}
	}

	// JWKS 配置
	if opts.JWKS != nil {
		iamOpts.JWKS = &iam.JWKSOptions{
			URL:             opts.JWKS.URL,
			GRPCEndpoint:    opts.JWKS.GRPCEndpoint, // gRPC 降级端点
			RefreshInterval: opts.JWKS.RefreshInterval,
			CacheTTL:        opts.JWKS.CacheTTL,
		}
	}

	// 用户缓存配置
	if opts.UserCache != nil {
		iamOpts.UserCache = &iam.CacheOptions{
			Enabled: opts.UserCache.Enabled,
			TTL:     opts.UserCache.TTL,
			MaxSize: opts.UserCache.MaxSize,
		}
	} else {
		// 默认启用用户缓存
		iamOpts.UserCache = &iam.CacheOptions{
			Enabled: true,
			TTL:     5 * time.Minute,
			MaxSize: 10000,
		}
	}

	// 监护关系缓存配置
	if opts.GuardianshipCache != nil {
		iamOpts.GuardianshipCache = &iam.CacheOptions{
			Enabled: opts.GuardianshipCache.Enabled,
			TTL:     opts.GuardianshipCache.TTL,
			MaxSize: opts.GuardianshipCache.MaxSize,
		}
	} else {
		// 默认启用监护关系缓存
		iamOpts.GuardianshipCache = &iam.CacheOptions{
			Enabled: true,
			TTL:     10 * time.Minute,
			MaxSize: 50000,
		}
	}

	return iamOpts
}
