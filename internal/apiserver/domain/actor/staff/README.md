# Staff 子域设计说明

## 概述

Staff（员工）是问卷&量表BC中的后台工作人员聚合根，代表 IAM.User 在本系统的业务视图投影。Staff 存储业务角色和机构隔离信息，采用充血模型设计，将领域服务从聚合根中提取。

## 核心组件

### 1. 聚合根：Staff

**职责**：维护员工的核心状态和不变量

**设计原则**：

- ✅ 是 IAM.User 的业务视图投影，不是完整用户实体
- ✅ 存储本BC的业务角色（IAM不关心的领域概念）
- ✅ 支持多租户隔离（同一IAM用户在不同机构有不同角色）
- ✅ 冗余缓存常用字段（name、email、phone）减少RPC
- ✅ 不存储IAM认证信息（密码、token）

**核心字段**：
```go
type Staff struct {
    // 核心标识
    id, orgID, iamUserID
    
    // 业务角色（本BC核心概念）
    roles []Role
    
    // 冗余缓存（从IAM同步）
    name, email, phone
    
    // 激活状态
    isActive
}
```

**行为方法**：

- `AssignRole()`, `RemoveRole()` - 角色管理
- `HasRole()`, `HasAnyRole()` - 角色检查
- `UpdateContactInfo()` - 更新联系方式
- `Activate()`, `Deactivate()` - 激活/停用
- `CanManageScales()`, `CanEvaluate()` 等 - 权限检查便捷方法

---

### 2. 领域服务

采用职责分离设计，将不同关注点独立为领域服务：

#### 2.1 Validator - 验证器

**职责**：验证 Staff 的字段合法性

**设计原则**：

- ✅ 按字段维度提供验证方法
- ✅ 可灵活组合，扩展性好
- ✅ 验证规则集中管理

**接口方法**：
```go
ValidateOrgID(orgID int64) error
ValidateIAMUserID(iamUserID int64) error
ValidateName(name string, required bool) error
ValidateEmail(email string) error
ValidatePhone(phone string) error
ValidateRole(role Role) error
ValidateRoles(roles []Role) error
```

**验证规则**：

- 机构ID和IAM用户ID必须为正数
- 姓名最长100字符
- 邮箱格式验证（包含@和.）
- 手机号长度7-20字符，只允许数字和特定符号
- 角色必须是预定义的枚举值
- 角色列表最多20个

---

#### 2.2 RoleManager - 角色管理器

**职责**：管理 Staff 的业务角色分配和权限检查

**核心逻辑**：

- 角色分配前验证合法性
- 只能给激活状态的员工分配角色
- 支持批量操作和角色替换
- 提供统一的权限验证入口

**接口方法**：
```go
AssignRole(staff, role) error
RemoveRole(staff, role) error
AssignRoles(staff, roles) error
ReplaceRoles(staff, roles) error
ClearRoles(staff) error
ValidatePermission(staff, requiredRoles...) error
```

**业务规则**：

1. 只有激活的员工可以分配角色
2. 移除角色时必须已拥有该角色
3. 批量替换时自动去重
4. 权限验证同时检查激活状态和角色

**使用场景**：
```go
roleManager := NewRoleManager(validator)

// 分配角色
err := roleManager.AssignRole(staff, RoleScaleAdmin)

// 验证权限
err := roleManager.ValidatePermission(staff, RoleScaleAdmin, RoleEvaluator)
if err != nil {
    // 无权限
}
```

---

#### 2.3 Editor - 编辑器

**职责**：管理 Staff 信息的变更

**设计原则**：

- 所有更新操作经过验证
- 关键操作可触发领域事件（如停用员工）
- 保证操作的幂等性

**接口方法**：
```go
UpdateContactInfo(staff, email, phone) error
UpdateName(staff, name) error
Activate(staff) error
Deactivate(staff, reason) error
```

**业务规则**：

- 停用员工时自动清空所有角色
- 激活/停用操作幂等
- 联系方式更新需验证格式

**重要行为**：
```go
func (e *editor) Deactivate(staff *Staff, reason string) error {
    // 业务规则：停用时清空角色
    staff.roles = make([]Role, 0)
    staff.Deactivate()
    
    // TODO: 发布领域事件
    // events.Publish(NewStaffDeactivatedEvent(staff.ID(), reason))
}
```

---

#### 2.4 IAMSynchronizer - IAM同步器

**职责**：从 IAM 系统同步员工信息到本地

**触发时机**：

- 员工登录时同步最新信息
- IAM 用户信息变更时（通过事件）
- 定期批量同步（修复数据不一致）

**接口方法**：
```go
SyncBasicInfo(ctx, staff, name, email, phone) error
ValidateIAMBinding(ctx, staff) error
```

**设计考虑**：

- 冗余字段（name、email、phone）来自IAM
- 同步时验证参数合法性
- 支持验证IAM绑定有效性（IAM用户是否还存在）
- 同步操作不影响业务角色

**与 Factory.SyncFromIAM 的区别**：

- Factory.SyncFromIAM 已标记为过时（Deprecated）
- IAMSynchronizer 更明确职责，不依赖仓储更新
- 应用层可以选择是否持久化同步结果

---

### 3. 值对象

#### Role - 角色枚举
```go
const (
    RoleScaleAdmin       Role = "scale_admin"        // 量表管理员
    RoleEvaluator        Role = "evaluator"          // 评估人员
    RoleScreeningOwner   Role = "screening_owner"    // 筛查项目负责人
    RoleReportAuditor    Role = "report_auditor"     // 报告审核员
)
```

**特点**：

- 字符串类型，便于存储和扩展
- 预定义枚举，通过 Validator 验证
- 每个角色对应特定业务能力

---

### 4. 工厂（Factory）

**职责**：创建或获取 Staff 实例，处理幂等性

**方法**：

- `GetOrCreateByIAMUser` - 通过 IAM User 获取或创建
- `SyncFromIAM` - 从IAM同步信息（已过时，推荐用 IAMSynchronizer）

**特点**：

- 内置参数验证（依赖 Validator）
- 幂等操作：如果已存在则返回，不存在则创建
- 新创建的员工默认激活，无角色

---

## 依赖关系

```
┌─────────────────────────────────────────────────┐
│              Application Layer                  │
│         (StaffAppService)                       │
└────────────┬────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────┐
│              Domain Layer                       │
│                                                 │
│  ┌──────────┐      ┌──────────────────────┐    │
│  │  Staff   │◄─────┤  Domain Services:    │    │
│  │(Aggregate│      │  - Validator         │    │
│  │  Root)   │      │  - RoleManager       │    │
│  └──────────┘      │  - Editor            │    │
│                    │  - IAMSynchronizer   │    │
│                    │  - Factory           │    │
│                    └──────────────────────┘    │
│                                                 │
│  ┌──────────────────────────────────────┐      │
│  │  Value Objects:                      │      │
│  │  - Role                              │      │
│  └──────────────────────────────────────┘      │
│                                                 │
│  ┌──────────────────────────────────────┐      │
│  │  Interfaces:                         │      │
│  │  - Repository                        │      │
│  └──────────────────────────────────────┘      │
└─────────────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────┐
│         Infrastructure Layer                    │
│       (MongoDB Repository)                      │
└─────────────────────────────────────────────────┘
```

---

## 使用示例

### 创建员工

```go
validator := staff.NewValidator()
factory := staff.NewFactory(repo, validator)

// 通过 IAM User 获取或创建
s, err := factory.GetOrCreateByIAMUser(
    ctx,
    orgID,
    iamUserID,
    "张三",
)
```

### 角色管理

```go
validator := staff.NewValidator()
roleManager := staff.NewRoleManager(validator)

// 分配角色
err := roleManager.AssignRole(s, staff.RoleScaleAdmin)
err = roleManager.AssignRole(s, staff.RoleEvaluator)

// 批量替换角色
newRoles := []staff.Role{staff.RoleScaleAdmin, staff.RoleReportAuditor}
err := roleManager.ReplaceRoles(s, newRoles)

// 验证权限
err := roleManager.ValidatePermission(s, staff.RoleScaleAdmin)
if err != nil {
    return errors.New("无权限管理量表")
}

// 保存
repo.Update(ctx, s)
```

### 编辑员工

```go
validator := staff.NewValidator()
editor := staff.NewEditor(validator)

// 更新联系方式
err := editor.UpdateContactInfo(s, "zhangsan@example.com", "13800138000")

// 更新姓名
err := editor.UpdateName(s, "张三三")

// 停用员工（会清空角色）
err := editor.Deactivate(s, "员工离职")

// 保存
repo.Update(ctx, s)
```

### IAM 同步

```go
validator := staff.NewValidator()
synchronizer := staff.NewIAMSynchronizer(repo, validator)

// 同步基本信息
err := synchronizer.SyncBasicInfo(
    ctx,
    s,
    "新姓名",
    "newemail@example.com",
    "13900139000",
)

// 验证 IAM 绑定
err := synchronizer.ValidateIAMBinding(ctx, s)
if err != nil {
    // IAM 用户不存在或已删除
}
```

---

## 设计优势

### 1. 清晰的职责分离

- **RoleManager**：专注角色和权限管理
- **Editor**：专注基本信息变更
- **IAMSynchronizer**：专注外部系统同步
- **Validator**：专注字段验证

### 2. 灵活的角色系统

- 基于枚举的角色定义
- 支持多角色组合
- 统一的权限验证入口
- 易于扩展新角色

### 3. IAM 集成设计

- 松耦合：通过 iamUserID 关联
- 冗余缓存：减少 RPC 调用
- 同步机制：保持数据一致性
- 验证机制：确保绑定有效性

### 4. 安全性考虑

- 停用员工自动清空角色
- 角色分配前验证激活状态
- 权限检查同时验证激活状态和角色
- 所有变更经过验证器

---

## 与 Testee 的对比

| 维度 | Testee | Staff |
|------|--------|-------|
| **身份来源** | 可选绑定 IAM（User/Child） | 必须绑定 IAM User |
| **核心数据** | 测评统计、标签、关注度 | 业务角色 |
| **主要操作** | 标签管理、统计更新 | 角色管理、权限检查 |
| **领域服务** | Validator, Binder, Editor, StatsUpdater | Validator, RoleManager, Editor, IAMSynchronizer |
| **多租户** | orgID 隔离 | orgID 隔离（同一IAM用户可在多机构） |

---

## 待实现

- [ ] Repository 的 MongoDB 实现
- [ ] 领域事件发布（员工停用、角色变更等）
- [ ] IAMSynchronizer 的 ValidateIAMBinding 调用 IAM 服务
- [ ] 权限检查的缓存机制（Redis）
- [ ] 应用服务层的 DTO 和组装
- [ ] 角色的细粒度权限配置（如：只能管理特定量表）
