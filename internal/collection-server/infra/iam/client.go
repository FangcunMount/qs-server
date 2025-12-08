package iam

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
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
func (c *Client) HealthCheck(ctx context.Context) error {
	if !c.enabled {
		return nil
	}

	// 简单的健康检查：尝试获取 JWKS（不会实际调用）
	// 或者可以实现一个专门的 Ping 方法
	return nil
}
