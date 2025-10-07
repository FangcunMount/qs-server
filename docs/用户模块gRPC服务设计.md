# 用户模块 gRPC 服务设计

## 1. 概述

### 1.1 设计目标

用户模块提供统一的 gRPC 服务,对外暴露用户、微信账号、受试者(Testee)、填写人(Writer)等所有用户相关功能。

**重要说明**: Auditor(审核员)是内部员工角色,不对外部服务(如 collection-server)开放,仅供内部管理后台使用。

### 1.2 架构定位

- **apiserver**: 基础服务层,通过 gRPC 对外提供服务
- **collection-server**: 微信小程序交互层,作为 gRPC 客户端调用 apiserver

```
collection-server (微信小程序) 
    ↓ gRPC 调用
apiserver (UserService)
    ↓ 应用服务
domain + infra (领域模型 + 基础设施)
```

### 1.3 服务范围

UserService 包含以下子服务:

| 子服务 | RPC 方法数 | 说明 |
|--------|-----------|------|
| User | 3 | 基础用户管理 |
| WechatAccount | 3 | 微信账号管理 |
| Testee | 4 | 受试者角色管理 |
| Writer | 3 | 填写人角色管理 |
| **总计** | **13** | **统一的用户模块服务** |

> **注意**: Auditor(审核员)不在此列表中,因为它仅供内部使用,不通过 gRPC 暴露给外部服务。

## 2. Proto 定义

### 2.1 服务结构

所有用户相关的 RPC 方法统一定义在 `user.proto` 中的 `UserService`:

```protobuf
syntax = "proto3";

package user;

option go_package = "github.com/fangcun-mount/qs-server/internal/apiserver/interface/grpc/proto/user";

service UserService {
  // ========== 用户服务 ==========
  rpc CreateUser(CreateUserRequest) returns (CreateUserResponse);
  rpc UpdateUserBasicInfo(UpdateUserBasicInfoRequest) returns (UpdateUserBasicInfoResponse);
  rpc GetUser(GetUserRequest) returns (GetUserResponse);

  // ========== 微信账号服务 ==========
  rpc CreateOrUpdateMiniProgramAccount(CreateOrUpdateMiniProgramAccountRequest) returns (WechatAccountResponse);
  rpc CreateOrUpdateOfficialAccount(CreateOrUpdateOfficialAccountRequest) returns (WechatAccountResponse);
  rpc GetWechatAccountByOpenID(GetWechatAccountByOpenIDRequest) returns (WechatAccountResponse);

  // ========== 受试者服务 ==========
  rpc CreateTestee(CreateTesteeRequest) returns (TesteeResponse);
  rpc UpdateTestee(UpdateTesteeRequest) returns (TesteeResponse);
  rpc GetTestee(GetTesteeRequest) returns (TesteeResponse);
  rpc TesteeExists(TesteeExistsRequest) returns (TesteeExistsResponse);

  // ========== 填写人服务 ==========
  rpc CreateWriter(CreateWriterRequest) returns (WriterResponse);
  rpc UpdateWriter(UpdateWriterRequest) returns (WriterResponse);
  rpc GetWriter(GetWriterRequest) returns (WriterResponse);
}
```

### 2.2 消息定义

详细消息定义参见 `internal/apiserver/interface/grpc/proto/user/user.proto`

## 3. 服务实现

### 3.1 UserService 结构

```go
type UserService struct {
    pb.UnimplementedUserServiceServer

    // 基础用户服务
    userCreator *userApp.UserCreator
    userEditor  *userApp.UserEditor

    // 微信账号服务
    wxAccountCreator *wechat.WechatAccountCreator

    // 角色服务
    testeeCreator *roleApp.TesteeCreator
    writerCreator *roleApp.WriterCreator
    
    // 注意: Auditor 不在此处,因为不对外暴露
}
```

### 3.2 依赖注入

```go
func NewUserService(
    userCreator *userApp.UserCreator,
    userEditor *userApp.UserEditor,
    wxAccountCreator *wechat.WechatAccountCreator,
    testeeCreator *roleApp.TesteeCreator,
    writerCreator *roleApp.WriterCreator,
) *UserService
```

### 3.3 服务注册

在 `internal/apiserver/grpc_registry.go` 中注册:

```go
func (r *GRPCRegistry) registerUserService() {
    userService := service.NewUserService(
        r.container.GetUserCreator(),
        r.container.GetUserEditor(),
        r.container.GetWechatAccountCreator(),
        r.container.GetTesteeCreator(),
        r.container.GetWriterCreator(),
    )
    userService.RegisterService(r.server)
}
```

## 4. 使用示例

### 4.1 collection-server 调用示例

#### 小程序用户登录流程

```go
// 1. 创建或更新小程序账号
wxAccountResp, err := userClient.CreateOrUpdateMiniProgramAccount(ctx, &pb.CreateOrUpdateMiniProgramAccountRequest{
    AppId:      "wx1234567890",
    OpenId:     "oABC123...",
    UnionId:    "uXYZ789...",
    Nickname:   "张三",
    Avatar:     "https://...",
    SessionKey: "session_key_xxx",
})

// 2. 检查用户是否已创建受试者角色
existsResp, err := userClient.TesteeExists(ctx, &pb.TesteeExistsRequest{
    UserId: wxAccountResp.UserId,
})

// 3. 如果不存在,创建受试者
if !existsResp.Exists {
    testeeResp, err := userClient.CreateTestee(ctx, &pb.CreateTesteeRequest{
        UserId: wxAccountResp.UserId,
        Name:   "张三",
        Sex:    1,
    })
}

// 4. 获取受试者信息
testeeResp, err := userClient.GetTestee(ctx, &pb.GetTesteeRequest{
    UserId: wxAccountResp.UserId,
})
```

#### 填写人管理

```go
// 创建填写人
writerResp, err := userClient.CreateWriter(ctx, &pb.CreateWriterRequest{
    UserId: userId,
    Name:   "李四",
})

// 更新填写人
writerResp, err := userClient.UpdateWriter(ctx, &pb.UpdateWriterRequest{
    UserId: userId,
    Name:   "李四(更新)",
})

// 获取填写人
writerResp, err := userClient.GetWriter(ctx, &pb.GetWriterRequest{
    UserId: userId,
})
```

## 5. 设计决策

### 5.1 为什么统一为 UserService?

**优点**:
1. **简化客户端**: collection-server 只需建立 1 个 gRPC 连接,而不是多个
2. **概念统一**: 对外是"用户模块",内部才区分用户、账号、角色
3. **降低复杂度**: 减少服务发现、连接管理的复杂性
4. **更好的事务性**: 跨子服务的操作可以在同一个 service 中协调

**缺点**:
1. 服务较大,但通过清晰的分组注释可以缓解
2. 单个 proto 文件较长,但 13 个方法仍然可控

### 5.2 为什么 Auditor 不在 gRPC 中?

**原因**:
1. **访问控制**: Auditor 是内部员工,不应该暴露给外部服务
2. **安全性**: 审核员管理属于敏感操作,应该由独立的管理后台处理
3. **职责分离**: collection-server 处理C端用户,不需要B端员工管理能力

**实现方式**:
- Auditor 的应用服务(`AuditorCreator`)仍然存在于 `application/role/` 目录
- 内部管理后台可以直接调用应用服务,无需走 gRPC
- 保持了清晰的 C 端/B 端边界

### 5.3 与替代方案的对比

#### 方案 A: 分离的服务 (否决)

```
UserService (3 RPCs)
WechatAccountService (3 RPCs)  
TesteeService (4 RPCs)
WriterService (3 RPCs)
```

缺点:
- 客户端需要管理 4 个连接
- 跨服务调用复杂
- 不符合"用户模块"的统一概念

#### 方案 B: 扁平化单服务 (否决)

所有方法没有分组,直接放在一起。

缺点:
- 缺乏组织结构
- 难以理解服务边界
- 不利于维护

#### 方案 C: 统一 UserService + 分组注释 (采用)

将所有用户相关功能放在一个服务中,通过注释清晰分组。

优点:
- 对外统一,对内清晰
- 易于使用和理解
- 便于扩展

## 6. 后续工作

### 6.1 需要实现的部分

1. **Repository 接口**
   - TesteeRepository
   - WriterRepository
   - 实现查询方法支持 GetUser, GetWechatAccountByOpenID

2. **基础设施层**
   - infra/mysql/testee/ 
   - infra/mysql/writer/
   - 数据库表设计和 SQL

3. **容器注入**
   - 更新 Container 提供所有依赖
   - 注册到 grpc_registry.go

4. **客户端实现**
   - collection-server 中创建 gRPC 客户端
   - 实现具体的业务调用逻辑

### 6.2 测试计划

1. 单元测试: 每个 RPC 方法的测试
2. 集成测试: 完整的用户注册登录流程
3. 压力测试: gRPC 服务性能测试

## 7. 总结

用户模块采用统一的 UserService 设计,包含 13 个 RPC 方法,覆盖用户、微信账号、受试者、填写人四大功能。Auditor 作为内部员工角色不对外暴露,保持了清晰的服务边界和访问控制。这种设计既满足了微信小程序的需求,又保证了系统的安全性和可维护性。
