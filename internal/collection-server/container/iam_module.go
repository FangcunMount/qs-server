package container

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	auth "github.com/FangcunMount/iam/v5/pkg/sdk/auth/verifier"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
)

// IAMModule IAM 集成模块
type IAMModule struct {
	client          *iam.Client
	tokenVerifier   *iam.TokenVerifier
	identityService *iam.IdentityService
	profileService  *iam.ProfileService
	profileLinkSvc  *iam.ProfileLinkService
}

// NewIAMModule 创建 IAM 模块
func NewIAMModule(ctx context.Context, opts *options.IAMOptions) (*IAMModule, error) {
	if opts == nil || !opts.Enabled {
		log.Info("IAM integration is disabled")
		return &IAMModule{}, nil
	}

	// 转换配置为 IAM 客户端格式
	clientOpts := convertIAMOptions(opts)

	// 创建 IAM 客户端
	client, err := iam.NewClient(ctx, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create IAM client: %w", err)
	}

	// 创建 Token 验证器。IAM 已启用时，缺少验证器必须阻止服务启动。
	var tokenVerifier *iam.TokenVerifier
	if client.IsEnabled() {
		tokenVerifier, err = iam.NewTokenVerifier(ctx, client)
		if err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("failed to create IAM token verifier: %w", err)
		}
	}

	// 创建 Identity 服务
	var identityService *iam.IdentityService
	if client.IsEnabled() {
		identityService, err = iam.NewIdentityService(client)
		if err != nil {
			log.Warnf("Failed to create identity service: %v", err)
		}
	}

	var profileService *iam.ProfileService
	if client.IsEnabled() {
		profileService, err = iam.NewProfileService(client)
		if err != nil {
			log.Warnf("Failed to create profile service: %v", err)
		}
	}

	// 创建 ProfileLink 服务
	var profileLinkSvc *iam.ProfileLinkService
	if client.IsEnabled() {
		profileLinkSvc, err = iam.NewProfileLinkService(client)
		if err != nil {
			log.Warnf("Failed to create profile link service: %v", err)
		}
	}

	log.Info("IAM module initialized successfully")

	return &IAMModule{
		client:          client,
		tokenVerifier:   tokenVerifier,
		identityService: identityService,
		profileService:  profileService,
		profileLinkSvc:  profileLinkSvc,
	}, nil
}

// Client 返回 IAM 客户端
func (m *IAMModule) Client() *iam.Client {
	return m.client
}

// TokenVerifier 返回 Token 验证器包装
func (m *IAMModule) TokenVerifier() *iam.TokenVerifier {
	return m.tokenVerifier
}

// SDKTokenVerifier 返回 SDK 的 TokenVerifier（用于 REST 中间件等需要原生 SDK 类型的场景）
func (m *IAMModule) SDKTokenVerifier() *auth.TokenVerifier {
	if m.tokenVerifier == nil {
		return nil
	}
	return m.tokenVerifier.SDKVerifier()
}

// IdentityService 返回身份服务
// 用于用户信息查询
func (m *IAMModule) IdentityService() *iam.IdentityService {
	return m.identityService
}

// ProfileService 返回档案命令服务
// 用于 collection-server 注册 testee 时创建 IAM Profile + ProfileLink。
func (m *IAMModule) ProfileService() *iam.ProfileService {
	return m.profileService
}

// ProfileLinkService 返回 ProfileLink 服务。
// 用于 Profile 访问校验和关系查询。
func (m *IAMModule) ProfileLinkService() *iam.ProfileLinkService {
	return m.profileLinkSvc
}

// IsEnabled 检查 IAM 模块是否启用
func (m *IAMModule) IsEnabled() bool {
	return m.client != nil && m.client.IsEnabled()
}

// Close 关闭 IAM 模块
func (m *IAMModule) Close() error {
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

// ValidateRequiredRuntime verifies the production AuthN and identity-link
// dependencies before collection routes begin accepting protected traffic.
func (m *IAMModule) ValidateRequiredRuntime(ctx context.Context) error {
	if m == nil || !m.IsEnabled() {
		return fmt.Errorf("IAM integration is required")
	}
	if m.SDKTokenVerifier() == nil {
		return fmt.Errorf("IAM token verifier is required")
	}
	if m.profileLinkSvc == nil {
		return fmt.Errorf("IAM ProfileLink service is required")
	}
	if m.profileService == nil {
		return fmt.Errorf("IAM Profile service is required")
	}
	if _, err := m.client.LocalCertificateIdentity(); err != nil {
		return err
	}
	if err := m.HealthCheck(ctx); err != nil {
		return fmt.Errorf("IAM health check failed: %w", err)
	}
	if _, err := m.client.SDK().ProfileLink().GetUserProfiles(ctx, "1"); err != nil {
		return fmt.Errorf("IAM ProfileLink startup probe failed: %w", err)
	}
	return nil
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
			Issuer:                  opts.JWT.Issuer,
			Audience:                opts.JWT.Audience,
			Algorithms:              opts.JWT.Algorithms,
			ClockSkew:               opts.JWT.ClockSkew,
			RequiredClaims:          opts.JWT.RequiredClaims,
			ForceRemoteVerification: opts.JWT.ForceRemoteVerification,
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

	// ProfileLink 缓存配置
	if opts.ProfileLinkCache != nil {
		iamOpts.ProfileLinkCache = &iam.CacheOptions{
			Enabled: opts.ProfileLinkCache.Enabled,
			TTL:     opts.ProfileLinkCache.TTL,
			MaxSize: opts.ProfileLinkCache.MaxSize,
		}
	} else {
		// 默认启用 ProfileLink 缓存
		iamOpts.ProfileLinkCache = &iam.CacheOptions{
			Enabled: true,
			TTL:     10 * time.Minute,
			MaxSize: 50000,
		}
	}

	if opts.AuthzAppName != "" {
		iamOpts.AuthzAppName = opts.AuthzAppName
	}
	if opts.AuthzCacheTTL > 0 {
		iamOpts.AuthzCacheTTL = opts.AuthzCacheTTL
	}

	return iamOpts
}
