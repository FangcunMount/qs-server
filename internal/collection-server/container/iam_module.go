package container

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
)

// IAMModule IAM 集成模块
type IAMModule struct {
	client *iam.Client
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

	log.Info("IAM module initialized successfully")

	return &IAMModule{
		client: client,
	}, nil
}

// Client 返回 IAM 客户端
func (m *IAMModule) Client() *iam.Client {
	return m.client
}

// IsEnabled 检查 IAM 模块是否启用
func (m *IAMModule) IsEnabled() bool {
	return m.client != nil && m.client.IsEnabled()
}

// Close 关闭 IAM 模块
func (m *IAMModule) Close() error {
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
		Enabled:     opts.Enabled,
		GRPCEnabled: opts.GRPCEnabled,
		JWKSEnabled: opts.JWKSEnabled,
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
			RefreshInterval: opts.JWKS.RefreshInterval,
			CacheTTL:        opts.JWKS.CacheTTL,
			FetchStrategies: opts.JWKS.FetchStrategies,
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
