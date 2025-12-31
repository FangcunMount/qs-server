package wechatapi

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/silenceper/wechat/v2"
	"github.com/silenceper/wechat/v2/cache"
	miniConfig "github.com/silenceper/wechat/v2/miniprogram/config"
	offiaConfig "github.com/silenceper/wechat/v2/officialaccount/config"
)

// AccessTokenResult 访问令牌结果
type AccessTokenResult struct {
	Token     string
	ExpiresAt time.Time
}

// TokenProvider 微信访问令牌提供器（使用 silenceper SDK）
type TokenProvider struct {
	cache cache.Cache // SDK 使用的缓存（可选，传 nil 则 SDK 使用内存缓存）
}

// NewTokenProvider 创建微信访问令牌提供器实例
func NewTokenProvider(sdkCache cache.Cache) *TokenProvider {
	return &TokenProvider{
		cache: sdkCache,
	}
}

// FetchMiniProgramToken 获取小程序访问令牌
func (p *TokenProvider) FetchMiniProgramToken(ctx context.Context, appID, appSecret string) (*AccessTokenResult, error) {
	if appID == "" || appSecret == "" {
		return nil, errors.New("appID and appSecret cannot be empty")
	}

	accessToken, expiresIn, err := p.fetchMiniProgramToken(appID, appSecret)
	if err != nil {
		return nil, err
	}

	return &AccessTokenResult{
		Token:     accessToken,
		ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second),
	}, nil
}

// FetchOfficialAccountToken 获取公众号访问令牌
func (p *TokenProvider) FetchOfficialAccountToken(ctx context.Context, appID, appSecret string) (*AccessTokenResult, error) {
	if appID == "" || appSecret == "" {
		return nil, errors.New("appID and appSecret cannot be empty")
	}

	accessToken, expiresIn, err := p.fetchOfficialAccountToken(appID, appSecret)
	if err != nil {
		return nil, err
	}

	return &AccessTokenResult{
		Token:     accessToken,
		ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second),
	}, nil
}

// fetchMiniProgramToken 获取小程序 access_token
func (p *TokenProvider) fetchMiniProgramToken(appID, appSecret string) (string, int64, error) {
	wc := wechat.NewWechat()
	cfg := &miniConfig.Config{
		AppID:     appID,
		AppSecret: appSecret,
		Cache:     p.cache,
	}

	miniProgram := wc.GetMiniProgram(cfg)
	// 获取 access token context
	accessToken, err := miniProgram.GetContext().GetAccessToken()
	if err != nil {
		return "", 0, fmt.Errorf("failed to get miniprogram access token: %w", err)
	}

	// SDK 返回的 token 已经是字符串，默认有效期 7200 秒
	return accessToken, 7200, nil
}

// fetchOfficialAccountToken 获取公众号 access_token
func (p *TokenProvider) fetchOfficialAccountToken(appID, appSecret string) (string, int64, error) {
	wc := wechat.NewWechat()
	cfg := &offiaConfig.Config{
		AppID:     appID,
		AppSecret: appSecret,
		Cache:     p.cache,
	}

	officialAccount := wc.GetOfficialAccount(cfg)
	// 获取 access token context
	accessToken, err := officialAccount.GetContext().GetAccessToken()
	if err != nil {
		return "", 0, fmt.Errorf("failed to get official account access token: %w", err)
	}

	// SDK 返回的 token 已经是字符串，默认有效期 7200 秒
	return accessToken, 7200, nil
}
