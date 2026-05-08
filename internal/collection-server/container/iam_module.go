package container

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	auth "github.com/FangcunMount/iam/v2/pkg/sdk/auth/verifier"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/iam"
	iamauth "github.com/FangcunMount/qs-server/internal/pkg/iamauth"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
)

// IAMModule IAM 集成模块
type IAMModule struct {
	client              *iam.Client
	tokenVerifier       *iam.TokenVerifier
	serviceAuthHelper   *iam.ServiceAuthHelper
	identityService     *iam.IdentityService
	profileService      *iam.ProfileService
	profileLinkSvc     *iam.ProfileLinkService
	authzSnapshotLoader *iamauth.SnapshotLoader
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

	// 创建 Token 验证器（使用 SDK 的 JWKS 本地验签 + 远程降级）
	var tokenVerifier *iam.TokenVerifier
	if client.IsEnabled() {
		tokenVerifier, err = iam.NewTokenVerifier(ctx, client)
		if err != nil {
			log.Warnf("Failed to create token verifier: %v, will use remote verification only", err)
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
				log.Infof("IAM server does not support IssueServiceToken, service-to-service auth disabled (ServiceID=%s, Audience=%v)",
					serviceAuthConfig.ServiceID, serviceAuthConfig.TargetAudience)
			} else {
				log.Warnf("Failed to create service auth helper: %v, service-to-service auth will not be available", err)
				// 不返回错误，允许继续运行
			}
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

	var authzSnapshotLoader *iamauth.SnapshotLoader
	if client.IsEnabled() && opts.GRPCEnabled {
		iamOpts := convertIAMOptions(opts)
		authzSnapshotLoader = iamauth.NewSnapshotLoader(client, iamauth.SnapshotLoaderOptions{
			AppName:              iamOpts.AuthzAppName,
			CacheTTL:             iamOpts.AuthzCacheTTL,
			CasbinDomainOverride: iamOpts.AuthzCasbinDomainOverride,
		})
	}

	log.Info("IAM module initialized successfully")

	return &IAMModule{
		client:              client,
		tokenVerifier:       tokenVerifier,
		serviceAuthHelper:   serviceAuthHelper,
		identityService:     identityService,
		profileService:      profileService,
		profileLinkSvc:     profileLinkSvc,
		authzSnapshotLoader: authzSnapshotLoader,
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

// ServiceAuthHelper 返回服务间认证助手
// 用于 Collection 服务以服务身份调用 IAM 或 QS-APIServer
func (m *IAMModule) ServiceAuthHelper() *iam.ServiceAuthHelper {
	return m.serviceAuthHelper
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

// AuthzSnapshotLoader 返回 IAM 授权快照加载器（与 apiserver 共用 pkg/iamauth）。
func (m *IAMModule) AuthzSnapshotLoader() *iamauth.SnapshotLoader {
	return m.authzSnapshotLoader
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
	iamOpts.AuthzCasbinDomainOverride = opts.AuthzCasbinDomainOverride

	return iamOpts
}
