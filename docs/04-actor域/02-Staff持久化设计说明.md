# Staff 持久化设计说明

## 问题

Staff 是否需要存储到数据库？它和 IAM 中的 User/Account 是什么关系？

## 回答

**Staff 需要持久化，但它是轻量级的业务视图投影，而非完整的用户实体。**

## 设计原则

### Staff 与 IAM.User 的关系

```text
IAM BC (统一身份认证)           问卷&量表 BC (业务领域)
┌──────────────────┐            ┌──────────────────────┐
│ User/Account     │            │ Staff                │
│ - UserID         │ ◄──────────┤ - StaffID            │
│ - Username       │  外键关联  │ - IAMUserID (FK)     │
│ - Password       │            │ - OrgID              │
│ - Phone          │            │ - Roles (业务角色)   │
│ - Email          │            │ - Name (缓存)        │
│ - Roles (通用)   │            │ - Email (缓存)       │
└──────────────────┘            └──────────────────────┘
```

### 职责划分

| 方面 | IAM.User/Account | Staff |
|------|------------------|-------|
| **认证** | ✅ 负责登录、密码、Token | ❌ 不管认证 |
| **通用权限** | ✅ 能否访问某个模块 | ❌ 不管粗粒度权限 |
| **业务角色** | ❌ 不管具体业务 | ✅ role:qs:content_manager, role:qs:evaluator 等 |
| **多租户** | ✅ 跨机构的统一身份 | ✅ 同一人在不同机构的不同角色 |
| **业务语义** | ❌ 只是技术账号 | ✅ 领域模型的一部分 |

## 为什么要持久化 Staff

### 1. 存储业务角色（核心原因）

**问题**：IAM 只管通用权限（如 "能否访问量表模块"），不管业务细节。

**业务角色示例（已迁移为统一权限中心标识）**：

- `qs:admin`：QS 管理员，拥有系统级管理权限
- `qs:content_manager`：内容管理员，能管理问卷与量表
- `qs:evaluator`：评估员，能执行测评相关操作（只读/重试等）
- `qs:staff`：普通员工，仅具备查看受试者权限

这些角色由权限中心下发并以字符串形式存储在 `roles` 字段中，属于本 BC 的领域概念，需要持久化。

```go
// 业务逻辑判断示例
func (s *Staff) CanManageScales() bool {
    return s.HasAnyRole(RoleContentManager, RoleQSAdmin)
}

func (s *ScaleService) UpdateScale(ctx context.Context, staffID StaffID, scale *Scale) error {
    staff := s.staffRepo.FindByID(ctx, staffID)
    if !staff.CanManageScales() {
        return errors.New("permission denied")
    }
    // ...
}
```

### 2. 多租户隔离

**场景**：同一个 IAM.User 可能在不同机构有不同身份。

| IAM.UserID | OrgID | StaffID | Roles |
|------------|-------|---------|-------|
| 1001 | 医院A | S-001 | qs:evaluator |
| 1001 | 医院B | S-002 | qs:content_manager, qs:evaluator |
| 1002 | 医院A | S-003 | qs:admin |

**实现**：

```go
// 查询时需要 OrgID + IAMUserID 才能定位唯一的 Staff
staff, err := staffRepo.FindByIAMUser(ctx, orgID, iamUserID)
```

### 3. 审计追溯

**需求**：操作记录需要记录"谁操作的"。

**方案对比**：

| 方案 | 优点 | 缺点 |
|------|------|------|
| 直接用 `IAMUserID` | 简单 | 无业务语义；IAM 用户删除后难以追溯 |
| 用 `StaffID` | 业务语义清晰；历史记录稳定 | 需要维护 Staff 表 |

**示例**：

```go
type Testee struct {
    // ...
    createdBy int64 // 操作员工的 IAM UserID
    // 或者更好的方式：
    createdBy StaffID // 业务语义更清晰
}
```

### 4. 性能优化（缓存）

**问题**：如果每次都调用 IAM RPC 查询用户信息（name, email），性能差。

**解决**：Staff 表冗余缓存常用字段。

```go
type Staff struct {
    // 核心业务数据
    iamUserID int64
    roles     []StaffRole
    
    // 冗余缓存（可从 IAM 同步）
    name      string
    email     string
    phone     string
}

// 定期同步
func (f *StaffFactory) SyncFromIAM(ctx context.Context, staff *Staff, name, email, phone string) error {
    staff.UpdateContactInfo(email, phone)
    staff.name = name
    return f.staffRepo.Update(ctx, staff)
}
```

## 设计要点

### 1. 轻量级设计

**不存储的内容**（由 IAM 管理）：

- ❌ 密码、Token
- ❌ 登录状态
- ❌ 通用权限（如 "能否访问后台"）

**存储的内容**（本 BC 业务数据）：

- ✅ 业务角色（roles）
- ✅ 机构隔离（orgID）
- ✅ 激活状态（isActive）
- ✅ 缓存字段（name, email）

### 2. 工厂模式（幂等创建）

```go
// GetOrCreateByIAMUser：首次使用时自动创建
staff, err := staffFactory.GetOrCreateByIAMUser(ctx, orgID, iamUserID, name)

// 后续直接查询
staff, err := staffRepo.FindByIAMUser(ctx, orgID, iamUserID)
```

### 3. 与 IAM 的集成

```go
// 应用层：从 Principal 获取 IAM UserID
func (s *StaffAppService) GetCurrentStaff(ctx context.Context) (*StaffDTO, error) {
    principal := GetPrincipal(ctx) // 从 token 解析
    iamUserID := principal.UserID
    
    // 查询或创建 Staff
    staff, err := s.staffFactory.GetOrCreateByIAMUser(ctx, principal.OrgID, iamUserID, principal.Name)
    // ...
}
```

## 对比：Testee 必须完整持久化

| 维度 | Staff | Testee |
|------|-------|--------|
| **本质** | 技术账号的业务投影 | 独立的业务实体 |
| **依赖 IAM** | 强依赖（必须绑定 IAM.User） | 弱依赖（可以不绑定） |
| **核心数据** | 业务角色（roles） | 测评历史、标签、风险等级 |
| **生命周期** | 跟随 IAM.User | 独立存在（可长期追踪） |
| **多租户** | 同一 IAM.User 在不同机构有不同 Staff | Testee 在机构内唯一 |

## 总结

**Staff 需要持久化，但设计上是轻量级的**：

1. **核心目的**：存储业务角色（roles），这是领域知识，IAM 不管
2. **外键关联**：通过 `iamUserID` 关联 IAM，不重复存储认证信息
3. **多租户支持**：同一 IAM.User 在不同机构可以有不同 Staff 记录
4. **性能优化**：缓存常用字段（name, email），减少 RPC 调用
5. **审计友好**：用 `StaffID` 记录操作，比直接用 `IAMUserID` 更有业务语义

**关键原则**：Staff 是 IAM.User 的"业务视图"，存储的是**本 BC 关心的业务属性**，而非用户的全量信息。
