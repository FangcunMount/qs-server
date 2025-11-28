# Scale 子域设计文档

> **版本**: 2.0  
> **更新日期**: 2025-01-28  
> **适用范围**: qs-server 问卷量表系统 - Scale 子域

---

## 核心要点（TL;DR）

Scale 子域是一个**纯配置型领域**，职责单一且明确：

```text
┌──────────────────────────────────────────────────────────────────┐
│                         Scale 子域核心定位                        │
├──────────────────────────────────────────────────────────────────┤
│  ✅ 量表是什么：标题、描述、关联问卷                               │
│  ✅ 测量什么维度：因子结构定义                                     │
│  ✅ 如何计分：每个因子的计分策略配置                               │
│  ✅ 如何解读：分数区间 → 风险等级 + 结论                           │
├──────────────────────────────────────────────────────────────────┤
│  ❌ 不关心问卷怎么展示 → Survey 子域                              │
│  ❌ 不关心谁做了测评 → Assessment 子域                            │
│  ❌ 不关心数据怎么存储 → Infrastructure 层                        │
└──────────────────────────────────────────────────────────────────┘
```

---

## 1. 整体架构

### 1.1 六边形架构视图

```text
                            ┌─────────────────────────────────┐
                            │         Interface Layer         │
                            │   (RESTful Handler / gRPC)      │
                            └───────────────┬─────────────────┘
                                            │ 调用
                                            ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                            Application Layer                                │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐             │
│  │LifecycleService │  │  FactorService  │  │  QueryService   │             │
│  │   (管理员)       │  │   (因子编辑者)   │  │   (所有用户)    │             │
│  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘             │
└───────────┼────────────────────┼────────────────────┼───────────────────────┘
            │                    │                    │
            │  协调领域服务       │                    │
            ▼                    ▼                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Domain Layer                                   │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                     MedicalScale (聚合根)                            │   │
│  │  ┌──────────┐  ┌──────────┐  ┌─────────────────────┐               │   │
│  │  │  Factor  │  │  Factor  │  │ InterpretationRule  │               │   │
│  │  │  (实体)  │  │  (实体)  │  │     (值对象)        │               │   │
│  │  └──────────┘  └──────────┘  └─────────────────────┘               │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌────────────┐  ┌────────────┐  ┌──────────────┐  ┌────────────┐         │
│  │ Lifecycle  │  │  BaseInfo  │  │FactorManager │  │ Validator  │         │
│  │  (服务)    │  │   (服务)   │  │   (服务)     │  │  (服务)    │         │
│  └────────────┘  └────────────┘  └──────────────┘  └────────────┘         │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    Repository (出站端口)                             │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└───────────┬─────────────────────────────────────────────────────────────────┘
            │ 依赖倒置
            ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                          Infrastructure Layer                               │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │               MongoDB Repository (适配器实现)                        │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐                          │   │
│  │  │ ScalePO  │  │ FactorPO │  │  Mapper  │                          │   │
│  │  └──────────┘  └──────────┘  └──────────┘                          │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 1.2 目录结构

```text
internal/apiserver/
├── domain/scale/                    # 领域层
│   ├── types.go                     # 类型定义（Status, FactorCode, RiskLevel 等）
│   ├── medical_scale.go             # 聚合根
│   ├── factor.go                    # 因子实体
│   ├── interpretation_rule.go       # 解读规则值对象
│   ├── lifecycle.go                 # 生命周期领域服务
│   ├── baseinfo.go                  # 基础信息领域服务
│   ├── factor_manager.go            # 因子管理领域服务
│   ├── validator.go                 # 发布验证器
│   └── repository.go                # 仓储接口（出站端口）
│
├── application/scale/               # 应用层
│   ├── interface.go                 # 服务接口定义（入站端口）
│   ├── dto.go                       # 数据传输对象
│   ├── converter.go                 # DTO ↔ Domain 转换器
│   ├── lifecycle_service.go         # 生命周期服务实现
│   ├── factor_service.go            # 因子服务实现
│   └── query_service.go             # 查询服务实现
│
├── infra/mongo/scale/               # 基础设施层
│   ├── po.go                        # 持久化对象
│   ├── mapper.go                    # PO ↔ Domain 映射器
│   └── repo.go                      # 仓储实现
│
├── interface/restful/               # 接口层
│   └── handler/scale.go             # RESTful 处理器
│
└── container/assembler/scale.go     # 模块组装器
```

---

## 2. 领域模型设计

### 2.1 聚合根：MedicalScale

MedicalScale 是 Scale 子域**唯一的聚合根**，代表一个医学/心理量表的完整定义。

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                           MedicalScale (聚合根)                             │
├─────────────────────────────────────────────────────────────────────────────┤
│  标识                                                                        │
│  ├── id: meta.ID              # 唯一标识（MongoDB ObjectID）                 │
│  └── scaleCode: meta.Code     # 业务编码（如 "SDS-001"）                     │
├─────────────────────────────────────────────────────────────────────────────┤
│  基本信息                                                                    │
│  ├── title: string            # 量表标题（如 "抑郁自评量表"）                 │
│  └── description: string      # 量表描述                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│  关联问卷                                                                    │
│  ├── questionnaireCode: meta.Code    # 问卷编码                              │
│  └── questionnaireVersion: string    # 问卷版本号                            │
├─────────────────────────────────────────────────────────────────────────────┤
│  状态                                                                        │
│  └── status: Status           # Draft → Published → Archived               │
├─────────────────────────────────────────────────────────────────────────────┤
│  因子列表                                                                    │
│  └── factors: []*Factor       # 包含多个因子（维度）                         │
└─────────────────────────────────────────────────────────────────────────────┘
```

**状态流转**：

```text
         Publish()              Archive()
  ┌─────┐ ────────► ┌──────────┐ ────────► ┌──────────┐
  │Draft│           │Published │           │ Archived │
  └─────┘ ◄──────── └──────────┘           └──────────┘
         Unpublish()
```

**代码链接**: [`domain/scale/medical_scale.go`](../../internal/apiserver/domain/scale/medical_scale.go)

### 2.2 实体：Factor

Factor 是量表的组成部分，代表一个测量维度（如"躯体症状"、"抑郁情绪"等）。

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Factor (实体)                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│  基本信息                                                                    │
│  ├── code: FactorCode         # 因子编码（如 "depression"）                  │
│  ├── title: string            # 因子标题（如 "抑郁症状"）                    │
│  ├── factorType: FactorType   # 类型（primary/multilevel）                  │
│  └── isTotalScore: bool       # 是否为总分因子                               │
├─────────────────────────────────────────────────────────────────────────────┤
│  题目关联                                                                    │
│  └── questionCodes: []meta.Code  # 该因子包含的题目编码列表                  │
├─────────────────────────────────────────────────────────────────────────────┤
│  计分配置                                                                    │
│  ├── scoringStrategy: ScoringStrategyCode  # 计分策略（sum/avg/custom）      │
│  └── scoringParams: map[string]string      # 策略参数                        │
├─────────────────────────────────────────────────────────────────────────────┤
│  解读规则                                                                    │
│  └── interpretRules: []InterpretationRule  # 该因子的解读规则列表            │
└─────────────────────────────────────────────────────────────────────────────┘
```

**设计要点**：

- Factor 通过 `questionCodes` **逻辑关联**题目，不持有 Question 对象
- 每个 Factor 独立配置计分策略和解读规则
- 一个量表**必须有且仅有一个**总分因子（`isTotalScore = true`）

**代码链接**: [`domain/scale/factor.go`](../../internal/apiserver/domain/scale/factor.go)

### 2.3 值对象：InterpretationRule

InterpretationRule 定义"分数区间 → 风险等级 + 文案"的映射。

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                        InterpretationRule (值对象)                          │
├─────────────────────────────────────────────────────────────────────────────┤
│  scoreRange: ScoreRange                                                      │
│  ├── min: float64      # 最小分（包含）                                      │
│  └── max: float64      # 最大分（不包含）                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│  riskLevel: RiskLevel  # none / low / medium / high / severe                │
├─────────────────────────────────────────────────────────────────────────────┤
│  conclusion: string    # 结论文案（如 "轻度抑郁症状"）                        │
├─────────────────────────────────────────────────────────────────────────────┤
│  suggestion: string    # 建议文案（如 "建议进行心理咨询"）                    │
└─────────────────────────────────────────────────────────────────────────────┘
```

**匹配逻辑**：分数区间使用左闭右开 `[min, max)` 避免边界重叠

```go
func (r InterpretationRule) Matches(score float64) bool {
    return r.scoreRange.Contains(score)  // min <= score < max
}
```

**代码链接**: [`domain/scale/interpretation_rule.go`](../../internal/apiserver/domain/scale/interpretation_rule.go)

---

## 3. 设计模式应用

### 3.1 Option 模式 - 灵活构造

问题：聚合根和实体有多个可选属性，传统构造函数参数过多

**解决方案**：使用 Option 函数模式

```go
// 聚合根构造选项
type MedicalScaleOption func(*MedicalScale)

// 构造函数 - 只包含必填参数
func NewMedicalScale(scaleCode meta.Code, title string, opts ...MedicalScaleOption) (*MedicalScale, error)

// 可选属性通过 With*** 函数设置
func WithID(id meta.ID) MedicalScaleOption
func WithDescription(desc string) MedicalScaleOption
func WithQuestionnaire(qCode meta.Code, qVersion string) MedicalScaleOption
func WithStatus(s Status) MedicalScaleOption
func WithFactors(factors []*Factor) MedicalScaleOption
```

**使用示例**：

```go
// 创建新量表 - 简洁清晰
scale, _ := NewMedicalScale(
    meta.NewCode("SDS-001"),
    "抑郁自评量表",
    WithDescription("Zung Self-Rating Depression Scale"),
    WithQuestionnaire(meta.NewCode("Q-001"), "1.0"),
)

// 从数据库重建 - 完整恢复
scale, _ := NewMedicalScale(
    meta.NewCode("SDS-001"),
    "抑郁自评量表",
    WithID(existingID),
    WithStatus(StatusPublished),
    WithFactors(existingFactors),
)
```

**代码链接**: [`domain/scale/medical_scale.go`](../../internal/apiserver/domain/scale/medical_scale.go) (第32-90行)

### 3.2 领域服务分离 - 单一职责

问题：聚合根方法过多，职责不清晰，难以维护

**解决方案**：按职责拆分为独立的领域服务

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                          领域服务分离                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   ┌────────────┐     ┌────────────┐     ┌──────────────┐    ┌────────────┐ │
│   │ Lifecycle  │     │  BaseInfo  │     │FactorManager │    │ Validator  │ │
│   │    服务    │     │    服务    │     │     服务     │    │    服务    │ │
│   └─────┬──────┘     └─────┬──────┘     └──────┬───────┘    └──────┬─────┘ │
│         │                  │                   │                    │       │
│   ┌─────▼──────────────────▼───────────────────▼────────────────────▼─────┐ │
│   │                        MedicalScale 聚合根                             │ │
│   │                    (私有方法：updateStatus, updateBasicInfo 等)        │ │
│   └───────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

| 服务 | 职责 | 典型方法 |
|------|------|----------|
| `Lifecycle` | 状态流转管理 | `Publish()`, `Unpublish()`, `Archive()` |
| `BaseInfo` | 基础信息维护 | `UpdateTitle()`, `UpdateDescription()`, `UpdateQuestionnaire()` |
| `FactorManager` | 因子增删改 | `AddFactor()`, `RemoveFactor()`, `ReplaceFactors()` |
| `Validator` | 发布前验证 | `ValidateForPublish()` |

**代码链接**:

- [`domain/scale/lifecycle.go`](../../internal/apiserver/domain/scale/lifecycle.go)
- [`domain/scale/baseinfo.go`](../../internal/apiserver/domain/scale/baseinfo.go)
- [`domain/scale/factor_manager.go`](../../internal/apiserver/domain/scale/factor_manager.go)
- [`domain/scale/validator.go`](../../internal/apiserver/domain/scale/validator.go)

### 3.3 SRP 服务设计 - 按行为者分离

问题：应用服务接口过大，不同角色的需求变更会相互影响

**解决方案**：按行为者（Actor）拆分应用服务

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                        应用服务按行为者分离                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌───────────────────┐   ┌───────────────────┐   ┌───────────────────┐     │
│  │ScaleLifecycleService│   │ ScaleFactorService │   │ ScaleQueryService │     │
│  │    (管理员)        │   │   (因子编辑者)     │   │   (所有用户)      │     │
│  ├───────────────────┤   ├───────────────────┤   ├───────────────────┤     │
│  │ Create()          │   │ AddFactor()       │   │ GetByCode()       │     │
│  │ UpdateBasicInfo() │   │ UpdateFactor()    │   │ List()            │     │
│  │ Publish()         │   │ RemoveFactor()    │   │ GetPublishedByCode│     │
│  │ Unpublish()       │   │ ReplaceFactors()  │   │ ListPublished()   │     │
│  │ Archive()         │   │ ReplaceInterpret  │   │                   │     │
│  │ Delete()          │   │   Rules()         │   │                   │     │
│  └───────────────────┘   └───────────────────┘   └───────────────────┘     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**设计优势**：

- 每个服务只有一个变更来源
- 接口小而专注，易于测试和 Mock
- 符合 SOLID 单一职责原则

**代码链接**: [`application/scale/interface.go`](../../internal/apiserver/application/scale/interface.go)

### 3.4 依赖倒置 - 仓储接口

问题：领域层不应依赖具体存储技术

**解决方案**：在领域层定义 Repository 接口（出站端口）

```go
// 领域层定义接口（抽象）
type Repository interface {
    Create(ctx context.Context, scale *MedicalScale) error
    FindByCode(ctx context.Context, code string) (*MedicalScale, error)
    Update(ctx context.Context, scale *MedicalScale) error
    Remove(ctx context.Context, code string) error
    // ...
}
```

```go
// 基础设施层实现接口（具体）
type Repository struct {  // MongoDB 实现
    BaseRepository
    mapper *ScaleMapper
}

func (r *Repository) FindByCode(ctx context.Context, code string) (*scale.MedicalScale, error) {
    // MongoDB 具体查询逻辑
}
```

**依赖方向**：

```text
Domain Layer                Infrastructure Layer
     │                              │
     │    ┌────────────────┐        │
     └────┤   Repository   ├────────┘
          │   (interface)  │
          └───────┬────────┘
                  │ 实现
          ┌───────▼────────┐
          │ MongoDB Repo   │
          └────────────────┘
```

**代码链接**:

- 接口定义: [`domain/scale/repository.go`](../../internal/apiserver/domain/scale/repository.go)
- MongoDB 实现: [`infra/mongo/scale/repo.go`](../../internal/apiserver/infra/mongo/scale/repo.go)

---

## 4. 运行流程

### 4.1 创建量表

```text
┌──────────┐     ┌──────────────┐     ┌─────────────────┐     ┌────────────┐
│  Client  │────►│ ScaleHandler │────►│LifecycleService │────►│ Repository │
└──────────┘     └──────────────┘     └─────────────────┘     └────────────┘
     │                  │                      │                     │
     │ POST /scales     │                      │                     │
     │ {title, desc}    │                      │                     │
     │─────────────────►│                      │                     │
     │                  │ Create(dto)          │                     │
     │                  │─────────────────────►│                     │
     │                  │                      │ NewMedicalScale()   │
     │                  │                      │──────┐              │
     │                  │                      │      │ 创建聚合根   │
     │                  │                      │◄─────┘              │
     │                  │                      │                     │
     │                  │                      │ repo.Create()       │
     │                  │                      │────────────────────►│
     │                  │                      │                     │ MongoDB
     │                  │                      │                     │ Insert
     │                  │   ScaleResult        │                     │
     │                  │◄─────────────────────│                     │
     │  201 Created     │                      │                     │
     │◄─────────────────│                      │                     │
```

**伪代码**：

```go
func (s *lifecycleService) Create(ctx context.Context, dto CreateScaleDTO) (*ScaleResult, error) {
    // 1. 生成编码
    code := meta.GenerateCode()
    
    // 2. 构造聚合根
    scale := NewMedicalScale(code, dto.Title,
        WithDescription(dto.Description),
        WithQuestionnaire(dto.QuestionnaireCode, dto.QuestionnaireVersion),
    )
    
    // 3. 持久化
    s.repo.Create(ctx, scale)
    
    // 4. 返回结果
    return toScaleResult(scale)
}
```

### 4.2 发布量表

```text
┌──────────┐     ┌──────────────┐     ┌─────────────────┐     ┌───────────┐
│  Client  │────►│ ScaleHandler │────►│LifecycleService │────►│ Lifecycle │
└──────────┘     └──────────────┘     └─────────────────┘     │  Service  │
     │                  │                      │              └─────┬─────┘
     │ POST /scales/:   │                      │                    │
     │   code/publish   │                      │                    │
     │─────────────────►│                      │                    │
     │                  │ Publish(code)        │                    │
     │                  │─────────────────────►│                    │
     │                  │                      │ repo.FindByCode()  │
     │                  │                      │──────┐             │
     │                  │                      │      │ 加载聚合根  │
     │                  │                      │◄─────┘             │
     │                  │                      │                    │
     │                  │                      │ lifecycle.Publish()│
     │                  │                      │───────────────────►│
     │                  │                      │                    │
     │                  │                      │    ┌───────────────┼───┐
     │                  │                      │    │ 1. 状态检查   │   │
     │                  │                      │    │ 2. 验证器校验 │   │
     │                  │                      │    │ 3. 更新状态   │   │
     │                  │                      │    └───────────────┼───┘
     │                  │                      │                    │
     │                  │                      │ repo.Update()      │
     │                  │                      │──────┐             │
     │                  │                      │      │ 持久化      │
     │                  │                      │◄─────┘             │
     │  200 OK          │                      │                    │
     │◄─────────────────│                      │                    │
```

**发布验证规则** (Validator)：

```go
func (Validator) ValidateForPublish(m *MedicalScale) []ValidationError {
    var errs []ValidationError
    
    // 必须有标题和编码
    if m.GetTitle() == "" { errs = append(errs, ...) }
    
    // 必须至少有一个因子
    if m.FactorCount() == 0 { errs = append(errs, ...) }
    
    // 必须有总分因子
    if _, ok := m.GetTotalScoreFactor(); !ok { errs = append(errs, ...) }
    
    // 必须关联问卷
    if m.GetQuestionnaireCode().IsEmpty() { errs = append(errs, ...) }
    
    // 每个因子必须有解读规则
    for _, factor := range m.GetFactors() {
        if len(factor.GetInterpretRules()) == 0 { errs = append(errs, ...) }
    }
    
    return errs
}
```

### 4.3 批量替换因子

```text
┌──────────┐     ┌──────────────┐     ┌───────────────┐     ┌──────────────┐
│  Client  │────►│ ScaleHandler │────►│ FactorService │────►│FactorManager │
└──────────┘     └──────────────┘     └───────────────┘     └──────────────┘
     │                  │                      │                     │
     │ PUT /scales/:    │                      │                     │
     │  code/factors    │                      │                     │
     │ [{code, title,   │                      │                     │
     │   questionCodes, │                      │                     │
     │   rules...}]     │                      │                     │
     │─────────────────►│                      │                     │
     │                  │ ReplaceFactors(code, │                     │
     │                  │   factors)           │                     │
     │                  │─────────────────────►│                     │
     │                  │                      │ 1. 加载量表         │
     │                  │                      │ 2. DTO → Domain     │
     │                  │                      │    转换因子         │
     │                  │                      │                     │
     │                  │                      │ factorManager.      │
     │                  │                      │   ReplaceFactors()  │
     │                  │                      │────────────────────►│
     │                  │                      │                     │
     │                  │                      │   ┌─────────────────┼─────┐
     │                  │                      │   │ 1. 验证唯一性   │     │
     │                  │                      │   │ 2. 验证总分因子 │     │
     │                  │                      │   │ 3. 替换列表     │     │
     │                  │                      │   └─────────────────┼─────┘
     │                  │                      │                     │
     │                  │                      │ 3. 持久化           │
     │  200 OK          │                      │                     │
     │◄─────────────────│                      │                     │
```

---

## 5. 模块组装

### 5.1 依赖注入

模块组装器（Assembler）负责按正确顺序创建和注入依赖：

```go
// ScaleModule 模块组装
type ScaleModule struct {
    Repo             scale.Repository
    Handler          *handler.ScaleHandler
    LifecycleService scaleApp.ScaleLifecycleService
    FactorService    scaleApp.ScaleFactorService
    QueryService     scaleApp.ScaleQueryService
}

func (m *ScaleModule) Initialize(params ...interface{}) error {
    mongoDB := params[0].(*mongo.Database)
    
    // 1. 初始化 Repository（最底层）
    m.Repo = scaleInfra.NewRepository(mongoDB)
    
    // 2. 初始化 Application Services（依赖 Repository）
    m.LifecycleService = scaleApp.NewLifecycleService(m.Repo)
    m.FactorService = scaleApp.NewFactorService(m.Repo)
    m.QueryService = scaleApp.NewQueryService(m.Repo)
    
    // 3. 初始化 Handler（依赖 Services）
    m.Handler = handler.NewScaleHandler(
        m.LifecycleService,
        m.FactorService,
        m.QueryService,
    )
    
    return nil
}
```

**依赖组装顺序**：

```text
MongoDB Database
       │
       ▼
┌────────────┐
│ Repository │ ← 最先创建（无依赖）
└─────┬──────┘
      │
      ▼
┌────────────────────────────────────────────┐
│             Application Services            │ ← 依赖 Repository
│  (LifecycleService, FactorService, Query)  │
└─────────────────────┬──────────────────────┘
                      │
                      ▼
               ┌────────────┐
               │  Handler   │ ← 依赖 Services
               └────────────┘
```

**代码链接**: [`container/assembler/scale.go`](../../internal/apiserver/container/assembler/scale.go)

### 5.2 路由注册

```go
// routers.go
func (r *Router) registerScaleProtectedRoutes(apiV1 *gin.RouterGroup) {
    scaleHandler := r.container.ScaleModule.Handler
    
    scales := apiV1.Group("/scales")
    {
        // 生命周期管理
        scales.POST("", scaleHandler.Create)
        scales.PUT("/:code/basic-info", scaleHandler.UpdateBasicInfo)
        scales.PUT("/:code/questionnaire", scaleHandler.UpdateQuestionnaire)
        scales.POST("/:code/publish", scaleHandler.Publish)
        scales.POST("/:code/unpublish", scaleHandler.Unpublish)
        scales.POST("/:code/archive", scaleHandler.Archive)
        scales.DELETE("/:code", scaleHandler.Delete)
        
        // 因子管理（仅批量操作）
        scales.PUT("/:code/factors", scaleHandler.ReplaceFactors)
        scales.PUT("/:code/interpret-rules", scaleHandler.ReplaceInterpretRules)
        
        // 查询接口
        scales.GET("/:code", scaleHandler.GetByCode)
        scales.GET("", scaleHandler.List)
        scales.GET("/by-questionnaire", scaleHandler.GetByQuestionnaireCode)
        scales.GET("/published/:code", scaleHandler.GetPublishedByCode)
        scales.GET("/published", scaleHandler.ListPublished)
    }
}
```

---

## 6. API 设计

### 6.1 RESTful 接口总览

| 方法 | 路径 | 说明 | 服务 |
|------|------|------|------|
| `POST` | `/api/v1/scales` | 创建量表 | Lifecycle |
| `PUT` | `/api/v1/scales/:code/basic-info` | 更新基本信息 | Lifecycle |
| `PUT` | `/api/v1/scales/:code/questionnaire` | 更新关联问卷 | Lifecycle |
| `POST` | `/api/v1/scales/:code/publish` | 发布量表 | Lifecycle |
| `POST` | `/api/v1/scales/:code/unpublish` | 下架量表 | Lifecycle |
| `POST` | `/api/v1/scales/:code/archive` | 归档量表 | Lifecycle |
| `DELETE` | `/api/v1/scales/:code` | 删除量表 | Lifecycle |
| `PUT` | `/api/v1/scales/:code/factors` | 批量替换因子 | Factor |
| `PUT` | `/api/v1/scales/:code/interpret-rules` | 批量设置解读规则 | Factor |
| `GET` | `/api/v1/scales/:code` | 获取量表详情 | Query |
| `GET` | `/api/v1/scales` | 量表列表 | Query |
| `GET` | `/api/v1/scales/by-questionnaire` | 按问卷查询 | Query |
| `GET` | `/api/v1/scales/published/:code` | 获取已发布量表 | Query |
| `GET` | `/api/v1/scales/published` | 已发布列表 | Query |

### 6.2 请求示例

**创建量表**：

```bash
POST /api/v1/scales
Content-Type: application/json

{
    "title": "抑郁自评量表",
    "description": "SDS 量表用于评估抑郁症状严重程度",
    "questionnaire_code": "Q-SDS-001",
    "questionnaire_version": "1.0"
}
```

**批量替换因子**：

```bash
PUT /api/v1/scales/SCALE-001/factors
Content-Type: application/json

{
    "factors": [
        {
            "code": "depression",
            "title": "抑郁症状",
            "factor_type": "primary",
            "is_total_score": false,
            "question_codes": ["Q1", "Q2", "Q3", "Q4"],
            "scoring_strategy": "sum",
            "interpret_rules": [
                {"min_score": 0, "max_score": 10, "risk_level": "none", "conclusion": "正常"},
                {"min_score": 10, "max_score": 20, "risk_level": "low", "conclusion": "轻度抑郁"}
            ]
        },
        {
            "code": "total",
            "title": "总分",
            "factor_type": "primary",
            "is_total_score": true,
            "question_codes": [],
            "scoring_strategy": "sum",
            "interpret_rules": [
                {"min_score": 0, "max_score": 50, "risk_level": "none", "conclusion": "无抑郁症状"},
                {"min_score": 50, "max_score": 70, "risk_level": "medium", "conclusion": "中度抑郁"}
            ]
        }
    ]
}
```

---

## 7. 数据持久化

### 7.1 MongoDB 文档结构

```javascript
// Collection: scales
{
    "_id": ObjectId("..."),
    "code": "SCALE-001",
    "title": "抑郁自评量表",
    "description": "SDS 量表",
    "questionnaire_code": "Q-SDS-001",
    "questionnaire_version": "1.0",
    "status": 1,  // 0=Draft, 1=Published, 2=Archived
    "factors": [
        {
            "code": "depression",
            "title": "抑郁症状",
            "factor_type": "primary",
            "is_total_score": false,
            "question_codes": ["Q1", "Q2", "Q3"],
            "scoring_strategy": "sum",
            "scoring_params": {},
            "interpret_rules": [
                {
                    "min_score": 0,
                    "max_score": 10,
                    "risk_level": "none",
                    "conclusion": "正常",
                    "suggestion": ""
                }
            ]
        }
    ],
    "created_at": ISODate("2025-01-28T00:00:00Z"),
    "updated_at": ISODate("2025-01-28T00:00:00Z"),
    "deleted_at": null
}
```

### 7.2 PO ↔ Domain 映射

```text
┌─────────────────┐                    ┌─────────────────┐
│  MedicalScale   │                    │    ScalePO      │
│  (Domain)       │◄───── Mapper ─────►│   (MongoDB)     │
├─────────────────┤                    ├─────────────────┤
│ id              │                    │ _id             │
│ scaleCode       │◄────────────────►  │ code            │
│ title           │◄────────────────►  │ title           │
│ status          │◄────────────────►  │ status (uint8)  │
│ factors []*Factor                    │ factors []FactorPO
└─────────────────┘                    └─────────────────┘
```

**代码链接**: [`infra/mongo/scale/mapper.go`](../../internal/apiserver/infra/mongo/scale/mapper.go)

---

## 8. 小结

Scale 子域采用经典的六边形架构，核心设计原则包括：

| 原则 | 应用 |
|------|------|
| **单一职责** | 领域服务按职责分离（Lifecycle/BaseInfo/FactorManager/Validator） |
| **依赖倒置** | Repository 接口定义在领域层，实现在基础设施层 |
| **按行为者分离** | 应用服务按用户角色拆分（管理员/编辑者/查询用户） |
| **Option 模式** | 灵活的聚合根构造，支持可选参数 |
| **值对象** | InterpretationRule 等无标识的不可变对象 |

这种设计确保了：

- **可维护性**：职责清晰，易于定位问题
- **可扩展性**：新增功能只需添加新服务
- **可测试性**：依赖注入，易于 Mock
- **灵活性**：存储技术可替换（MongoDB → PostgreSQL）
