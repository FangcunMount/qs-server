package oauth

import (
    "fmt"

    "github.com/fangcun-mount/qs-server/internal/collection-server/infrastructure/auth"
    grpcclient "github.com/fangcun-mount/qs-server/internal/collection-server/infrastructure/grpc"
    "github.com/fangcun-mount/qs-server/internal/collection-server/infrastructure/wechat"
)

// OAuthType OAuth 类型
type OAuthType string

const (
    // OAuthTypeWechat 微信小程序 OAuth
    OAuthTypeWechat OAuthType = "wechat"
    // OAuthTypeQWechat 企业微信 OAuth（暂未实现）
    OAuthTypeQWechat OAuthType = "qwechat"
)

// Factory OAuth 工厂
// 对应 PHP 的 BaseOAuth::createOAuth 静态方法
type Factory struct {
    tokenGenerator    TokenGenerator
    userLoader        UserLoader
    miniProgramClient *wechat.MiniProgramClient
    appID             string
}

// NewFactory 创建 OAuth 工厂
func NewFactory(
    userServiceClient *grpcclient.UserServiceClient,
    miniProgramClient *wechat.MiniProgramClient,
    jwtManager *auth.JWTManager,
    appID string,
) *Factory {
    return &Factory{
        tokenGenerator:    NewJWTTokenGenerator(jwtManager),
        userLoader:        NewGRPCUserLoader(userServiceClient),
        miniProgramClient: miniProgramClient,
        appID:             appID,
    }
}

// CreateOAuth 创建 OAuth 实例
func (f *Factory) CreateOAuth(oauthType OAuthType) (OAuth, error) {
    switch oauthType {
    case OAuthTypeWechat:
        return NewWechatOAuth(
            f.tokenGenerator,
            f.userLoader,
            f.miniProgramClient,
            f.appID,
        ), nil
    case OAuthTypeQWechat:
        // TODO: 实现企业微信 OAuth
        return nil, fmt.Errorf("qwechat oauth not implemented yet")
    default:
        return nil, fmt.Errorf("unsupported oauth type: %s", oauthType)
    }
}
