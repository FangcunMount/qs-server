package iam

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	auth "github.com/FangcunMount/iam/v3/pkg/sdk/auth/verifier"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
)

// Module IAM 集成模块
type Module struct {
	client              *iam.Client
	tokenVerifier       *iam.TokenVerifier
	serviceAuthHelper   *iam.ServiceAuthHelper
	identityService     *iam.IdentityService
	operationAccountSvc *iam.OperationAccountService
	profileLinkSvc      *iam.ProfileLinkService
	wechatAppService    *iam.WeChatAppService
	authzSnapshotLoader *iam.AuthzSnapshotLoader
	objectAuthzChecker  *iam.ObjectAuthorizationChecker
}

type RuntimeOptions struct {
	Limiter backpressure.Acquirer
}

func NewWithRuntimeOptions(ctx context.Context, opts *options.IAMOptions, runtime RuntimeOptions) (*Module, error) {
	if opts == nil || !opts.Enabled {
		logger.L(context.Background()).Infow("IAM integration is disabled",
			"component", "iam_module",
		)
		return &Module{}, nil
	}

	// 转换配置为 IAM 客户端格式
	clientOpts := convertIAMOptions(opts)

	// 创建 IAM 客户端
	client, err := iam.NewClientWithRuntimeOptions(ctx, clientOpts, iam.ClientRuntimeOptions{
		Limiter: runtime.Limiter,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create IAM client: %w", err)
	}

	serviceAuthHelper := newIAMServiceAuthHelper(ctx, client, opts)
	module := &Module{
		client:              client,
		tokenVerifier:       newIAMTokenVerifier(ctx, client),
		serviceAuthHelper:   serviceAuthHelper,
		identityService:     newIAMIdentityService(client),
		operationAccountSvc: newIAMOperationAccountService(client),
		profileLinkSvc:      newIAMProfileLinkService(client),
		wechatAppService:    newIAMWeChatAppService(client),
		authzSnapshotLoader: newIAMAuthzSnapshotLoader(client, opts, serviceAuthHelper),
		objectAuthzChecker:  iam.NewObjectAuthorizationChecker(client, serviceAuthHelper),
	}

	logger.L(context.Background()).Infow("IAM module initialized successfully",
		"component", "iam_module",
		"result", "success",
	)

	return module, nil
}

func newIAMTokenVerifier(ctx context.Context, client *iam.Client) *iam.TokenVerifier {
	if client == nil || !client.IsEnabled() {
		return nil
	}

	tokenVerifier, err := iam.NewTokenVerifier(ctx, client)
	if err != nil {
		logger.L(context.Background()).Warnw("Failed to create required IAM token verifier",
			"component", "iam_module",
			"error", err.Error(),
		)
		return nil
	}
	return tokenVerifier
}

func newIAMServiceAuthHelper(ctx context.Context, client *iam.Client, opts *options.IAMOptions) *iam.ServiceAuthHelper {
	if client == nil || !client.IsEnabled() || opts == nil || opts.ServiceAuth == nil || opts.ServiceAuth.ServiceID == "" {
		return nil
	}

	serviceAuthConfig := &iam.ServiceAuthConfig{
		ServiceID:      opts.ServiceAuth.ServiceID,
		TargetAudience: opts.ServiceAuth.TargetAudience,
		TokenTTL:       int64(opts.ServiceAuth.TokenTTL.Seconds()),
		RefreshBefore:  int64(opts.ServiceAuth.RefreshBefore.Seconds()),
	}

	serviceAuthHelper, err := iam.NewServiceAuthHelper(ctx, client, serviceAuthConfig)
	if err == nil {
		return serviceAuthHelper
	}
	if errors.Is(err, iam.ErrServiceTokenNotSupported) {
		logger.L(context.Background()).Warnw("IAM server does not support required service-to-service authentication",
			"component", "iam_module",
			"service_id", serviceAuthConfig.ServiceID,
			"target_audience", serviceAuthConfig.TargetAudience,
		)
		return nil
	}

	logger.L(context.Background()).Warnw("Failed to create required IAM service auth helper",
		"component", "iam_module",
		"error", err.Error(),
	)
	return nil
}

func newIAMIdentityService(client *iam.Client) *iam.IdentityService {
	if client == nil || !client.IsEnabled() {
		return nil
	}
	identityService, err := iam.NewIdentityService(client)
	if err != nil {
		logger.L(context.Background()).Warnw("Failed to create identity service",
			"component", "iam_module",
			"error", err.Error(),
		)
		return nil
	}
	return identityService
}

func newIAMOperationAccountService(client *iam.Client) *iam.OperationAccountService {
	if client == nil || !client.IsEnabled() {
		return nil
	}
	service, err := iam.NewOperationAccountService(client)
	if err != nil {
		logger.L(context.Background()).Warnw("Failed to create operation account service",
			"component", "iam_module",
			"error", err.Error(),
		)
		return nil
	}
	return service
}

func newIAMProfileLinkService(client *iam.Client) *iam.ProfileLinkService {
	if client == nil || !client.IsEnabled() {
		return nil
	}
	service, err := iam.NewProfileLinkService(client)
	if err != nil {
		logger.L(context.Background()).Warnw("Failed to create profile link service",
			"component", "iam_module",
			"error", err.Error(),
		)
		return nil
	}
	return service
}

func newIAMWeChatAppService(client *iam.Client) *iam.WeChatAppService {
	if client == nil || !client.IsEnabled() {
		return nil
	}
	service, err := iam.NewWeChatAppService(client)
	if err != nil {
		logger.L(context.Background()).Warnw("Failed to create wechat app service",
			"component", "iam_module",
			"error", err.Error(),
		)
		return nil
	}
	return service
}

func newIAMAuthzSnapshotLoader(client *iam.Client, opts *options.IAMOptions, tokens *iam.ServiceAuthHelper) *iam.AuthzSnapshotLoader {
	if client == nil || !client.IsEnabled() || opts == nil || !opts.GRPCEnabled {
		return nil
	}
	iamOpts := convertIAMOptions(opts)
	return iam.NewAuthzSnapshotLoader(client, iam.AuthzSnapshotLoaderOptions{
		AppName:              iamOpts.AuthzAppName,
		CacheTTL:             iamOpts.AuthzCacheTTL,
		DomainOverride:       iamOpts.AuthzDomainOverride,
		ServiceTokenProvider: tokens,
	})
}

// Client 返回 IAM 客户端
func (m *Module) Client() *iam.Client {
	return m.client
}

// TokenVerifier 返回 Token 验证器包装（使用 SDK JWKS 本地验签）
func (m *Module) TokenVerifier() *iam.TokenVerifier {
	return m.tokenVerifier
}

// SDKTokenVerifier 返回 SDK 的 TokenVerifier（用于 gRPC 拦截器等需要原生 SDK 类型的场景）
func (m *Module) SDKTokenVerifier() *auth.TokenVerifier {
	if m.tokenVerifier == nil {
		return nil
	}
	return m.tokenVerifier.SDKVerifier()
}

// ServiceAuthHelper 返回服务间认证助手
// 用于 QS 服务以服务身份调用 IAM
func (m *Module) ServiceAuthHelper() *iam.ServiceAuthHelper {
	return m.serviceAuthHelper
}

// IdentityService 返回身份服务
// 用于用户信息查询
func (m *Module) IdentityService() *iam.IdentityService {
	return m.identityService
}

// OperationAccountService 返回运营账号注册服务。
func (m *Module) OperationAccountService() *iam.OperationAccountService {
	return m.operationAccountSvc
}

// ProfileLinkService 返回 ProfileLink 服务。
// 用于 Profile 访问校验和关系查询。
func (m *Module) ProfileLinkService() *iam.ProfileLinkService {
	return m.profileLinkSvc
}

// WeChatAppService 返回微信应用服务
// 用于查询微信应用信息（AppID、AppSecret 等）
func (m *Module) WeChatAppService() *iam.WeChatAppService {
	return m.wechatAppService
}

// AuthzSnapshotLoader 返回 IAM 授权快照加载器（gRPC GetAuthorizationSnapshot + 本地缓存）。
func (m *Module) AuthzSnapshotLoader() *iam.AuthzSnapshotLoader {
	return m.authzSnapshotLoader
}

// ObjectAuthorizationChecker returns the authoritative IAM AuthZ v3 object checker.
func (m *Module) ObjectAuthorizationChecker() *iam.ObjectAuthorizationChecker {
	return m.objectAuthzChecker
}

// IsEnabled 检查 IAM 模块是否启用
func (m *Module) IsEnabled() bool {
	return m.client != nil && m.client.IsEnabled()
}

// ValidateRequiredAuthzRuntime verifies the production authorization
// dependencies before any protected transport is started.
func (m *Module) ValidateRequiredAuthzRuntime(ctx context.Context) error {
	if m == nil || !m.IsEnabled() {
		return fmt.Errorf("IAM integration is required")
	}
	if m.SDKTokenVerifier() == nil {
		return fmt.Errorf("IAM token verifier is required")
	}
	if m.serviceAuthHelper == nil {
		return fmt.Errorf("IAM service authentication is required for AuthZ v3")
	}
	if m.authzSnapshotLoader == nil {
		return fmt.Errorf("IAM AuthZ v3 snapshot loader is required")
	}
	if m.objectAuthzChecker == nil {
		return fmt.Errorf("IAM AuthZ v3 object checker is required")
	}
	if err := m.HealthCheck(ctx); err != nil {
		return fmt.Errorf("IAM health check failed: %w", err)
	}
	// A read-only sentinel snapshot proves that AuthZ v3 registration, service
	// identity ACL, policy runtime, and the configured authorization domain are
	// all usable before protected traffic is accepted.
	if _, err := m.authzSnapshotLoader.Load(ctx, m.authzSnapshotLoader.AuthorizationDomain(), "1"); err != nil {
		return fmt.Errorf("IAM AuthZ v3 startup probe failed: %w", err)
	}
	return nil
}

// Close 关闭 IAM 模块
func (m *Module) Close() error {
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
func (m *Module) HealthCheck(ctx context.Context) error {
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
			Address:                      opts.GRPC.Address,
			Timeout:                      opts.GRPC.Timeout,
			RetryMax:                     opts.GRPC.RetryMax,
			KeepaliveTime:                opts.GRPC.KeepaliveTime,
			KeepaliveTimeout:             opts.GRPC.KeepaliveTimeout,
			KeepalivePermitWithoutStream: opts.GRPC.KeepalivePermitWithoutStream,
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
	iamOpts.AuthzDomainOverride = opts.AuthzDomainOverride

	return iamOpts
}
