package iam

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	sdk "github.com/FangcunMount/iam-contracts/pkg/sdk"
	"github.com/FangcunMount/iam-contracts/pkg/sdk/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// ServiceAuthHelper 服务间认证助手封装
// 用于简化 QS 服务以服务身份调用 IAM（而非用户身份）
type ServiceAuthHelper struct {
	helper *auth.ServiceAuthHelper
	config *ServiceAuthConfig
}

// ServiceAuthConfig 服务间认证配置
type ServiceAuthConfig struct {
	ServiceID      string   // 当前服务标识（如 "qs-service"）
	TargetAudience []string // 目标服务（如 ["iam-service"]）
	TokenTTL       int64    // Token 有效期（秒）
	RefreshBefore  int64    // 提前刷新时间（秒）
}

// NewServiceAuthHelper 创建服务间认证助手
// 需要传入已初始化的 IAM Client
func NewServiceAuthHelper(ctx context.Context, client *Client, config *ServiceAuthConfig) (*ServiceAuthHelper, error) {
	if client == nil || !client.enabled {
		return nil, fmt.Errorf("IAM client not enabled")
	}

	if config == nil {
		return nil, fmt.Errorf("service auth config is nil")
	}

	// 构建 SDK ServiceAuthConfig
	sdkConfig := &sdk.ServiceAuthConfig{
		ServiceID:      config.ServiceID,
		TargetAudience: config.TargetAudience,
	}

	// SDK 使用 time.Duration，需要从秒转换
	if config.TokenTTL > 0 {
		sdkConfig.TokenTTL = time.Duration(config.TokenTTL) * time.Second
	}
	if config.RefreshBefore > 0 {
		sdkConfig.RefreshBefore = time.Duration(config.RefreshBefore) * time.Second
	}

	// 使用 SDK 创建 ServiceAuthHelper
	helper, err := sdk.NewServiceAuthHelper(sdkConfig, client.sdk)
	if err != nil {
		return nil, fmt.Errorf("failed to create service auth helper: %w", err)
	}

	logger.L(ctx).Infow("ServiceAuthHelper initialized",
		"component", "iam.service_auth",
		"service_id", config.ServiceID,
		"target_audience", config.TargetAudience,
		"result", "success",
	)

	return &ServiceAuthHelper{
		helper: helper,
		config: config,
	}, nil
}

// GetToken 获取当前有效的服务 Token
func (h *ServiceAuthHelper) GetToken(ctx context.Context) (string, error) {
	if h.helper == nil {
		return "", fmt.Errorf("service auth helper not initialized")
	}
	return h.helper.GetToken(ctx)
}

// NewAuthenticatedContext 创建带认证信息的 Context
func (h *ServiceAuthHelper) NewAuthenticatedContext(ctx context.Context) (context.Context, error) {
	if h.helper == nil {
		return nil, fmt.Errorf("service auth helper not initialized")
	}
	return h.helper.NewAuthenticatedContext(ctx)
}

// CallWithAuth 使用认证信息执行调用
func (h *ServiceAuthHelper) CallWithAuth(ctx context.Context, fn func(ctx context.Context) error) error {
	if h.helper == nil {
		return fmt.Errorf("service auth helper not initialized")
	}
	return h.helper.CallWithAuth(ctx, fn)
}

// GetRequestMetadata 实现 credentials.PerRPCCredentials 接口
// 用于 gRPC WithPerRPCCredentials
func (h *ServiceAuthHelper) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	token, err := h.GetToken(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"authorization": "Bearer " + token,
	}, nil
}

// RequireTransportSecurity 实现 credentials.PerRPCCredentials 接口
func (h *ServiceAuthHelper) RequireTransportSecurity() bool {
	return false // 根据实际需求设置，在 mTLS 环境下可以返回 true
}

// Stats 获取刷新统计
func (h *ServiceAuthHelper) Stats() auth.RefreshStats {
	if h.helper == nil {
		return auth.RefreshStats{}
	}
	return h.helper.Stats()
}

// Stop 停止后台刷新
func (h *ServiceAuthHelper) Stop() {
	if h.helper != nil {
		h.helper.Stop()
	}
	logger.L(context.Background()).Debugw("ServiceAuthHelper stopped",
		"component", "iam.service_auth",
	)
}

// 确保实现 PerRPCCredentials 接口
var _ credentials.PerRPCCredentials = (*ServiceAuthHelper)(nil)

// DialWithServiceAuth 创建带服务认证的 gRPC 连接
// 使用示例：
//
//	conn, err := DialWithServiceAuth(authHelper, "other-service:8081", opts...)
func DialWithServiceAuth(authHelper *ServiceAuthHelper, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	if authHelper == nil {
		return nil, fmt.Errorf("service auth helper is nil")
	}

	// 添加 PerRPCCredentials 到 dial options
	opts = append(opts, grpc.WithPerRPCCredentials(authHelper))

	conn, err := grpc.Dial(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", target, err)
	}

	return conn, nil
}
