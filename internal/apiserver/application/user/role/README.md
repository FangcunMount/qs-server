# Role Services - 角色服务

## 概述

本目录包含用户角色相关的应用服务，负责管理受试者(Testee)、填写人(Writer)、审核员(Auditor)三种角色。

## 服务列表

### 1. TesteeCreator - 受试者创建器

**文件**: `testee_creator.go`

**职责**: 管理受试者(被测者/考生)角色的创建、更新和查询。

**主要方法**:
- `CreateTestee(ctx, userID, name, sex, birthday)` - 创建受试者
- `UpdateTestee(ctx, userID, name, sex, birthday)` - 更新受试者信息
- `GetTesteeByUserID(ctx, userID)` - 根据用户ID获取受试者
- `TesteeExists(ctx, userID)` - 检查受试者是否存在

**使用场景**:
- 用户首次参与心理测评时创建受试者角色
- 更新受试者的个人信息(姓名、性别、生日)
- 在答卷提交前检查用户是否已注册为受试者

### 2. WriterCreator - 填写人创建器

**文件**: `writer_creator.go`

**职责**: 管理填写人角色的创建、更新和查询。

**主要方法**:
- `CreateWriter(ctx, userID, name)` - 创建填写人
- `UpdateWriter(ctx, userID, name)` - 更新填写人信息
- `GetWriterByUserID(ctx, userID)` - 根据用户ID获取填写人

**使用场景**:
- 用户首次填写问卷时创建填写人角色
- 更新填写人的姓名信息
- 记录问卷填写人信息

### 3. AuditorCreator - 审核员创建器

**文件**: `auditor_creator.go`

**职责**: 管理审核员(内部员工)角色的创建、更新、状态管理和权限检查。

**主要方法**:
- `CreateAuditor(ctx, userID, name, employeeID, department, position, hiredAt)` - 创建审核员
- `UpdateAuditorInfo(ctx, userID, name, department, position)` - 更新审核员信息
- `UpdateAuditorStatus(ctx, userID, status)` - 更新审核员状态
- `GetAuditorByUserID(ctx, userID)` - 根据用户ID获取审核员
- `CanAudit(ctx, userID)` - 检查是否有审核权限

**审核员状态**:
- `OnDuty` (1) - 在职
- `OnLeave` (2) - 请假
- `Suspended` (3) - 停职
- `Resigned` (4) - 离职

**使用场景**:
- 内部管理后台创建/管理审核员
- 检查审核员是否有权限审核问卷
- 管理审核员的工作状态

**注意**: 
- ⚠️ **审核员服务仅供内部使用，不对外部 gRPC 暴露**
- 审核员是B端员工角色，与C端用户角色(Testee/Writer)分离
- 只能通过内部管理后台直接调用应用服务，不走 collection-server

## 设计原则

### 1. 角色独立性
每个角色有独立的创建器，互不干扰：
- 一个用户可以同时拥有多个角色
- 角色之间没有强制关联
- 角色创建失败不影响用户账号

### 2. 领域驱动
角色服务协调领域对象，不包含业务逻辑：
- 业务规则在领域对象中(domain/user/role/)
- 应用服务负责编排和持久化
- 遵循六边形架构的应用层定位

### 3. 依赖注入
所有服务通过构造函数注入 Repository：
```go
func NewTesteeCreator(
    testeeRepo port.TesteeRepository,
    userRepo port.UserRepository,
) *TesteeCreator
```

## 使用示例

### 创建受试者

```go
import "github.com/FangcunMount/qs-server/internal/apiserver/application/user/role"

testeeCreator := role.NewTesteeCreator(testeeRepo, userRepo)

birthday := time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)
testee, err := testeeCreator.CreateTestee(
    ctx,
    user.NewUserID(123),
    "张三",
    1, // 男性
    &birthday,
)
```

### 创建填写人

```go
writerCreator := role.NewWriterCreator(writerRepo, userRepo)

writer, err := writerCreator.CreateWriter(
    ctx,
    user.NewUserID(123),
    "李四",
)
```

### 创建审核员(内部使用)

```go
auditorCreator := role.NewAuditorCreator(auditorRepo, userRepo)

hiredAt := time.Now()
auditor, err := auditorCreator.CreateAuditor(
    ctx,
    user.NewUserID(456),
    "王经理",
    "EMP001",
    "质量管理部",
    "高级审核员",
    &hiredAt,
)

// 检查审核权限
canAudit, err := auditorCreator.CanAudit(ctx, user.NewUserID(456))
```

## 集成到 gRPC

在 gRPC 服务中的使用：

```go
import (
    roleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/user/role"
)

type UserService struct {
    testeeCreator *roleApp.TesteeCreator
    writerCreator *roleApp.WriterCreator
    // 注意: auditorCreator 不在 gRPC 服务中
}
```

## 与 Domain 的关系

```text
application/user/role/
├── testee_creator.go    → domain/user/role/testee.go
├── writer_creator.go    → domain/user/role/writer.go
└── auditor_creator.go   → domain/user/role/auditor.go
```

应用层调用领域层的工厂方法和业务方法，不直接操作数据结构。
