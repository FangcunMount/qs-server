package qrcode

import (
	"context"
	"testing"

	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
)

type wechatAppConfigProviderStub struct {
	enabled bool
	config  *iambridge.WeChatAppConfig
	err     error
	lastID  string
}

func (s *wechatAppConfigProviderStub) IsEnabled() bool {
	return s.enabled
}

func (s *wechatAppConfigProviderStub) ResolveWeChatAppConfig(_ context.Context, wechatAppID string) (*iambridge.WeChatAppConfig, error) {
	s.lastID = wechatAppID
	return s.config, s.err
}

func TestServiceGetWechatAppConfigUsesProvider(t *testing.T) {
	provider := &wechatAppConfigProviderStub{
		enabled: true,
		config: &iambridge.WeChatAppConfig{
			AppID:     "wx-app-id",
			AppSecret: "secret",
		},
	}
	svc := &service{
		config: &Config{WeChatAppID: "wechat-app-1"},
		wechatAppService: provider,
	}

	appID, appSecret, err := svc.getWechatAppConfig(context.Background())
	if err != nil {
		t.Fatalf("getWechatAppConfig() error = %v", err)
	}
	if provider.lastID != "wechat-app-1" {
		t.Fatalf("provider called with %q, want wechat-app-1", provider.lastID)
	}
	if appID != "wx-app-id" || appSecret != "secret" {
		t.Fatalf("unexpected config: appID=%q appSecret=%q", appID, appSecret)
	}
}
