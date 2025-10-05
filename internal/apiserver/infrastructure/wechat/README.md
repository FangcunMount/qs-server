# Wechat SDK 适配器

## 📋 目录说明

本目录包含微信SDK的适配器实现，属于 Infrastructure 层的外部服务集成部分。

## 🎯 职责

### 防腐层（Anti-Corruption Layer）
隔离第三方微信SDK (`github.com/silenceper/wechat/v2`)，防止外部依赖污染领域模型。

### 适配器模式
实现 `domain/wechat/port/sdk.go` 中定义的 `WechatSDK` 接口：
- Code2Session（小程序登录）
- DecryptPhoneNumber（解密手机号）
- GetUserInfo（获取公众号用户信息）
- SendSubscribeMessage（发送订阅消息）
- SendTemplateMessage（发送模板消息）

### 工厂模式
管理微信客户端的生命周期：
- 按 AppID 创建和缓存小程序客户端
- 按 AppID 创建和缓存公众号客户端
- 集成 Redis 缓存 access_token

## 📁 文件说明

- `client_factory.go` - 微信客户端工厂，管理小程序和公众号客户端

## 🏗️ 架构位置

```
Application Layer
       ↓ 使用
Domain Port (WechatSDK 接口)
       ↑ 实现
Infrastructure Layer
    └── wechat/
        └── client_factory.go  ← 这里
```

## 🔌 依赖

- **第三方SDK**: `github.com/silenceper/wechat/v2`
- **领域接口**: `domain/wechat/port.WechatSDK`
- **仓储接口**: `domain/wechat/port.AppRepository`
- **缓存**: Redis

## 💡 使用示例

```go
// 1. 创建工厂
factory := wechat.NewWxClientFactory(appRepo, redisClient)

// 2. 获取小程序客户端
mini, err := factory.GetMini(ctx, "wx123456")

// 3. code换session
openID, sessionKey, unionID, err := factory.Code2Session(ctx, appID, jsCode)

// 4. 解密手机号
phone, err := factory.DecryptPhoneNumber(ctx, appID, sessionKey, encryptedData, iv)
```

## ⚠️ 注意事项

1. **缓存管理**
   - 客户端实例会被缓存，避免重复创建
   - 配置更新时需调用 `ClearCache(appID)` 清除缓存

2. **错误处理**
   - 微信API错误会被包装为 `code.ErrExternal`
   - 数据库错误会被包装为 `code.ErrDatabase`

3. **线程安全**
   - 使用 `sync.Map` 存储缓存，线程安全
   - 使用 `sync.RWMutex` 保护并发访问

## 🔄 扩展点

如果需要支持更多微信API：
1. 在 `domain/wechat/port/sdk.go` 添加接口方法
2. 在 `client_factory.go` 实现该方法
3. 调用对应的第三方SDK方法

## 🧪 测试

使用 Mock 测试时：
1. Mock `WechatSDK` 接口，而不是直接 Mock 第三方SDK
2. 这样可以在不依赖外部服务的情况下测试业务逻辑

```go
type MockWechatSDK struct {
    mock.Mock
}

func (m *MockWechatSDK) Code2Session(ctx context.Context, appID, jsCode string) (string, string, string, error) {
    args := m.Called(ctx, appID, jsCode)
    return args.String(0), args.String(1), args.String(2), args.Error(3)
}
```
