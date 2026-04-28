package iam

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	idpv1 "github.com/FangcunMount/iam/api/grpc/iam/idp/v1"
	"github.com/FangcunMount/iam/pkg/sdk/idp"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
)

// WeChatAppService 微信应用服务封装
// 提供微信应用信息查询功能
type WeChatAppService struct {
	client  *idp.Client
	enabled bool
	limiter backpressure.Acquirer
}

// NewWeChatAppService 创建微信应用服务
func NewWeChatAppService(client *Client) (*WeChatAppService, error) {
	if client == nil || !client.enabled {
		return &WeChatAppService{enabled: false}, nil
	}

	sdkClient := client.SDK()
	if sdkClient == nil {
		return nil, fmt.Errorf("SDK client is nil")
	}

	idpClient := sdkClient.IDP()
	if idpClient == nil {
		return nil, fmt.Errorf("IDP client is nil")
	}

	logger.L(context.Background()).Infow("WeChatAppService initialized",
		"component", "iam.wechatapp",
		"result", "success",
	)
	return &WeChatAppService{
		client:  idpClient,
		enabled: true,
		limiter: client.Limiter(),
	}, nil
}

// IsEnabled 检查服务是否启用
func (s *WeChatAppService) IsEnabled() bool {
	return s.enabled
}

// GetWechatApp 获取微信应用信息
// appID: 微信应用ID（wechatappId），例如 "597792321089581614"
func (s *WeChatAppService) GetWechatApp(ctx context.Context, appID string) (*idpv1.GetWechatAppResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wechat app service not enabled")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.client.GetWechatApp(ctx, appID)
}

// Raw 返回原始 SDK 客户端（用于高级用法）
func (s *WeChatAppService) Raw() *idp.Client {
	return s.client
}

func (s *WeChatAppService) acquire(ctx context.Context) (context.Context, func(), error) {
	if s == nil || s.limiter == nil {
		return ctx, func() {}, nil
	}
	return s.limiter.Acquire(ctx)
}
