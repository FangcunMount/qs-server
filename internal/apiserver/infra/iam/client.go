package iam

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	authnv1 "github.com/FangcunMount/iam-contracts/api/grpc/iam/authn/v1"
	sdk "github.com/FangcunMount/iam-contracts/pkg/sdk"
)

// IAMOptions 简化的 IAM 配置（避免导入循环）
type IAMOptions struct {
	Enabled           bool
	GRPCEnabled       bool
	JWKSEnabled       bool
	GRPC              *GRPCOptions
	JWT               *JWTOptions
	JWKS              *JWKSOptions
	UserCache         *CacheOptions
	GuardianshipCache *CacheOptions
}

type GRPCOptions struct {
	Address  string
	Timeout  time.Duration
	RetryMax int
	TLS      *TLSOptions
}

type TLSOptions struct {
	Enabled  bool
	CAFile   string
	CertFile string
	KeyFile  string
}

type JWTOptions struct {
	Issuer         string
	Audience       []string
	Algorithms     []string
	ClockSkew      time.Duration
	RequiredClaims []string
}

type JWKSOptions struct {
	URL             string
	GRPCEndpoint    string // gRPC 降级端点（HTTP 失败时使用）
	RefreshInterval time.Duration
	CacheTTL        time.Duration
}

type CacheOptions struct {
	Enabled bool
	TTL     time.Duration
	MaxSize int
}

// Client IAM SDK 客户端封装
type Client struct {
	sdk     *sdk.Client
	config  *IAMOptions
	enabled bool
}

// NewClient 创建 IAM 客户端
func NewClient(ctx context.Context, opts *IAMOptions) (*Client, error) {
	if opts == nil || !opts.Enabled {
		log.Info("IAM integration is disabled, skipping client initialization")
		return &Client{
			enabled: false,
			config:  opts,
		}, nil
	}

	log.Info("Initializing IAM SDK client", log.String("address", opts.GRPC.Address))

	// 构建 SDK 配置
	sdkConfig := &sdk.Config{
		Endpoint: opts.GRPC.Address,
		Timeout:  opts.GRPC.Timeout,
	}

	// 配置 mTLS
	if opts.GRPC.TLS != nil && opts.GRPC.TLS.Enabled {
		sdkConfig.TLS = &sdk.TLSConfig{
			Enabled:    true,
			CACert:     opts.GRPC.TLS.CAFile,
			ClientCert: opts.GRPC.TLS.CertFile,
			ClientKey:  opts.GRPC.TLS.KeyFile,
		}
	}

	// 配置重试策略
	if opts.GRPC.RetryMax > 0 {
		sdkConfig.Retry = &sdk.RetryConfig{
			MaxAttempts: opts.GRPC.RetryMax,
		}
	}

	// 创建 SDK 客户端
	client, err := sdk.NewClient(ctx, sdkConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create IAM SDK client: %w", err)
	}

	log.Info("IAM SDK client initialized successfully")

	return &Client{
		sdk:     client,
		config:  opts,
		enabled: true,
	}, nil
}

// SDK 返回底层的 SDK 客户端
func (c *Client) SDK() *sdk.Client {
	return c.sdk
}

// IsEnabled 返回 IAM 集成是否启用
func (c *Client) IsEnabled() bool {
	return c.enabled
}

// Config 返回配置
func (c *Client) Config() *IAMOptions {
	return c.config
}

// Close 关闭客户端连接
func (c *Client) Close() error {
	if !c.enabled || c.sdk == nil {
		return nil
	}

	log.Info("Closing IAM SDK client")
	return c.sdk.Close()
}

// HealthCheck 健康检查
// 通过尝试调用 IAM 服务来验证连接是否正常
func (c *Client) HealthCheck(ctx context.Context) error {
	if !c.enabled {
		return nil
	}

	if c.sdk == nil {
		return fmt.Errorf("IAM SDK client is nil")
	}

	// 尝试使用一个空的 token 调用 VerifyToken
	// 如果 IAM 服务可达，应该返回 token 无效的错误，而不是连接错误
	// 这样可以验证 gRPC 连接和证书是否正常
	_, err := c.sdk.Auth().VerifyToken(ctx, &authnv1.VerifyTokenRequest{
		AccessToken: "", // 空 token，预期返回无效错误
	})
	if err != nil {
		// 检查是否是连接错误（而非业务错误）
		// 业务错误（如 token 无效）是预期的，说明服务可达
		errStr := err.Error()
		// 如果是 InvalidArgument 或 Unauthenticated 错误，说明服务可达
		if strings.Contains(errStr, "InvalidArgument") ||
			strings.Contains(errStr, "Unauthenticated") ||
			strings.Contains(errStr, "invalid") ||
			strings.Contains(errStr, "token") {
			return nil // 服务可达，健康
		}
		// 其他错误（如连接失败、证书错误等）则认为不健康
		return fmt.Errorf("IAM health check failed: %w", err)
	}

	return nil
}
