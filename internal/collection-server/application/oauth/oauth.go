package oauth

import (
    "context"
    "fmt"
)

// OAuth 接口定义
// 对应 PHP 的 IOAuth 接口
type OAuth interface {
    // Code2Token 通过授权码换取访问令牌
    // code: 授权码
    // payload: 额外的载荷数据
    // 返回: JWT token
    Code2Token(ctx context.Context, code string, payload map[string]interface{}) (string, error)
}

// UserInfo 用户信息接口
type UserInfo interface {
    // GetUniqueID 获取用户唯一标识（如 unionid）
    GetUniqueID() string
    // GetOpenID 获取平台用户标识（如 openid）
    GetOpenID() string
    // GetNickname 获取用户昵称
    GetNickname() string
    // GetAvatar 获取用户头像
    GetAvatar() string
}

// TokenGenerator Token 生成器接口
type TokenGenerator interface {
    // GenerateToken 生成访问令牌
    GenerateToken(userID, appID, openID string) (string, error)
}

// UserLoader 用户加载器接口
type UserLoader interface {
    // LoadOrCreateUser 加载或创建用户
    // 返回: 用户ID, 是否新创建, 错误
    LoadOrCreateUser(ctx context.Context, userInfo UserInfo, payload map[string]interface{}) (string, bool, error)
}

// BaseOAuth OAuth 基础实现
// 对应 PHP 的 BaseOAuth 抽象类
type BaseOAuth struct {
    tokenGenerator TokenGenerator
    userLoader     UserLoader
}

// NewBaseOAuth 创建 OAuth 基础实例
func NewBaseOAuth(tokenGenerator TokenGenerator, userLoader UserLoader) *BaseOAuth {
    return &BaseOAuth{
        tokenGenerator: tokenGenerator,
        userLoader:     userLoader,
    }
}

// Code2Token 通过授权码换取访问令牌（模板方法模式）
// 这是一个最终方法，定义了完整的授权流程
func (b *BaseOAuth) Code2Token(
    ctx context.Context,
    code string,
    payload map[string]interface{},
    queryUserInfo func(ctx context.Context, code string, payload map[string]interface{}) (UserInfo, error),
    checkUserInfo func(ctx context.Context, userInfo UserInfo, payload map[string]interface{}) error,
) (string, error) {
    // 1. 查询用户信息（由具体实现提供）
    userInfo, err := queryUserInfo(ctx, code, payload)
    if err != nil {
        return "", fmt.Errorf("failed to query user info: %w", err)
    }

    // 2. 校验用户信息（由具体实现提供）
    if err := checkUserInfo(ctx, userInfo, payload); err != nil {
        return "", fmt.Errorf("failed to check user info: %w", err)
    }

    // 3. 加载或创建用户
    userID, _, err := b.userLoader.LoadOrCreateUser(ctx, userInfo, payload)
    if err != nil {
        return "", fmt.Errorf("failed to load user: %w", err)
    }

    // 4. 生成访问令牌
    appID, _ := payload["app_id"].(string)
    token, err := b.tokenGenerator.GenerateToken(userID, appID, userInfo.GetOpenID())
    if err != nil {
        return "", fmt.Errorf("failed to generate token: %w", err)
    }

    return token, nil
}
