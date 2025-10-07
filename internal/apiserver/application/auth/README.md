# Auth Application Layer - 认证应用层

## 目录说明

本目录包含所有认证相关的应用服务，遵循**单一职责原则**。认证是独立的关注点，与用户管理、账号管理分离。

## 服务列表

### 1. Authenticator (authenticator.go)
- **职责**: 用户名密码认证
- **方法**: `Authenticate(ctx, username, password)` - 验证用户名和密码
- **用途**: 传统的用户名密码登录

### 2. WechatAuthenticator (wechat_authenticator.go)
- **职责**: 微信登录认证
- **方法**: `LoginWithMiniProgram(ctx, req)` - 处理微信小程序登录
- **用途**: 微信小程序/公众号的OAuth登录流程

## 设计理念

### 为什么认证要独立？

1. **关注点分离**: 认证逻辑与用户管理、账号管理是不同的关注点
   - **用户管理** (`user/`): 用户的 CRUD 操作
   - **账号管理** (`user/wechat/`): 微信账号的创建、更新
   - **认证** (`auth/`): 验证身份的合法性

2. **安全性**: 认证涉及密码验证、token生成等安全敏感操作，应该集中管理

3. **可扩展性**: 便于添加新的认证方式（如手机验证码、第三方OAuth等）

## 认证流程

### 用户名密码认证

```go
authenticator := auth.NewAuthenticator(userRepo)
user, err := authenticator.Authenticate(ctx, "username", "password")
// 验证通过后，由 gin-jwt 中间件生成 token
```

### 微信小程序认证

```go
wechatAuth := auth.NewWechatAuthenticator(wxAccountRepo, mergeLogRepo, appRepo, userRepo)

loginReq := &auth.LoginRequest{
    AppID:    "wx1234567890",
    Platform: "mini",
    Code:     "code_from_wechat",
    OpenID:   "openid_xxx",
    UnionID:  &unionID,
    Nickname: "张三",
    Avatar:   "https://...",
}

loginResp, err := wechatAuth.LoginWithMiniProgram(ctx, loginReq)
// loginResp 包含: UserID, IsNewUser, SessionKey, NeedBindInfo
```

## 与其他层的关系

```
Controller/Handler
    ↓
Auth Application Layer (认证)
    ↓
User Application Layer (用户/账号管理)
    ↓
Domain Layer (领域模型)
    ↓
Infrastructure Layer (数据持久化)
```

## 注意事项

1. **Authenticator vs WechatAccountCreator**:
   - `Authenticator`: 负责**验证**身份，属于认证逻辑
   - `WechatAccountCreator`: 负责**创建/更新**微信账号，属于账号管理

2. **Token生成**: 认证器只负责验证身份，token的生成由上层的 middleware (如 gin-jwt) 处理

3. **密码验证**: 使用 bcrypt 算法，确保与用户创建时一致

## 未来扩展

可以添加更多认证方式：

- `PhoneAuthenticator` - 手机验证码登录
- `EmailAuthenticator` - 邮箱验证码登录  
- `OAuthAuthenticator` - 第三方OAuth登录（微博、GitHub等）
- `BiometricAuthenticator` - 生物识别认证
