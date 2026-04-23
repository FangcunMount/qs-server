package iambridge

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type WeChatAppConfig struct {
	AppID     string
	AppSecret string
}

type WeChatAppConfigProvider interface {
	IsEnabled() bool
	ResolveWeChatAppConfig(ctx context.Context, wechatAppID string) (*WeChatAppConfig, error)
}

type IdentityResolver interface {
	IsEnabled() bool
	ResolveUserNames(ctx context.Context, ids []meta.ID) map[string]string
}

type GuardianshipReader interface {
	IsEnabled() bool
	ValidateChildExists(ctx context.Context, childID string) error
}
