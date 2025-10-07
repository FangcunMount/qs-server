package wechat

import (
	"context"
	"fmt"

	wechatSDK "github.com/silenceper/wechat/v2"
	"github.com/silenceper/wechat/v2/cache"
	"github.com/silenceper/wechat/v2/miniprogram"
	"github.com/silenceper/wechat/v2/miniprogram/config"
)

// Code2SessionResult 微信小程序 code2Session 结果
type Code2SessionResult struct {
	OpenID     string
	SessionKey string
	UnionID    string
}

// MiniProgramClient 小程序客户端
// 封装 github.com/silenceper/wechat/v2 的小程序 SDK
type MiniProgramClient struct {
	mini *miniprogram.MiniProgram
}

// NewMiniProgramClient 创建小程序客户端
func NewMiniProgramClient(appID, appSecret string) *MiniProgramClient {
	wc := wechatSDK.NewWechat()

	// 使用内存缓存
	memCache := cache.NewMemory()

	cfg := &config.Config{
		AppID:     appID,
		AppSecret: appSecret,
		Cache:     memCache,
	}

	mini := wc.GetMiniProgram(cfg)

	return &MiniProgramClient{
		mini: mini,
	}
}

// Code2Session 通过 code 换取 openid 和 session_key
func (c *MiniProgramClient) Code2Session(ctx context.Context, code string) (*Code2SessionResult, error) {
	result, err := c.mini.GetAuth().Code2Session(code)
	if err != nil {
		return nil, fmt.Errorf("code2session failed: %w", err)
	}

	if result.ErrCode != 0 {
		return nil, fmt.Errorf("wechat api error: code=%d, msg=%s", result.ErrCode, result.ErrMsg)
	}

	return &Code2SessionResult{
		OpenID:     result.OpenID,
		SessionKey: result.SessionKey,
		UnionID:    result.UnionID,
	}, nil
}

// GetAccessToken 获取 access_token（用于调用其他微信接口）
func (c *MiniProgramClient) GetAccessToken() (string, error) {
	token, err := c.mini.GetContext().GetAccessToken()
	if err != nil {
		return "", fmt.Errorf("get access token failed: %w", err)
	}
	return token, nil
}

// GetMiniProgram 获取底层的 MiniProgram 实例（用于高级功能）
func (c *MiniProgramClient) GetMiniProgram() *miniprogram.MiniProgram {
	return c.mini
}
