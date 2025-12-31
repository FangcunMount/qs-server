package wechat

import (
	"context"
	"fmt"

	"github.com/silenceper/wechat/v2"
	"github.com/silenceper/wechat/v2/cache"
	workConfig "github.com/silenceper/wechat/v2/work/config"

	authPort "github.com/FangcunMount/iam-contracts/internal/apiserver/domain/authn/authentication"
	wechatAuthPort "github.com/FangcunMount/iam-contracts/internal/apiserver/infra/wechatapi/port"
)

// IdentityProviderImpl 微信身份提供商的实现
// - 微信小程序登录：委托 IDP 模块提供的 AuthProvider 调用微信接口
// - 企业微信登录：暂时保留 silenceper SDK 实现
type IdentityProviderImpl struct {
	miniAuth wechatAuthPort.AuthProvider
	cache    cache.Cache
}

// 确保实现了接口
var _ authPort.IdentityProvider = (*IdentityProviderImpl)(nil)

// NewIdentityProvider 创建微信身份提供商
func NewIdentityProvider(miniAuth wechatAuthPort.AuthProvider, cache cache.Cache) authPort.IdentityProvider {
	return &IdentityProviderImpl{
		miniAuth: miniAuth,
		cache:    cache,
	}
}

// ExchangeWxMinipCode 微信小程序 jsCode 换取 session
// 文档: https://developers.weixin.qq.com/miniprogram/dev/OpenApiDoc/user-login/code2Session.html
func (p *IdentityProviderImpl) ExchangeWxMinipCode(ctx context.Context, appID, appSecret, jsCode string) (openID, unionID string, err error) {
	if p.miniAuth == nil {
		return "", "", fmt.Errorf("wechat auth provider is not configured")
	}

	result, err := p.miniAuth.Code2Session(ctx, appID, appSecret, jsCode)
	if err != nil {
		return "", "", fmt.Errorf("failed to call code2session: %w", err)
	}
	if result.OpenID == "" {
		return "", "", fmt.Errorf("openid is empty in code2session result")
	}
	return result.OpenID, result.UnionID, nil
}

// ExchangeWecomCode 企业微信 code 换取用户信息
// 文档: https://developer.work.weixin.qq.com/document/path/91023
func (p *IdentityProviderImpl) ExchangeWecomCode(ctx context.Context, corpID, agentID, corpSecret, code string) (openUserID, userID string, err error) {
	// 创建企业微信实例（依赖 silenceper SDK）
	cfg := &workConfig.Config{
		CorpID:     corpID,
		CorpSecret: corpSecret,
		AgentID:    agentID,
		Cache:      p.cache,
	}
	workApp := wechat.NewWechat().GetWork(cfg)

	// 获取用户信息
	userInfo, err := workApp.GetOauth().GetUserInfo(code)
	if err != nil {
		return "", "", fmt.Errorf("failed to get wecom user info: %w", err)
	}

	return userInfo.OpenID, userInfo.UserID, nil
}
