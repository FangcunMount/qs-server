# Wechat Services - 微信服务

## 概述

本目录包含微信生态相关的应用服务，负责处理微信小程序、微信公众号的账号管理和事件处理。

## 目录结构

```text
wechat/
├── account_manager.go              # 微信账号管理器（创建、更新、查询、SessionKey）
├── subscriber.go                   # 公众号订阅管理器（关注/取关）
├── wechat_phone_binder.go          # 手机号绑定器（账号合并）
└── README.md
```

**注意**: 
- 微信登录认证已移至 `application/auth/wechat_authenticator.go` (认证关注点分离)

## 服务列表

### 1. AccountManager - 微信账号管理器

**文件**: `account_manager.go`

**职责**: 统一管理微信账号的创建、更新、查询和 SessionKey 更新。

**主要方法**:

- `CreateOrUpdateMiniProgramAccount(ctx, appID, openID, unionID, nickname, avatar, sessionKey)` - 创建或更新小程序账号
- `CreateOrUpdateOfficialAccount(ctx, appID, openID, unionID, nickname, avatar)` - 创建或更新公众号账号
- `GetWechatAccountByOpenID(ctx, appID, platform, openID)` - 根据 OpenID 查询微信账号
- `UpdateSessionKey(ctx, appID, openID, sessionKey)` - 更新小程序 SessionKey

**核心功能**:

- 根据 OpenID 查找已有账号，存在则更新，不存在则创建
- 验证微信应用是否存在且已启用
- 更新用户资料（昵称、头像）
- 管理 SessionKey（仅小程序）

**使用场景**:

- 用户首次使用小程序时通过微信登录
- 用户关注公众号时创建公众号账号
- 小程序每次调用 wx.login() 后更新 SessionKey
- 需要查询微信账号信息时

### 2. Subscriber - 公众号订阅管理器

**文件**: `subscriber.go`

**职责**: 统一管理微信公众号的关注和取关事件业务逻辑。

**主要方法**:

- `Subscribe(ctx, appID, openID, unionID, nickname, avatar)` - 处理关注事件
- `Unsubscribe(ctx, appID, openID)` - 处理取关事件

**关注处理流程**:

1. 验证微信应用是否存在且已启用
2. 查找或创建微信公众号账号
3. 更新账号资料(昵称、头像、UnionID)
4. 标记账号为已关注状态
5. 若有 UnionID 且未绑定用户，尝试通过 UnionID 绑定到已有用户
6. 记录账号合并日志(如果发生合并)
7. 保存或更新账号

**取关处理流程**:

1. 查找微信公众号账号
2. 标记账号为未关注状态
3. 更新账号

**设计原则**:

- **内聚性**: 关注和取关是相对的操作，整合在同一个服务中
- **幂等性**: 多次调用相同参数不会产生副作用
- **自动合并**: 通过 UnionID 自动关联小程序和公众号账号
- **容错性**: 即使部分步骤失败也不影响用户体验
- **数据保留**: 取关不删除账号、不解绑用户，只标记状态

### 3. PhoneBinder - 手机号绑定器

**文件**: `wechat_phone_binder.go`

**职责**: 将手机号绑定到用户账号，并处理账号合并逻辑。

**主要方法**:
- `BindPhone(ctx, userID, phone)` - 绑定手机号

**绑定流程**:
1. 查找用户
2. 检查手机号是否已被其他用户使用
3. 绑定手机号到用户
4. 更新用户信息

**注意事项**:
- 如果手机号已被使用，当前实现会拒绝绑定
- 未来可以实现账号合并逻辑

## 设计原则

### 1. 平台区分

清晰区分小程序和公众号：

- 小程序有 SessionKey，公众号没有
- 公众号有关注状态，小程序没有
- 使用 `WxPlatform` 枚举区分平台类型

### 2. UnionID 合并

通过 UnionID 实现跨平台账号打通：

- 同一微信用户在小程序和公众号有不同的 OpenID
- 但拥有相同的 UnionID(需开通开放平台)
- 自动合并同一 UnionID 的账号到同一用户

### 3. 幂等性设计

所有创建操作都是"创建或更新"：

- 避免重复创建
- 保证接口幂等性
- 简化调用方逻辑

### 4. 自动化

自动创建关联资源：

- 创建微信账号时自动创建用户
- 记录账号合并日志
- 更新账号活跃时间

## 使用示例

### 小程序登录

```go
import "github.com/fangcun-mount/qs-server/internal/apiserver/application/user/wechat"

authenticator := wechat.NewAuthenticator(wxAccountRepo, mergeLogRepo, appRepo, userRepo)

loginResp, err := authenticator.Login(ctx, &wechat.LoginRequest{
    AppID:    "wx1234567890",
    Platform: "mini",
    Code:     "code_from_wx_login",
    OpenID:   "openid_from_code2session",
    UnionID:  &unionID,
    Nickname: "张三",
    Avatar:   "https://...",
})

if err != nil {
    // 处理错误
}

// loginResp.UserID - 用户ID
// loginResp.IsNewUser - 是否新用户
// loginResp.SessionKey - SessionKey
```

### 创建或更新微信账号

```go
wxAccountCreator := wechat.NewWechatAccountCreator(wxAccountRepo, userRepo, mergeLogRepo, appRepo)

// 小程序账号
wxAccount, err := wxAccountCreator.CreateOrUpdateMiniProgramAccount(
    ctx,
    "wx1234567890",
    "openid_xxx",
    &unionID,
    "张三",
    "https://avatar_url",
    "session_key_xxx",
)

// 公众号账号
oaAccount, err := wxAccountCreator.CreateOrUpdateOfficialAccount(
    ctx,
    "wx0987654321",
    "openid_yyy",
    &unionID,
    "张三",
    "https://avatar_url",
)
```

### 处理公众号关注

```go
follower := wechat.NewFollower(wxAccountRepo, mergeLogRepo, appRepo)

// 关注事件
err := follower.HandleSubscribe(
    ctx,
    "wx0987654321",
    "openid_yyy",
    &unionID,
    "张三",
    "https://avatar_url",
)

// 取关事件
err := follower.HandleUnsubscribe(ctx, "wx0987654321", "openid_yyy")
```

### 绑定手机号

```go
phoneBinder := wechat.NewPhoneBinder(userRepo, wxAccountRepo, mergeLogRepo)

err := phoneBinder.BindPhone(ctx, user.NewUserID(123), "13800138000")
```

### 更新 SessionKey

```go
accountManager := wechat.NewAccountManager(wxAccountRepo, appRepo)

err := accountManager.UpdateSessionKey(ctx, "wx1234567890", "openid_xxx", "new_session_key")
```

## 集成到 gRPC

在 gRPC 服务中的使用：

```go
import (
    wechatApp "github.com/fangcun-mount/qs-server/internal/apiserver/application/user/wechat"
)

type UserService struct {
    wxAccountManager *wechatApp.AccountManager
    // 其他服务...
}

func (s *UserService) CreateOrUpdateMiniProgramAccount(
    ctx context.Context,
    req *pb.CreateOrUpdateMiniProgramAccountRequest,
) (*pb.WechatAccountResponse, error) {
    wxAccount, err := s.wxAccountManager.CreateOrUpdateMiniProgramAccount(
        ctx,
        req.AppId,
        req.OpenId,
        &req.UnionId,
        req.Nickname,
        req.Avatar,
        req.SessionKey,
    )
    // ...
}
```

## 与 Domain 的关系

```text
application/user/wechat/
├── wechat_account_creator.go  → domain/user/account/wechat_account.go
├── wechat_authenticator.go    → domain/user/account/wechat_account.go
├── wechat_follower.go          → domain/user/account/wechat_account.go
├── wechat_phone_binder.go      → domain/user/user.go
└── wechat_session_updater.go   → domain/user/account/wechat_account.go
```

应用层调用领域层的工厂方法和业务方法，协调多个聚合根的交互。

## 相关领域对象

- `domain/user/account.WechatAccount` - 微信账号聚合根
- `domain/user/account.MergeLog` - 账号合并日志
- `domain/wechat.App` - 微信应用配置
- `domain/user.User` - 用户聚合根
