package oauth

import (
    "context"
    "fmt"

    "github.com/fangcun-mount/qs-server/internal/collection-server/infrastructure/wechat"
    "github.com/fangcun-mount/qs-server/pkg/log"
)

// WechatUserInfo 微信用户信息
type WechatUserInfo struct {
    OpenID     string
    UnionID    string
    SessionKey string
    Nickname   string
    Avatar     string
}

// GetUniqueID 获取唯一标识（unionid）
func (w *WechatUserInfo) GetUniqueID() string {
    return w.UnionID
}

// GetOpenID 获取平台标识（openid）
func (w *WechatUserInfo) GetOpenID() string {
    return w.OpenID
}

// GetNickname 获取昵称
func (w *WechatUserInfo) GetNickname() string {
    return w.Nickname
}

// GetAvatar 获取头像
func (w *WechatUserInfo) GetAvatar() string {
    return w.Avatar
}

// WechatOAuth 微信 OAuth 实现
// 对应 PHP 的 WxOAuth 类
type WechatOAuth struct {
    *BaseOAuth
    miniProgramClient *wechat.MiniProgramClient
    appID             string
}

// NewWechatOAuth 创建微信 OAuth 实例
func NewWechatOAuth(
    tokenGenerator TokenGenerator,
    userLoader UserLoader,
    miniProgramClient *wechat.MiniProgramClient,
    appID string,
) *WechatOAuth {
    return &WechatOAuth{
        BaseOAuth:         NewBaseOAuth(tokenGenerator, userLoader),
        miniProgramClient: miniProgramClient,
        appID:             appID,
    }
}

// Code2Token 通过微信授权码换取访问令牌
func (w *WechatOAuth) Code2Token(ctx context.Context, code string, payload map[string]interface{}) (string, error) {
    // 确保 payload 中包含 app_id
    if payload == nil {
        payload = make(map[string]interface{})
    }
    payload["app_id"] = w.appID

    return w.BaseOAuth.Code2Token(
        ctx,
        code,
        payload,
        w.queryUserInfo,
        w.checkUserInfo,
    )
}

// queryUserInfo 查询微信用户信息
func (w *WechatOAuth) queryUserInfo(ctx context.Context, code string, payload map[string]interface{}) (UserInfo, error) {
    log.Debugf("Calling wechat code2session with code: %s", code)

    // 调用微信 API
    sessionResp, err := w.miniProgramClient.Code2Session(ctx, code)
    if err != nil {
        log.Errorf("Failed to call code2session: %v", err)
        return nil, fmt.Errorf("微信查询用户信息失败: %w", err)
    }

    log.Infof("Successfully got wechat session for openid: %s", sessionResp.OpenID)

    // 从 payload 中获取额外的用户信息
    nickname, _ := payload["nickname"].(string)
    avatar, _ := payload["avatar"].(string)

    return &WechatUserInfo{
        OpenID:     sessionResp.OpenID,
        UnionID:    sessionResp.UnionID,
        SessionKey: sessionResp.SessionKey,
        Nickname:   nickname,
        Avatar:     avatar,
    }, nil
}

// checkUserInfo 校验微信用户信息
func (w *WechatOAuth) checkUserInfo(ctx context.Context, userInfo UserInfo, payload map[string]interface{}) error {
    // 微信用户信息基本校验
    if userInfo.GetOpenID() == "" {
        return fmt.Errorf("invalid wechat user info: openid is empty")
    }

    // 可以添加更多的业务校验逻辑
    // 例如：检查是否在黑名单、是否被封禁等

    return nil
}
