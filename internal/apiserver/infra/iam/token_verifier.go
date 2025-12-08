package iam

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/log"
	sdk "github.com/FangcunMount/iam-contracts/pkg/sdk"
	"github.com/FangcunMount/iam-contracts/pkg/sdk/auth"
)

// TokenVerifier Token 验证器封装
// 使用 IAM SDK 的 auth.TokenVerifier，支持本地 JWKS 验签 + 远程降级
type TokenVerifier struct {
	verifier    *auth.TokenVerifier
	jwksManager *auth.JWKSManager
}

// NewTokenVerifier 创建 Token 验证器（使用 SDK）
// SDK 内部实现：
// 1. JWKS 本地验签（优先，高性能）
// 2. gRPC 远程验证（降级）
// 3. 缓存管理和自动刷新
// 4. 熔断保护
func NewTokenVerifier(ctx context.Context, client *Client) (*TokenVerifier, error) {
	if client == nil || !client.enabled {
		return nil, fmt.Errorf("IAM client not enabled")
	}

	config := client.config
	if config == nil {
		return nil, fmt.Errorf("IAM config is nil")
	}

	// 构建 SDK TokenVerifyConfig
	verifyCfg := &sdk.TokenVerifyConfig{}
	if config.JWT != nil {
		verifyCfg.AllowedAudience = config.JWT.Audience
		verifyCfg.AllowedIssuer = config.JWT.Issuer
		verifyCfg.ClockSkew = config.JWT.ClockSkew
		// SDK v0.0.5 新增支持
		verifyCfg.RequiredClaims = config.JWT.RequiredClaims
		verifyCfg.Algorithms = config.JWT.Algorithms
	}

	// 构建 SDK JWKSConfig（如果启用 JWKS 本地验签）
	var jwksCfg *sdk.JWKSConfig
	if config.JWKSEnabled && config.JWKS != nil {
		jwksCfg = &sdk.JWKSConfig{
			URL:             config.JWKS.URL,
			GRPCEndpoint:    config.JWKS.GRPCEndpoint, // gRPC 降级端点
			RefreshInterval: config.JWKS.RefreshInterval,
			CacheTTL:        config.JWKS.CacheTTL,
			FallbackOnError: true, // 失败时使用缓存
		}
		log.Infof("JWKS enabled: URL=%s, GRPCEndpoint=%s, RefreshInterval=%v, CacheTTL=%v",
			config.JWKS.URL, config.JWKS.GRPCEndpoint, config.JWKS.RefreshInterval, config.JWKS.CacheTTL)
	} else {
		log.Warn("JWKS disabled, will use remote verification only")
	}

	// 使用 SDK 的 NewTokenVerifier（自动创建 JWKSManager 和选择验证策略）
	verifier, err := sdk.NewTokenVerifier(verifyCfg, jwksCfg, client.sdk)
	if err != nil {
		return nil, fmt.Errorf("failed to create token verifier: %w", err)
	}

	log.Info("Token verifier initialized successfully (using IAM SDK)")
	log.Infof("  Strategy: %s", verifier.Strategy().Name())

	return &TokenVerifier{
		verifier: verifier,
	}, nil
}

// Verify 验证 Token
// 返回 SDK 的 VerifyResult，包含完整的 Claims 信息
func (v *TokenVerifier) Verify(ctx context.Context, token string) (*auth.VerifyResult, error) {
	if v.verifier == nil {
		return nil, fmt.Errorf("token verifier not initialized")
	}
	return v.verifier.Verify(ctx, token, nil)
}

// VerifyWithOptions 验证 Token（带选项）
func (v *TokenVerifier) VerifyWithOptions(ctx context.Context, token string, opts *auth.VerifyOptions) (*auth.VerifyResult, error) {
	if v.verifier == nil {
		return nil, fmt.Errorf("token verifier not initialized")
	}
	return v.verifier.Verify(ctx, token, opts)
}

// SDKVerifier 返回底层的 SDK TokenVerifier
func (v *TokenVerifier) SDKVerifier() *auth.TokenVerifier {
	return v.verifier
}

// Close 关闭验证器
func (v *TokenVerifier) Close() {
	if v.jwksManager != nil {
		v.jwksManager.Stop()
	}
	log.Debug("Token verifier closed")
}
