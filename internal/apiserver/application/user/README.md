# User Application Layer - 用户应用层

## 目录说明

本目录包含用户模块的所有应用服务，遵循**单一职责原则**和**分离关注点原则**。每个服务专注于一个特定的业务能力。

## 目录结构

```
application/user/
├── README.md                   # 本文档
├── creator.go                  # 用户创建服务
├── editor.go                   # 用户编辑服务
├── queryer.go                  # 用户查询服务
├── activator.go                # 用户状态管理服务
├── password_changer.go         # 密码管理服务
├── role/                       # 角色服务子模块
│   ├── testee_creator.go       # 受试者角色管理
│   ├── writer_creator.go       # 填写人角色管理
│   └── auditor_creator.go      # 审核员角色管理（内部）
└── wechat/                     # 微信服务子模块
    ├── wechat_account_creator.go     # 微信账号创建
    ├── wechat_authenticator.go       # 微信登录认证
    ├── wechat_follower.go            # 公众号关注/取关
    ├── wechat_phone_binder.go        # 手机号绑定
    └── wechat_session_updater.go     # SessionKey更新
```

## 服务分类

### 1. 基础用户服务 (package user)

#### UserCreator (creator.go)
- **职责**: 用户创建
- **方法**: `CreateUser()` - 创建新用户并进行唯一性检查

#### UserEditor (editor.go)
- **职责**: 用户信息编辑
- **方法**: 
  - `UpdateBasicInfo()` - 更新用户基本信息
  - `UpdateAvatar()` - 更新用户头像

#### UserQueryer (queryer.go)
- **职责**: 用户查询
- **方法**:
  - `GetUser()` - 根据ID获取用户
  - `GetUserByUsername()` - 根据用户名获取用户
  - `ListUsers()` - 获取用户列表

#### UserActivator (activator.go)
- **职责**: 用户状态管理
- **方法**:
  - `ActivateUser()` - 激活用户
  - `BlockUser()` - 封禁用户
  - `DeactivateUser()` - 禁用用户

#### PasswordChanger (password_changer.go)
- **职责**: 密码管理
- **方法**: `ChangePassword()` - 修改用户密码

---

### 2. 微信账号服务 (package wechat)

位于 `wechat/` 子目录，处理微信小程序和公众号账号管理相关功能。

> **注意**: 微信登录认证逻辑 (`WechatAuthenticator`) 已移至 `application/auth/` 目录，因为认证是独立的关注点。

#### WechatAccountCreator (account_creator.go)
- **职责**: 微信账号创建和更新
- **方法**:
  - `CreateOrUpdateMiniProgramAccount()` - 创建或更新小程序账号
  - `CreateOrUpdateOfficialAccount()` - 创建或更新公众号账号

#### Follower (follower.go)
- **职责**: 公众号关注/取关处理
- **方法**:
  - `HandleSubscribe()` - 处理公众号关注事件
  - `HandleUnsubscribe()` - 处理公众号取关事件

#### PhoneBinder (phone_binder.go)
- **职责**: 手机号绑定
- **方法**: `BindPhone()` - 绑定手机号到用户账号

#### SessionUpdater (session_updater.go)
- **职责**: SessionKey更新
- **方法**: `UpdateSessionKey()` - 更新小程序SessionKey

---

### 3. 角色服务 (package role)

位于 `role/` 子目录，处理用户角色（受试者、填写人、审核员）管理。

#### TesteeCreator (testee_creator.go)
- **职责**: 受试者角色管理
- **方法**:
  - `CreateTestee()` - 创建受试者
  - `UpdateTestee()` - 更新受试者信息
  - `GetTesteeByUserID()` - 获取受试者
  - `TesteeExists()` - 检查受试者是否存在

#### WriterCreator (writer_creator.go)
- **职责**: 填写人角色管理
- **方法**:
  - `CreateWriter()` - 创建填写人
  - `UpdateWriter()` - 更新填写人信息
  - `GetWriterByUserID()` - 获取填写人

#### AuditorCreator (auditor_creator.go)
- **职责**: 审核员角色管理 (仅供内部使用)
- **方法**:
  - `CreateAuditor()` - 创建审核员
  - `UpdateAuditorInfo()` - 更新审核员信息
  - `UpdateAuditorStatus()` - 更新审核员状态
  - `GetAuditorByUserID()` - 获取审核员
  - `CanAudit()` - 检查是否可以审核

---

## 设计原则

### 1. 单一职责原则 (SRP)
每个服务类只负责一个特定的业务能力：
- `UserCreator` 只负责创建用户
- `UserEditor` 只负责编辑用户
- `PasswordChanger` 只负责密码管理

### 2. 分离关注点 (Separation of Concerns)
不同的业务关注点被清晰地分离到不同的服务中：
- **CQRS模式**: `UserCreator`/`UserEditor` (命令) vs `UserQueryer` (查询)
- **业务聚合**: 基础用户、微信账号、角色管理分别聚合

### 3. 依赖注入
所有服务通过构造函数注入依赖，便于测试和解耦。

### 4. 领域驱动设计 (DDD)
应用层协调领域层的聚合根和实体，不包含业务逻辑。

---

## 使用示例

### 创建用户并设置为受试者

```go
// 1. 创建用户
userCreator := user.NewUserCreator(userRepo)
u, err := userCreator.CreateUser(ctx, "zhangsan", "password", "张三", "zhangsan@example.com", "13800138000", "")

// 2. 创建受试者角色
testeeCreator := role.NewTesteeCreator(testeeRepo, userRepo)
testee, err := testeeCreator.CreateTestee(ctx, u.ID(), "张三", 1, &birthday)
```

### 微信小程序登录

```go
// 1. 创建或更新微信账号
wxAccountCreator := wechat.NewWechatAccountCreator(wxAccountRepo, userRepo, mergeLogRepo, appRepo)
wxAccount, err := wxAccountCreator.CreateOrUpdateMiniProgramAccount(ctx, "wx123", "openid123", nil, "张三", "avatar_url", "session_key")

// 2. 使用认证器登录 (注意：认证器在 auth 包中)
authenticator := auth.NewWechatAuthenticator(wxAccountRepo, mergeLogRepo, appRepo, userRepo)
loginResp, err := authenticator.LoginWithMiniProgram(ctx, loginRequest)
```

---

## gRPC 集成

在 gRPC 服务层，这些应用服务被组合使用：

```go
// interface/grpc/service/user.go
import (
    userApp "github.com/fangcun-mount/qs-server/internal/apiserver/application/user"
    roleApp "github.com/fangcun-mount/qs-server/internal/apiserver/application/user/role"
    wechatApp "github.com/fangcun-mount/qs-server/internal/apiserver/application/user/wechat"
)

type UserService struct {
    userCreator      *userApp.UserCreator
    userEditor       *userApp.UserEditor
    wxAccountCreator *wechatApp.WechatAccountCreator
    testeeCreator    *roleApp.TesteeCreator
    writerCreator    *roleApp.WriterCreator
}
```

**注意**: `AuditorCreator` 不在 gRPC 服务中暴露，因为审核员是内部员工角色。

---

## 文件命名规范

- 使用下划线命名: `wechat_account_creator.go`
- 服务名称体现职责: `*Creator`, `*Editor`, `*Queryer`, `*Activator`, `*Changer`, `*Updater`, `*Binder`
- 文件名与服务名匹配: `TesteeCreator` → `testee_creator.go`

---

## 依赖关系

```text
Application Layer
├── user/                    (基础用户服务)
├── user/role/              (角色服务)
└── user/wechat/            (微信服务)
        ↓ 依赖
Domain Layer
├── domain/user/            (用户聚合根)
├── domain/user/account/    (账号子域)
└── domain/user/role/       (角色子域)
        ↓ 定义接口
Infrastructure Layer
└── infra/mysql/            (MySQL实现)
```

应用层服务通过 Repository 接口与基础设施层解耦。
