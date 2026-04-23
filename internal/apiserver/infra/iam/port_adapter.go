package iam

import (
	"context"
	"fmt"

	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func (s *WeChatAppService) ResolveWeChatAppConfig(ctx context.Context, appID string) (*iambridge.WeChatAppConfig, error) {
	resp, err := s.GetWechatApp(ctx, appID)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.App == nil {
		return nil, fmt.Errorf("IAM returned empty wechat app")
	}
	return &iambridge.WeChatAppConfig{
		AppID:     resp.App.GetAppId(),
		AppSecret: resp.App.GetAppSecret(),
	}, nil
}

func (s *IdentityService) ResolveUserNames(ctx context.Context, ids []meta.ID) map[string]string {
	return ResolveUserNames(ctx, s, ids)
}
