# Testee 子域设计说明

## 概述

Testee（受试者）是问卷&量表BC中的核心聚合根，代表"被测评的人"。本模块采用充血模型设计，将领域服务从聚合根中提取，遵循单一职责原则。

## 核心组件

### 1. 聚合根：Testee

**职责**：维护受试者的核心状态和不变量

**设计原则**：
- ✅ 只包含通用属性（特定场景如筛查的属性在对应子域维护）
- ✅ 以行为为中心，而非数据中心
- ✅ 审计字段由基础设施层（PO）处理
- ✅ 限制对内部状态的直接访问，通过行为方法暴露能力

**核心字段**：
```go
type Testee struct {
    // 核心标识
    id, orgID
    
    // IAM 映射（松耦合）
    iamUserID, iamChildID
    
    // 基本属性
    name, gender, birthday
    
    // 业务标签与关注
    tags, source, isKeyFocus
    
    // 测评统计快照（读模型）
    assessmentStats
}
```

**行为方法**：
- `IsBoundToIAM()` - 检查IAM绑定状态
- `GetAge()` - 计算年龄
- `HasTag()`, `AddTag()`, `RemoveTag()` - 标签管理
- `MarkAsKeyFocus()` - 标记重点关注

---

### 2. 领域服务

采用职责分离设计，将不同关注点独立为领域服务：

#### 2.1 Validator - 验证器

**职责**：验证 Testee 的字段合法性

**设计原则**：
- ✅ **按字段维度提供验证方法**，而非按场景
- ✅ 可灵活组合，扩展性好
- ✅ 验证规则集中管理

**接口方法**：
```go
ValidateOrgID(orgID int64) error
ValidateName(name string, required bool) error
ValidateGender(gender Gender) error
ValidateBirthday(birthday *time.Time) error
ValidateTag(tag string) error
ValidateTags(tags []string) error
```

**使用示例**：
```go
validator := NewValidator()

// 创建场景：验证必填字段
validator.ValidateOrgID(orgID)
validator.ValidateName(name, true)  // required=true

// 更新场景：名字可选
validator.ValidateName(name, false) // required=false
```

---

#### 2.2 Binder - IAM绑定器

**职责**：管理 Testee 与 IAM 系统的绑定关系

**核心逻辑**：
- 防止重复绑定（一个 Testee 只能绑定一个 IAM 身份）
- 防止绑定冲突（一个 IAM 身份只能绑定一个 Testee）
- 验证绑定的有效性

**接口方法**：
```go
BindToIAMUser(ctx, testee, iamUserID) error
BindToIAMChild(ctx, testee, iamChildID) error
Unbind(ctx, testee) error
VerifyBinding(ctx, testee) error
```

**业务规则**：
1. Testee 不能同时绑定 User 和 Child
2. 同一个 IAM User/Child 在同一机构下只能绑定一个 Testee
3. 解绑后可以重新绑定

---

#### 2.3 Editor - 编辑器

**职责**：管理 Testee 信息的变更，包含业务规则验证

**设计原则**：
- 所有更新操作经过验证
- 关键操作可触发领域事件（如标记重点关注）
- 保证操作的幂等性

**接口方法**：
```go
UpdateBasicInfo(testee, name, gender, birthday) error
AddTag(testee, tag) error
RemoveTag(testee, tag) error
ReplaceTags(testee, tags) error
MarkAsKeyFocus(testee, reason) error
UnmarkAsKeyFocus(testee) error
```

**业务规则**：
- 标签数量最多 50 个
- 标签长度不超过 50 字符
- 重点关注操作应记录原因（预留）

---

#### 2.4 StatsUpdater - 统计更新器

**职责**：更新 Testee 的测评统计快照

**触发时机**：
- 测评完成时（通过领域事件触发）
- 数据修复时（手动触发重新计算）

**接口方法**：
```go
UpdateAfterAssessment(ctx, testee, assessmentTime, riskLevel) error
RecalculateStats(ctx, testee) error
IncrementCount(testee) error
```

**设计考虑**：
- 统计数据是**读模型快照**，用于性能优化
- 不应直接修改，而是通过事件驱动更新
- 支持从源数据重新计算（修复数据不一致）

**自动标签**：
- 高风险测评自动打 `high_risk` 标签（可配置）

---

### 3. 值对象

#### Gender - 性别枚举
```go
const (
    GenderUnknown Gender = 0
    GenderMale    Gender = 1
    GenderFemale  Gender = 2
)
```

#### AssessmentStats - 测评统计快照
```go
type AssessmentStats struct {
    lastAssessmentAt time.Time
    totalCount       int
    lastRiskLevel    string
}
```

**特点**：
- 不可变值对象
- 通过构造函数创建
- 只提供 getter 方法

---

### 4. 工厂（Factory）

**职责**：创建或获取 Testee 实例，处理幂等性

**方法**：
- `GetOrCreateByIAMChild` - 通过 IAM Child 获取或创建
- `GetOrCreateByIAMUser` - 通过 IAM User 获取或创建
- `CreateTemporary` - 创建临时受试者（无 IAM 绑定）

**特点**：
- 内置参数验证（依赖 Validator）
- 幂等操作：如果已存在则返回，不存在则创建
- 自动设置 source 字段标识来源

---

## 依赖关系

```
┌─────────────────────────────────────────────────┐
│              Application Layer                  │
│         (TesteeAppService)                      │
└────────────┬────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────┐
│              Domain Layer                       │
│                                                 │
│  ┌──────────┐      ┌──────────────────────┐    │
│  │  Testee  │◄─────┤  Domain Services:    │    │
│  │(Aggregate│      │  - Validator         │    │
│  │  Root)   │      │  - Binder            │    │
│  └──────────┘      │  - Editor            │    │
│                    │  - StatsUpdater      │    │
│                    │  - Factory           │    │
│                    └──────────────────────┘    │
│                                                 │
│  ┌──────────────────────────────────────┐      │
│  │  Value Objects:                      │      │
│  │  - Gender                            │      │
│  │  - AssessmentStats                   │      │
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

### 创建受试者

```go
// 1. 通过 IAM Child 创建
validator := testee.NewValidator()
factory := testee.NewFactory(repo, validator)

t, err := factory.GetOrCreateByIAMChild(
    ctx, 
    orgID, 
    iamChildID, 
    "张三", 
    int8(testee.GenderMale), 
    &birthday,
)

// 2. 创建临时受试者
t, err := factory.CreateTemporary(
    ctx,
    orgID,
    "李四",
    int8(testee.GenderFemale),
    &birthday,
    "walk_in", // 来源：线下到店
)
```

### 编辑受试者

```go
validator := testee.NewValidator()
editor := testee.NewEditor(validator)

// 更新基本信息
err := editor.UpdateBasicInfo(t, "张三三", testee.GenderMale, &newBirthday)

// 添加标签
err := editor.AddTag(t, "adhd_suspect")

// 标记重点关注
err := editor.MarkAsKeyFocus(t, "多次高风险测评")

// 保存到仓储
repo.Update(ctx, t)
```

### 绑定 IAM

```go
binder := testee.NewBinder(repo)

// 绑定到 IAM User
err := binder.BindToIAMUser(ctx, t, iamUserID)

// 验证绑定有效性
err := binder.VerifyBinding(ctx, t)

// 保存
repo.Update(ctx, t)
```

### 更新测评统计

```go
updater := testee.NewStatsUpdater(repo)

// 测评完成后更新
err := updater.UpdateAfterAssessment(
    ctx,
    t,
    time.Now(),
    "high", // 风险等级
)

// 保存
repo.Update(ctx, t)
```

---

## 设计优势

### 1. 单一职责
- 每个领域服务专注一个职责
- 聚合根不再承担所有逻辑
- 易于测试和维护

### 2. 可扩展性
- Validator 按字段验证，可灵活组合
- 新增业务规则只需修改对应服务
- 不影响其他部分

### 3. 可测试性
- 领域服务可独立单元测试
- Mock Repository 即可测试
- 业务逻辑与基础设施分离

### 4. 明确的依赖关系
- 领域服务依赖仓储接口，不依赖具体实现
- Factory 和 Binder 需要 Repository
- Editor 和 Validator 无状态，可独立使用

---

## 待实现

- [ ] Repository 的 MongoDB 实现
- [ ] 领域事件发布（测评统计更新、重点关注标记等）
- [ ] StatsUpdater 的 RecalculateStats 完整实现
- [ ] Binder 的 VerifyBinding 调用 IAM 服务验证
- [ ] 应用服务层的 DTO 和组装
