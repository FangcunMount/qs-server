# Testee 领域服务重新设计

## 概述

Testee 聚合根的领域服务层已完成重新设计，将不同职责的业务逻辑拆分到独立的领域服务中，遵循单一职责原则和领域驱动设计（DDD）最佳实践。

## 领域服务清单

### 1. Validator - 数据验证服务

**文件**: `validator.go`

**职责**: 负责在 Testee 创建、修改等操作时进行数据验证

**接口方法**:
```go
type Validator interface {
    // 场景验证
    ValidateForCreation(ctx context.Context, orgID int64, name string, gender Gender) error
    ValidateForUpdate(ctx context.Context, testee *Testee, name *string, gender *Gender) error
    ValidateProfileBinding(ctx context.Context, testee *Testee, profileID uint64) error
    
    // 字段验证
    ValidateName(name string, required bool) error
    ValidateGender(gender Gender) error
    ValidateBirthday(birthday *time.Time) error
    ValidateTag(tag string) error
    ValidateTags(tags []string) error
}
```

**使用场景**:
- 创建受试者前验证必填字段
- 更新受试者信息时验证数据合法性
- 绑定档案时验证绑定规则（如防止重复绑定）

**设计要点**:
- 提供场景级验证（ValidateForCreation）和字段级验证（ValidateName）
- 集成业务规则验证（如档案重复绑定检查）
- 依赖 Repository 检查数据唯一性

---

### 2. Binder - 档案绑定服务

**文件**: `binder.go`

**职责**: 负责将 Testee 与用户档案（Profile）进行绑定和解绑

**接口方法**:
```go
type Binder interface {
    // Bind 绑定到用户档案
    Bind(ctx context.Context, testee *Testee, profileID uint64) error
    
    // Unbind 解除档案绑定
    Unbind(ctx context.Context, testee *Testee) error
    
    // IsBound 检查是否已绑定
    IsBound(testee *Testee) bool
}
```

**使用场景**:
- C端用户注册后绑定档案
- 管理后台手动绑定受试者和档案
- 解除错误绑定

**设计要点**:
- 防止重复绑定：同一档案不能绑定多个受试者
- 幂等操作：重复绑定同一档案不报错
- 解绑后可重新绑定其他档案

**业务规则**:
1. 一个受试者最多绑定一个档案
2. 一个档案最多绑定一个受试者（同一机构内）
3. 绑定关系可以解除并重新建立

---

### 3. Tagger - 标签管理服务

**文件**: `tagger.go`

**职责**: 负责给受试者打标签、移除标签、清空标签

**接口方法**:
```go
type Tagger interface {
    // Tag 给受试者打标签
    Tag(ctx context.Context, testee *Testee, tag string) error
    
    // UnTag 移除受试者的标签
    UnTag(ctx context.Context, testee *Testee, tag string) error
    
    // CleanTag 清空受试者的所有标签
    CleanTag(ctx context.Context, testee *Testee) error
}
```

**使用场景**:
- 根据测评结果自动打标签（如 "high_risk"）
- 手动添加业务标签（如 "vip", "adhd_suspect"）
- 清理过期标签

**设计要点**:
- 自动去重：不会添加重复标签
- 幂等操作：移除不存在的标签不报错
- 依赖 Validator 验证标签格式

**标签示例**:
- `high_risk` - 高风险
- `adhd_suspect` - ADHD 嫌疑
- `vip` - VIP 用户
- `screening_2024` - 2024年筛查对象

---

### 4. Editor - 信息编辑服务

**文件**: `editor.go`

**职责**: 负责编辑受试者的基本信息和关注状态

**接口方法**:
```go
type Editor interface {
    // UpdateBasicInfo 更新基本信息（姓名、性别、生日）
    UpdateBasicInfo(ctx context.Context, testee *Testee, name *string, gender *Gender, birthday *time.Time) error
    
    // MarkAsKeyFocus 标记为重点关注
    MarkAsKeyFocus(ctx context.Context, testee *Testee) error
    
    // UnmarkAsKeyFocus 取消重点关注
    UnmarkAsKeyFocus(ctx context.Context, testee *Testee) error
}
```

**使用场景**:
- 更新受试者个人信息
- 标记高风险用户为重点关注
- 取消重点关注状态

**设计要点**:
- 支持部分字段更新（使用指针参数）
- 依赖 Validator 验证更新数据
- 幂等操作：重复标记/取消标记不报错

**参数设计**:
- 使用 `*string`, `*Gender` 指针区分"不更新"和"更新为空"
- `nil` = 不更新该字段
- `&value` = 更新为指定值

---

### 5. AssessmentCounter - 测评统计服务

**文件**: `stats_updater.go` (保留文件名向后兼容)

**职责**: 负责统计测评次数和更新测评快照

**接口方法**:
```go
type AssessmentCounter interface {
    // AddAssessment 添加测评记录并更新统计
    AddAssessment(ctx context.Context, testee *Testee, assessmentTime time.Time, riskLevel string) error
    
    // RecalculateStats 重新计算统计（用于修复数据）
    RecalculateStats(ctx context.Context, testee *Testee) error
}
```

**使用场景**:
- 测评完成后更新受试者统计
- 修复统计数据不一致问题
- 自动打标签（高风险用户）

**设计要点**:
- 统计信息包括：总测评次数、最后测评时间、最后风险等级
- AddAssessment 会自动增加计数并更新最后测评信息
- RecalculateStats 从数据库重新计算（需要集成 Assessment 仓储）

**触发时机**:
- 通过领域事件触发：`AssessmentCompletedEvent`
- 应用层监听事件并调用 `AddAssessment`

**兼容性**:
```go
// 保留旧接口名用于兼容
type StatsUpdater = AssessmentCounter
func NewStatsUpdater(repo Repository) StatsUpdater
```

---

## 架构设计

### 职责分离

```
┌─────────────────────────────────────────────────────────┐
│                    Application Layer                     │
│              (编排领域服务，处理业务流程)                │
└────────────────────┬────────────────────────────────────┘
                     │ 调用
┌────────────────────▼────────────────────────────────────┐
│                    Domain Services                       │
│   ┌──────────────┐ ┌──────────┐ ┌──────────────────┐   │
│   │  Validator   │ │  Binder  │ │  Tagger          │   │
│   │  数据验证    │ │  档案绑定│ │  标签管理        │   │
│   └──────────────┘ └──────────┘ └──────────────────┘   │
│   ┌──────────────┐ ┌──────────────────────────────┐   │
│   │  Editor      │ │  AssessmentCounter           │   │
│   │  信息编辑    │ │  测评统计                    │   │
│   └──────────────┘ └──────────────────────────────┘   │
└────────────────────┬────────────────────────────────────┘
                     │ 操作
┌────────────────────▼────────────────────────────────────┐
│                  Testee Aggregate                        │
│              (包内方法: bindProfile, addTag, etc)        │
└──────────────────────────────────────────────────────────┘
```

### 依赖关系

```
AssessmentCounter ──► Repository
Validator ────────────► Repository
Binder ───────────────► Repository
Tagger ───────────────► Validator
Editor ───────────────► Validator

所有服务 ──────────► Testee (聚合根)
```

### 方法可见性

**Testee 聚合根方法分类**:

1. **公开方法** (外部可调用):
   - ID(), OrgID(), Name(), Gender(), Birthday()
   - Tags(), HasTag(), IsKeyFocus()
   - ProfileID(), IsBoundToProfile()

2. **包内方法** (领域服务可调用):
   - bindProfile(profileID)
   - addTag(tag), removeTag(tag), clearTags()
   - updateBasicInfo(name, gender, birthday)
   - markAsKeyFocus(), unmarkAsKeyFocus()
   - updateAssessmentStats(stats)

3. **仓储专用方法** (仅持久化层使用):
   - SetID(id), SetSource(source), SetTags(tags)
   - RestoreFromRepository(...)

---

## 使用示例

### 示例 1: 创建受试者并绑定档案

```go
// 应用层代码
func (s *TesteeManagementService) CreateAndBind(
    ctx context.Context,
    orgID int64,
    profileID uint64,
    name string,
    gender int8,
    birthday *time.Time,
) (*Testee, error) {
    // 1. 验证数据
    if err := s.validator.ValidateForCreation(ctx, orgID, name, Gender(gender)); err != nil {
        return nil, err
    }
    
    // 2. 创建受试者
    testee := NewTestee(orgID, name, Gender(gender), birthday)
    
    // 3. 绑定档案
    if err := s.binder.Bind(ctx, testee, profileID); err != nil {
        return nil, err
    }
    
    // 4. 持久化
    if err := s.repo.Save(ctx, testee); err != nil {
        return nil, err
    }
    
    return testee, nil
}
```

### 示例 2: 测评完成后更新统计

```go
// 应用层事件处理器
func (h *AssessmentCompletedHandler) Handle(ctx context.Context, event AssessmentCompletedEvent) error {
    // 1. 获取受试者
    testee, err := h.repo.FindByID(ctx, event.TesteeID)
    if err != nil {
        return err
    }
    
    // 2. 更新测评统计
    if err := h.counter.AddAssessment(ctx, testee, event.CompletedAt, event.RiskLevel); err != nil {
        return err
    }
    
    // 3. 根据风险等级打标签
    if event.RiskLevel == "high" {
        if err := h.tagger.Tag(ctx, testee, "high_risk"); err != nil {
            return err
        }
    }
    
    // 4. 持久化
    return h.repo.Update(ctx, testee)
}
```

### 示例 3: 更新基本信息

```go
// 应用层代码
func (s *TesteeManagementService) UpdateInfo(
    ctx context.Context,
    testeeID ID,
    name *string,
    gender *int8,
) error {
    // 1. 获取受试者
    testee, err := s.repo.FindByID(ctx, testeeID)
    if err != nil {
        return err
    }
    
    // 2. 转换性别类型
    var genderPtr *Gender
    if gender != nil {
        g := Gender(*gender)
        genderPtr = &g
    }
    
    // 3. 更新信息（Editor 会自动验证）
    if err := s.editor.UpdateBasicInfo(ctx, testee, name, genderPtr, nil); err != nil {
        return err
    }
    
    // 4. 持久化
    return s.repo.Update(ctx, testee)
}
```

---

## 测试策略

### 单元测试

每个领域服务都应该有独立的单元测试：

```go
// validator_test.go
func TestValidator_ValidateForCreation(t *testing.T) {
    // 测试各种验证场景
}

// binder_test.go
func TestBinder_Bind_PreventDuplicate(t *testing.T) {
    // 测试防止重复绑定
}

// tagger_test.go
func TestTagger_Tag_Idempotent(t *testing.T) {
    // 测试幂等性
}
```

### 集成测试

测试领域服务与 Repository 的交互：

```go
// integration_test.go
func TestBinderWithRepository(t *testing.T) {
    // 使用真实数据库测试绑定逻辑
}
```

---

## 迁移指南

### 应用层代码调整

**旧代码**:
```go
// 直接操作实体
testee.addTag("high_risk")
testee.markAsKeyFocus()
```

**新代码**:
```go
// 通过领域服务操作
tagger.Tag(ctx, testee, "high_risk")
editor.MarkAsKeyFocus(ctx, testee)
```

### 依赖注入

应用层服务需要注入领域服务：

```go
type TesteeManagementService struct {
    repo      Repository
    validator Validator
    binder    Binder
    tagger    Tagger
    editor    Editor
    counter   AssessmentCounter
}

func NewTesteeManagementService(repo Repository) *TesteeManagementService {
    validator := NewValidator(repo)
    return &TesteeManagementService{
        repo:      repo,
        validator: validator,
        binder:    NewBinder(repo),
        tagger:    NewTagger(validator),
        editor:    NewEditor(validator),
        counter:   NewAssessmentCounter(repo),
    }
}
```

---

## 设计原则

### 1. 单一职责原则 (SRP)
每个领域服务只负责一个业务领域：
- Validator 只管验证
- Binder 只管绑定
- Tagger 只管标签

### 2. 依赖倒置原则 (DIP)
领域服务依赖接口，不依赖具体实现：
```go
type Validator interface { ... }  // 接口
type validator struct { ... }      // 实现
```

### 3. 开闭原则 (OCP)
通过接口扩展功能，不修改现有代码：
- 新增验证规则：实现新的 Validator
- 新增统计维度：扩展 AssessmentCounter

### 4. 接口隔离原则 (ISP)
接口方法精简，避免臃肿：
- Binder 只有 3 个方法
- Tagger 只有 3 个方法

### 5. 最少知识原则 (LoD)
领域服务只访问 Testee 的包内方法，不直接修改私有字段：
```go
// ✅ 正确
testee.bindProfile(profileID)

// ❌ 错误
testee.profileID = &profileID
```

---

## 后续优化

### 1. 领域事件
- `TesteeCreated` - 受试者创建
- `ProfileBound` - 档案绑定
- `TagAdded` - 标签添加
- `KeyFocusMarked` - 标记重点关注
- `AssessmentStatsUpdated` - 统计更新

### 2. 规格模式 (Specification)
提取复杂查询条件到 Specification：
```go
type HighRiskSpecification struct{}
func (s *HighRiskSpecification) IsSatisfiedBy(testee *Testee) bool {
    // 判断是否高风险
}
```

### 3. 策略模式
不同场景的验证策略：
```go
type ValidationStrategy interface {
    Validate(testee *Testee) error
}

type CreationValidationStrategy struct{}
type UpdateValidationStrategy struct{}
```

---

## 总结

✅ **完成情况**:
- 5 个领域服务已完成重新设计
- 所有代码编译通过
- 职责清晰，依赖合理
- 遵循 DDD 和 SOLID 原则

⏳ **待完成**:
- 应用层代码调整（使用新的领域服务）
- 单元测试和集成测试
- 领域事件集成

🎯 **设计目标达成**:
- 单一职责：每个服务职责明确
- 可测试性：易于编写单元测试
- 可扩展性：易于添加新功能
- 可维护性：代码结构清晰
