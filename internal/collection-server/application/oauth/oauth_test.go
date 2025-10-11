package oauth_test

import (
    "context"
    "testing"

    "github.com/fangcun-mount/qs-server/internal/collection-server/application/oauth"
    "github.com/fangcun-mount/qs-server/internal/collection-server/infrastructure/auth"
    grpcclient "github.com/fangcun-mount/qs-server/internal/collection-server/infrastructure/grpc"
    "github.com/fangcun-mount/qs-server/internal/collection-server/infrastructure/wechat"
)

// TestOAuthUsageExample OAuth 使用示例
func TestOAuthUsageExample(t *testing.T) {
    // 这是一个示例测试，展示如何使用 OAuth 模块
    // 在实际测试中需要使用 mock 对象

    // 1. 准备依赖
    userServiceClient := &grpcclient.UserServiceClient{} // 实际使用时需要初始化
    miniProgramClient := wechat.NewMiniProgramClient("test_appid", "test_secret")
    jwtManager := auth.NewJWTManager("test_secret", 24*7)
    appID := "test_appid"

    // 2. 创建 OAuth 工厂
    factory := oauth.NewFactory(
        userServiceClient,
        miniProgramClient,
        jwtManager,
        appID,
    )

    // 3. 创建微信 OAuth 实例
    wechatOAuth, err := factory.CreateOAuth(oauth.OAuthTypeWechat)
    if err != nil {
        t.Fatalf("Failed to create wechat oauth: %v", err)
    }

    // 4. 使用 OAuth 进行登录
    ctx := context.Background()
    code := "test_code_from_miniprogram"
    payload := map[string]interface{}{
        "nickname": "测试用户",
        "avatar":   "https://example.com/avatar.jpg",
    }

    // 注意：这个测试不会真正执行，因为需要真实的微信 code
    _, err = wechatOAuth.Code2Token(ctx, code, payload)
    if err != nil {
        // 预期会失败，因为这是一个示例
        t.Logf("Expected error in example test: %v", err)
    }
}

// Example_OAuthFactory 展示如何使用 OAuth 工厂
func Example_OAuthFactory() {
    // 初始化依赖
    userServiceClient := &grpcclient.UserServiceClient{}
    miniProgramClient := wechat.NewMiniProgramClient("your_appid", "your_secret")
    jwtManager := auth.NewJWTManager("your_jwt_secret", 24*7)
    appID := "your_appid"

    // 创建工厂
    factory := oauth.NewFactory(
        userServiceClient,
        miniProgramClient,
        jwtManager,
        appID,
    )

    // 创建微信 OAuth
    wechatOAuth, _ := factory.CreateOAuth(oauth.OAuthTypeWechat)

    // 使用 OAuth 进行登录
    ctx := context.Background()
    token, err := wechatOAuth.Code2Token(ctx, "code_from_wechat", map[string]interface{}{
        "nickname": "张三",
        "avatar":   "https://example.com/avatar.jpg",
    })

    if err != nil {
        // 处理错误
        return
    }

    // 使用 token
    _ = token
}
