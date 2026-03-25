# 11-06-02 Assessment 聚合设计

> **版本**：V2.0  
> **最后更新**：2025-11-29  
> **状态**：✅ 已实现并验证

---

## 📋 文档导航

**当前位置**：Assessment 聚合设计（你在这里）  
**前置阅读**：[11-06-01 Evaluation子域架构总览](./11-06-01-Evaluation子域架构总览.md)  
**后续阅读**：

- [11-06-03 Calculation计算策略设计](./11-06-03-Calculation计算策略设计.md)
- [11-06-04 Interpretation解读策略设计](./11-06-04-Interpretation解读策略设计.md)

---

## 🎯 核心设计思想（30秒速览）

> **如果只有30秒，你需要知道这些：**

```text
┌────────────────────────────────────────────────────────────┐
│  Assessment = 测评实例 = "一次具体的测评行为"             │
│                                                            │
│  它回答三个关键问题：                                      │
│    1. 谁在什么时候用什么量表做了测评？（元数据）          │
│    2. 测评处于什么状态？（状态机）                        │
│    3. 测评结果是什么？（得分 + 风险等级）                 │
└────────────────────────────────────────────────────────────┘

核心设计：
  ✓ 聚合根模式 - Assessment 管理 Score 实体，保证一致性
  ✓ 状态机模式 - 4个状态，单向流转，不可回退
  ✓ 建造者模式 - AssessmentCreator 验证并创建复杂对象
  ✓ 事件驱动 - 状态变更自动发布领域事件
  ✓ 引用对象 - 不直接持有其他聚合，用值对象引用
```

---

## 一、为什么需要 Assessment 聚合？（问题域）

### 1.1 我们要解决什么问题？

在测评业务中，**一次测评不是一个简单的数据记录**，而是一个**复杂的业务流程**：

```text
┌─────────────────────────────────────────────────────────────┐
│  业务场景：小明需要做一次心理健康测评                      │
│                                                             │
│  流程1：创建测评                                            │
│    - 小明（Testee）                                        │
│    - 使用"抑郁自评量表"（MedicalScale）                   │
│    - 基于"SDS问卷 v1.0"（Questionnaire）                  │
│    - 答卷ID: AS-20250129-001                               │
│    → 生成 Assessment: A-12345                              │
│                                                             │
│  流程2：提交测评                                            │
│    - 小明填写完20道题                                      │
│    - 点击"提交"                                            │
│    → Assessment 状态：Pending → Submitted                  │
│    → 发布事件：AssessmentCreated                           │
│                                                             │
│  流程3：异步评估（qs-worker）                              │
│    - 计算各因子得分                                        │
│    - 计算总分                                              │
│    - 生成解读                                              │
│    → Assessment 状态：Submitted → Interpreted              │
│    → 记录结果：TotalScore = 68, RiskLevel = Moderate       │
│                                                             │
│  流程4：查询结果                                            │
│    - 小明查看测评报告                                      │
│    - 管理员查看统计数据                                    │
│    → 基于 Assessment 查询历史、趋势、统计                  │
└─────────────────────────────────────────────────────────────┘
```

**问题拆解**：

| 问题 | 挑战 | Assessment 的解决方案 |
| ------ | ------ | --------------------- |
| 如何关联多个实体？ | 测评涉及受试者、问卷、答卷、量表 | **引用对象模式** - 使用值对象引用，不直接持有 |
| 如何管理复杂流程？ | 创建 → 提交 → 评估 → 完成/失败 | **状态机模式** - 4个状态，合法性校验 |
| 如何保证数据一致性？ | 测评 + 得分必须同时成功或失败 | **聚合模式** - Assessment 管理 Score 实体 |
| 如何触发后续流程？ | 提交后需要异步评估 | **事件驱动** - 发布 AssessmentCreated 事件 |
| 如何简化复杂创建？ | 需要验证多个依赖对象 | **建造者模式** - AssessmentCreator |

### 1.2 职责边界（我管什么，我不管什么）

```text
┌─────────────────────────────────────────────────────────────┐
│                   Assessment 聚合边界                        │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ✅ 我负责的（核心职责）：                                  │
│    1. 记录测评元数据（谁、问卷、答卷、量表、来源）         │
│    2. 管理测评状态（Pending → Submitted → Interpreted）   │
│    3. 记录评估结果（总分、风险等级）                       │
│    4. 管理得分实体（Score 生命周期）                       │
│    5. 发布领域事件（状态变更、评估完成）                   │
│                                                             │
│  ❌ 我不关心的（其他模块负责）：                            │
│    1. 如何计算分数 → Calculation 领域服务                  │
│    2. 如何解读结果 → Interpretation 领域服务               │
│    3. 如何生成报告 → Report 聚合                           │
│    4. 问卷/量表的内部结构 → Survey/Scale 子域              │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## 二、Assessment 聚合结构（是什么？）

### 2.1 聚合根：Assessment

**核心定位**：测评实例的聚合根，代表"一次具体的测评行为"

```go
// 代码位置：internal/apiserver/domain/evaluation/assessment/assessment.go
type Assessment struct {
    // ===== 核心标识 =====
    id    ID              // 测评唯一标识
    orgID int64           // 所属机构
    
    // ===== 关联实体引用（防腐层）=====
    testeeRef        testee.ID         // 受试者引用
    questionnaireRef QuestionnaireRef  // 问卷引用（Code + Version）
    answerSheetRef   AnswerSheetRef    // 答卷引用（ID）
    medicalScaleRef  *MedicalScaleRef  // 量表引用（可选）
    
    // ===== 业务来源 =====
    origin Origin  // adhoc | plan | screening
    
    // ===== 状态与结果 =====
    status     Status      // pending | submitted | interpreted | failed
    totalScore *float64    // 总分（评估后填充）
    riskLevel  *RiskLevel  // 风险等级（评估后填充）
    
    // ===== 时间戳 =====
    createdAt     time.Time    // 创建时间
    submittedAt   *time.Time   // 提交时间
    interpretedAt *time.Time   // 解读完成时间
    failedAt      *time.Time   // 失败时间
    
    // ===== 失败信息 =====
    failureReason *string
    
    // ===== 领域事件（未持久化）=====
    events []DomainEvent
}
```

**聚合结构图**：

```text
┌──────────────────────────────────────────────────────────┐
│         Assessment（聚合根）                             │
├──────────────────────────────────────────────────────────┤
│  核心属性：                                              │
│  - ID, OrgID                                            │
│  - Status（状态机）                                     │
│  - TotalScore, RiskLevel（评估结果）                   │
│                                                          │
│  引用对象（值对象）：                                    │
│  - TesteeRef                                            │
│  - QuestionnaireRef (Code + Version)                   │
│  - AnswerSheetRef (ID)                                 │
│  - MedicalScaleRef (Code, 可选)                        │
│                                                          │
│  聚合边界内的实体：                                      │
│  ┌────────────────────────────────────┐                │
│  │  Score（得分实体）                 │                │
│  │  - AssessmentID（外键）            │                │
│  │  - FactorCode（因子编码）          │                │
│  │  - Score（得分值）                 │                │
│  │  - RiskLevel（风险等级）           │                │
│  └────────────────────────────────────┘                │
│                                                          │
│  领域事件：                                              │
│  - AssessmentCreatedEvent                               │
│  - EvaluationCompletedEvent                             │
│  - EvaluationFailedEvent                                │
└──────────────────────────────────────────────────────────┘
```

### 2.2 实体：Score（得分）

**核心定位**：记录评估的具体得分（因子得分或总分）

```go
// 代码位置：internal/apiserver/domain/evaluation/assessment/score.go
type Score struct {
    id           ID            // 得分ID
    assessmentID AssessmentID  // 所属测评
    
    // 因子信息（nil 表示总分）
    factorCode *string  // 因子编码（如 "F1", "F2"）
    factorName *string  // 因子名称（如 "认知因子"）
    
    // 得分信息
    score      float64    // 得分值
    percentage *float64   // 百分比（相对于满分）
    
    // 解读信息
    level          *string  // 风险等级（low/moderate/high）
    interpretation *string  // 解读文本
    
    // 时间戳
    createdAt time.Time
}
```

**Score 与 Assessment 的关系**：

```text
┌────────────────────────────────────────────────────────┐
│  Assessment (1) ───管理───> (N) Score                 │
├────────────────────────────────────────────────────────┤
│                                                        │
│  聚合边界：Score 只能通过 Assessment 访问             │
│  生命周期：Score 随 Assessment 创建和销毁             │
│  事务边界：Assessment + Scores 在同一事务中持久化     │
│                                                        │
│  示例：                                                │
│    Assessment ID: A-12345                             │
│      ├─ Score: FactorCode=null (总分)                │
│      ├─ Score: FactorCode="F1" (认知因子)            │
│      ├─ Score: FactorCode="F2" (情感因子)            │
│      └─ Score: FactorCode="F3" (躯体因子)            │
└────────────────────────────────────────────────────────┘
```

### 2.3 值对象：引用对象（Reference Objects）

**为什么使用引用对象？**

```text
┌──────────────────────────────────────────────────────────┐
│  问题：如何引用其他子域的聚合根？                        │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  ❌ 错误做法：直接持有聚合根                             │
│                                                          │
│    type Assessment struct {                             │
│        questionnaire *Questionnaire  // 直接依赖        │
│        answerSheet   *AnswerSheet    // 强耦合          │
│    }                                                     │
│                                                          │
│  问题：                                                  │
│    - 跨聚合耦合严重                                     │
│    - 生命周期混乱                                       │
│    - 事务边界不清                                       │
│    - 加载性能差                                         │
│                                                          │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  ✅ 正确做法：使用引用对象（值对象）                     │
│                                                          │
│    type Assessment struct {                             │
│        questionnaireRef QuestionnaireRef  // 值对象引用 │
│        answerSheetRef   AnswerSheetRef    // 轻量级     │
│    }                                                     │
│                                                          │
│    type QuestionnaireRef struct {                       │
│        code    string  // 问卷编码                      │
│        version string  // 问卷版本                      │
│    }                                                     │
│                                                          │
│  优势：                                                  │
│    ✓ 解耦：只存储必要的引用信息                         │
│    ✓ 轻量：易于序列化和传输                             │
│    ✓ 独立：可以独立验证有效性                           │
│    ✓ 清晰：明确跨聚合边界                               │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

**引用对象定义**：

```go
// 代码位置：internal/apiserver/domain/evaluation/assessment/types.go

// QuestionnaireRef 问卷引用
type QuestionnaireRef struct {
    code    string  // 问卷编码（如 "SDS"）
    version string  // 问卷版本（如 "v1.0"）
}

// AnswerSheetRef 答卷引用
type AnswerSheetRef struct {
    id string  // 答卷ID（MongoDB ObjectID）
}

// MedicalScaleRef 量表引用
type MedicalScaleRef struct {
    code string  // 量表编码（如 "SDS"）
}
```

### 2.4 值对象：Origin（测评来源）

**核心定位**：记录测评的业务来源

```go
// 代码位置：internal/apiserver/domain/evaluation/assessment/types.go

// OriginType 来源类型
type OriginType string

const (
    OriginAdhoc     OriginType = "adhoc"      // 一次性测评（手动创建）
    OriginPlan      OriginType = "plan"       // 测评计划
    OriginScreening OriginType = "screening"  // 入校筛查
)

// Origin 测评来源值对象
type Origin struct {
    originType OriginType  // 来源类型
    refID      *string     // 关联ID（plan/screening 时必填）
}

// 工厂方法
func NewAdhocOrigin() Origin {
    return Origin{originType: OriginAdhoc}
}

func NewPlanOrigin(planID string) Origin {
    return Origin{originType: OriginPlan, refID: &planID}
}

func NewScreeningOrigin(screeningProjectID string) Origin {
    return Origin{originType: OriginScreening, refID: &screeningProjectID}
}
```

**Origin 的作用**：

```text
┌──────────────────────────────────────────────────────────┐
│  Origin 的三种类型及其用途                               │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  1. Adhoc（一次性测评）                                 │
│     场景：管理员手动为某用户创建测评                    │
│     特点：不关联任何计划或筛查                          │
│     示例：Origin{type: "adhoc"}                         │
│                                                          │
│  2. Plan（测评计划）                                    │
│     场景：患者出院后周期性随访测评                      │
│     特点：关联 AssessmentPlan，可能重复执行             │
│     示例：Origin{type: "plan", refID: "PLAN-001"}      │
│                                                          │
│  3. Screening（入校筛查）                               │
│     场景：新生入学心理健康筛查                          │
│     特点：关联 ScreeningProject，批量执行               │
│     示例：Origin{type: "screening", refID: "SCR-001"}  │
│                                                          │
│  作用：                                                  │
│    - 统计分析：按来源统计测评数量                       │
│    - 业务追溯：回溯测评的业务场景                       │
│    - 权限控制：不同来源有不同的访问权限                 │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

---

## 三、Assessment 状态机（如何运转？）

### 3.1 状态机全景图

```text
┌────────────────────────────────────────────────────────────────┐
│  Assessment 生命周期状态机                                     │
└────────────────────────────────────────────────────────────────┘

        ┌───────────┐
   创建 │  Pending  │  待提交（初始状态）
        └─────┬─────┘
              │
              │ Submit()
              │ - 验证答卷完整性
              │ - 发布 AssessmentCreatedEvent
              ↓
        ┌───────────┐
        │ Submitted │  已提交（等待评估）
        └─────┬─────┘
              │
              │ qs-worker 消费事件
              │ 调用 EvaluationService.Evaluate()
              ↓
        ┌────────────┐
        │ Evaluating │  评估中（瞬态，可选）
        └──┬──────┬──┘
           │      │
           │      │ MarkAsFailed(reason)
           │      │ - 记录失败原因
           │      │ - 发布 EvaluationFailedEvent
           │      ↓
           │  ┌────────┐
           │  │ Failed │  失败（终态）
           │  └────────┘
           │
           │ ApplyEvaluation(result)
           │ - 记录总分和风险等级
           │ - 发布 EvaluationCompletedEvent
           ↓
    ┌──────────────┐
    │ Interpreted  │  已解读（终态）
    └──────────────┘
```

**状态定义**：

```go
// 代码位置：internal/apiserver/domain/evaluation/assessment/types.go

type Status string

const (
    StatusPending     Status = "pending"      // 待提交
    StatusSubmitted   Status = "submitted"    // 已提交
    StatusInterpreted Status = "interpreted"  // 已解读
    StatusFailed      Status = "failed"       // 失败
)

// 状态判断方法
func (s Status) IsPending() bool     { return s == StatusPending }
func (s Status) IsSubmitted() bool   { return s == StatusSubmitted }
func (s Status) IsInterpreted() bool { return s == StatusInterpreted }
func (s Status) IsFailed() bool      { return s == StatusFailed }
func (s Status) IsTerminal() bool    { return s.IsInterpreted() || s.IsFailed() }
```

### 3.2 状态转换方法（领域逻辑）

#### 3.2.1 Submit() - 提交测评

```go
// 代码位置：internal/apiserver/domain/evaluation/assessment/assessment.go

func (a *Assessment) Submit() error {
    // 1. 状态校验
    if !a.status.IsPending() {
        return ErrInvalidStatusTransition
    }
    
    // 2. 状态迁移
    a.status = StatusSubmitted
    now := time.Now()
    a.submittedAt = &now
    
    // 3. 发布领域事件
    event := NewAssessmentCreatedEvent(
        a.id,
        a.testeeRef,
        a.questionnaireRef,
        a.answerSheetRef,
        a.medicalScaleRef,
        a.origin,
        now,
    )
    a.publishEvent(event)
    
    return nil
}
```

**流程图**：

```text
┌─────────────────────────────────────────────────────┐
│  Submit() 执行流程                                  │
├─────────────────────────────────────────────────────┤
│                                                     │
│  1. 检查前置条件                                    │
│     ├─ 状态必须是 Pending                          │
│     └─ 如果不是 → 返回错误                         │
│                                                     │
│  2. 状态迁移                                        │
│     ├─ status: Pending → Submitted                 │
│     └─ submittedAt: time.Now()                     │
│                                                     │
│  3. 发布领域事件                                    │
│     └─ AssessmentCreatedEvent                      │
│         ├─ EventType: "assessment.created"         │
│         ├─ AssessmentID                            │
│         ├─ TesteeID                                │
│         └─ ... (其他元数据)                        │
│                                                     │
│  4. 返回成功                                        │
│                                                     │
└─────────────────────────────────────────────────────┘
```

#### 3.2.2 ApplyEvaluation() - 应用评估结果

```go
func (a *Assessment) ApplyEvaluation(result *EvaluationResult) error {
    // 1. 状态校验
    if !a.status.IsSubmitted() {
        return ErrInvalidStatusTransition
    }
    
    // 2. 验证评估结果
    if result == nil || result.TotalScore < 0 {
        return ErrInvalidEvaluationResult
    }
    
    // 3. 记录评估结果
    a.totalScore = &result.TotalScore
    a.riskLevel = &result.RiskLevel
    
    // 4. 状态迁移
    a.status = StatusInterpreted
    now := time.Now()
    a.interpretedAt = &now
    
    // 5. 发布领域事件
    event := NewEvaluationCompletedEvent(
        a.id,
        result.TotalScore,
        result.RiskLevel,
        now,
    )
    a.publishEvent(event)
    
    return nil
}
```

#### 3.2.3 MarkAsFailed() - 标记失败

```go
func (a *Assessment) MarkAsFailed(reason string) {
    // 1. 记录失败信息
    a.status = StatusFailed
    a.failureReason = &reason
    now := time.Now()
    a.failedAt = &now
    
    // 2. 发布领域事件
    event := NewEvaluationFailedEvent(
        a.id,
        reason,
        now,
    )
    a.publishEvent(event)
}
```

### 3.3 状态转换合法性校验

```text
┌─────────────────────────────────────────────────────────────┐
│  状态转换规则（状态机不变性）                               │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ✅ 合法转换：                                              │
│     Pending     → Submitted     (Submit)                   │
│     Submitted   → Interpreted   (ApplyEvaluation)          │
│     Submitted   → Failed        (MarkAsFailed)             │
│                                                             │
│  ❌ 非法转换（代码层面禁止）：                              │
│     Interpreted → Pending       (已完成不能回退)           │
│     Failed      → Submitted     (失败不能重新提交)         │
│     Pending     → Interpreted   (必须经过 Submitted)       │
│                                                             │
│  终态判断：                                                 │
│     Interpreted - 终态（成功完成）                         │
│     Failed      - 终态（失败结束）                         │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## 四、AssessmentCreator（建造者模式）

### 4.1 为什么需要 Creator？

**问题场景**：创建 Assessment 需要验证多个依赖对象

```text
┌──────────────────────────────────────────────────────────┐
│  创建 Assessment 的挑战                                  │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  需要验证：                                              │
│    1. Testee 是否存在？                                 │
│    2. Questionnaire 是否存在且已发布？                  │
│    3. AnswerSheet 是否存在且属于该问卷？                │
│    4. MedicalScale 是否存在且与问卷关联？（可选）       │
│                                                          │
│  如果不用 Builder：                                      │
│    ❌ 验证逻辑分散在应用服务层（逻辑泄漏）              │
│    ❌ 每个调用方都要重复验证（代码重复）                │
│    ❌ 验证失败时，部分对象已创建（不一致）              │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

**建造者模式解决方案**：

```text
┌──────────────────────────────────────────────────────────┐
│  AssessmentCreator（建造者）                             │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  职责：                                                  │
│    1. 验证所有依赖对象（跨聚合验证）                    │
│    2. 创建 Assessment 聚合根                            │
│    3. 自动提交（调用 Submit）                           │
│    4. 返回完整的 Assessment 对象                        │
│                                                          │
│  优势：                                                  │
│    ✓ 验证逻辑封装在领域层                               │
│    ✓ 创建过程可控（分步验证）                           │
│    ✓ 易于测试（Mock 验证器）                            │
│    ✓ 原子性保证（全部成功或全部失败）                   │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

### 4.2 Creator 接口设计

```go
// 代码位置：internal/apiserver/domain/evaluation/assessment/creator.go

// AssessmentCreator 测评创建器接口（领域服务）
type AssessmentCreator interface {
    // Create 创建测评
    // - 验证所有依赖对象
    // - 创建 Assessment 聚合根
    // - 自动提交（状态 → Submitted）
    // - 发布 AssessmentCreatedEvent
    Create(ctx context.Context, req CreateAssessmentRequest) (*Assessment, error)
}

// CreateAssessmentRequest 创建请求
type CreateAssessmentRequest struct {
    // 必填字段
    OrgID            int64
    TesteeID         testee.ID
    QuestionnaireRef QuestionnaireRef
    AnswerSheetRef   AnswerSheetRef
    
    // 来源信息
    Origin Origin
    
    // 可选字段
    MedicalScaleRef *MedicalScaleRef
    
    // 是否自动提交（默认 true）
    AutoSubmit bool
}
```

### 4.3 Creator 实现

```go
// DefaultAssessmentCreator 默认实现
type DefaultAssessmentCreator struct {
    testeeValidator        TesteeValidator
    questionnaireValidator QuestionnaireValidator
    answerSheetValidator   AnswerSheetValidator
    scaleValidator         ScaleValidator
}

// Create 创建测评
func (c *DefaultAssessmentCreator) Create(
    ctx context.Context,
    req CreateAssessmentRequest,
) (*Assessment, error) {
    
    // ========== 步骤1: 验证受试者 ==========
    exists, err := c.testeeValidator.Exists(ctx, req.TesteeID)
    if err != nil {
        return nil, fmt.Errorf("验证受试者失败: %w", err)
    }
    if !exists {
        return nil, ErrTesteeNotFound
    }
    
    // ========== 步骤2: 验证问卷 ==========
    exists, err = c.questionnaireValidator.Exists(ctx, req.QuestionnaireRef)
    if err != nil {
        return nil, fmt.Errorf("验证问卷失败: %w", err)
    }
    if !exists {
        return nil, ErrQuestionnaireNotFound
    }
    
    // 检查问卷是否已发布
    published, err := c.questionnaireValidator.IsPublished(ctx, req.QuestionnaireRef)
    if err != nil {
        return nil, fmt.Errorf("检查问卷状态失败: %w", err)
    }
    if !published {
        return nil, ErrQuestionnaireNotPublished
    }
    
    // ========== 步骤3: 验证答卷 ==========
    exists, err = c.answerSheetValidator.Exists(ctx, req.AnswerSheetRef)
    if err != nil {
        return nil, fmt.Errorf("验证答卷失败: %w", err)
    }
    if !exists {
        return nil, ErrAnswerSheetNotFound
    }
    
    // 检查答卷是否属于该问卷
    belongs, err := c.answerSheetValidator.BelongsToQuestionnaire(
        ctx, 
        req.AnswerSheetRef, 
        req.QuestionnaireRef,
    )
    if err != nil {
        return nil, fmt.Errorf("检查答卷归属失败: %w", err)
    }
    if !belongs {
        return nil, ErrAnswerSheetNotBelongsToQuestionnaire
    }
    
    // ========== 步骤4: 验证量表（可选）==========
    if req.MedicalScaleRef != nil {
        exists, err = c.scaleValidator.Exists(ctx, *req.MedicalScaleRef)
        if err != nil {
            return nil, fmt.Errorf("验证量表失败: %w", err)
        }
        if !exists {
            return nil, ErrMedicalScaleNotFound
        }
        
        // 检查量表是否与问卷关联
        linked, err := c.scaleValidator.IsLinkedToQuestionnaire(
            ctx,
            *req.MedicalScaleRef,
            req.QuestionnaireRef,
        )
        if err != nil {
            return nil, fmt.Errorf("检查量表关联失败: %w", err)
        }
        if !linked {
            return nil, ErrMedicalScaleNotLinkedToQuestionnaire
        }
    }
    
    // ========== 步骤5: 创建 Assessment ==========
    assess, err := NewAssessment(
        req.OrgID,
        req.TesteeID,
        req.QuestionnaireRef,
        req.AnswerSheetRef,
        req.Origin,
        WithMedicalScale(req.MedicalScaleRef),
    )
    if err != nil {
        return nil, fmt.Errorf("创建测评失败: %w", err)
    }
    
    // ========== 步骤6: 自动提交（可选）==========
    if req.AutoSubmit {
        if err := assess.Submit(); err != nil {
            return nil, fmt.Errorf("提交测评失败: %w", err)
        }
    }
    
    return assess, nil
}
```

**创建流程图**：

```text
┌────────────────────────────────────────────────────────────┐
│  AssessmentCreator.Create() 执行流程                       │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  1. 验证 Testee                                            │
│     ├─ 调用 TesteeValidator.Exists()                      │
│     └─ 不存在 → 返回 ErrTesteeNotFound                    │
│                                                            │
│  2. 验证 Questionnaire                                     │
│     ├─ 调用 QuestionnaireValidator.Exists()               │
│     ├─ 调用 QuestionnaireValidator.IsPublished()          │
│     └─ 未发布 → 返回 ErrQuestionnaireNotPublished         │
│                                                            │
│  3. 验证 AnswerSheet                                       │
│     ├─ 调用 AnswerSheetValidator.Exists()                 │
│     ├─ 调用 BelongsToQuestionnaire()                      │
│     └─ 不匹配 → 返回 ErrNotBelongs                        │
│                                                            │
│  4. 验证 MedicalScale（可选）                              │
│     ├─ 调用 ScaleValidator.Exists()                       │
│     ├─ 调用 IsLinkedToQuestionnaire()                     │
│     └─ 未关联 → 返回 ErrNotLinked                         │
│                                                            │
│  5. 创建 Assessment                                        │
│     ├─ NewAssessment(...)                                 │
│     └─ 状态：Pending                                      │
│                                                            │
│  6. 自动提交（AutoSubmit=true）                           │
│     ├─ assess.Submit()                                    │
│     ├─ 状态：Pending → Submitted                          │
│     └─ 发布：AssessmentCreatedEvent                       │
│                                                            │
│  7. 返回 Assessment                                        │
│                                                            │
└────────────────────────────────────────────────────────────┘
```

---

## 五、领域事件设计

### 5.1 事件接口

```go
// 代码位置：internal/apiserver/domain/evaluation/assessment/events.go

// DomainEvent 领域事件接口
type DomainEvent interface {
    EventID() string      // 事件唯一标识
    EventType() string    // 事件类型
    OccurredAt() time.Time // 事件发生时间
    AggregateID() ID      // 聚合根ID
}
```

### 5.2 核心事件

#### 5.2.1 AssessmentCreatedEvent - 测评已创建

**触发时机**：Assessment.Submit() 被调用时

```go
type AssessmentCreatedEvent struct {
    baseEvent
    assessmentID     ID
    testeeID         testee.ID
    questionnaireRef QuestionnaireRef
    answerSheetRef   AnswerSheetRef
    medicalScaleRef  *MedicalScaleRef
    origin           Origin
    submittedAt      time.Time
}

// NewAssessmentCreatedEvent 创建事件
func NewAssessmentCreatedEvent(
    assessmentID ID,
    testeeID testee.ID,
    questionnaireRef QuestionnaireRef,
    answerSheetRef AnswerSheetRef,
    medicalScaleRef *MedicalScaleRef,
    origin Origin,
    submittedAt time.Time,
) *AssessmentCreatedEvent {
    return &AssessmentCreatedEvent{
        baseEvent:        newBaseEvent("assessment.created"),
        assessmentID:     assessmentID,
        testeeID:         testeeID,
        questionnaireRef: questionnaireRef,
        answerSheetRef:   answerSheetRef,
        medicalScaleRef:  medicalScaleRef,
        origin:           origin,
        submittedAt:      submittedAt,
    }
}
```

**用途**：

- qs-worker 消费此事件，触发评估流程
- 通知服务消费此事件，发送"答卷已提交"通知
- 统计服务消费此事件，更新实时统计数据

#### 5.2.2 EvaluationCompletedEvent - 评估已完成

**触发时机**：Assessment.ApplyEvaluation() 被调用时

```go
type EvaluationCompletedEvent struct {
    baseEvent
    assessmentID  ID
    totalScore    float64
    riskLevel     RiskLevel
    completedAt   time.Time
}
```

**用途**：

- 通知服务消费此事件，发送"评估完成"通知
- 统计服务消费此事件，更新统计图表
- 触发后续业务流程（如高风险预警）

#### 5.2.3 EvaluationFailedEvent - 评估失败

**触发时机**：Assessment.MarkAsFailed() 被调用时

```go
type EvaluationFailedEvent struct {
    baseEvent
    assessmentID ID
    reason       string
    failedAt     time.Time
}
```

**用途**：

- 告警服务消费此事件，发送告警通知
- 日志服务记录失败原因
- 重试机制判断是否可以重试

### 5.3 事件流转图

```text
┌────────────────────────────────────────────────────────────┐
│  事件流转全景                                              │
└────────────────────────────────────────────────────────────┘

  用户操作            Assessment          EventBus         订阅者
     │                    │                  │                │
     │ 1. 提交答卷        │                  │                │
     ├──────────────────→│                  │                │
     │                    │                  │                │
     │                    │ 2. Submit()      │                │
     │                    │ - 状态迁移       │                │
     │                    │ - 发布事件       │                │
     │                    ├─────────────────→│                │
     │                    │ AssessmentCreated│                │
     │                    │                  │                │
     │ 3. 立即返回        │                  │ 4. 异步消费    │
     │←──────────────────│                  ├───────────────→│
     │                    │                  │                qs-worker
     │                    │                  │                │
     │                    │                  │ 5. Evaluate()  │
     │                    │                  │                │
     │                    │ 6. ApplyEvaluation()              │
     │                    │←──────────────────────────────────│
     │                    │                  │                │
     │                    │ 7. 发布事件      │                │
     │                    ├─────────────────→│                │
     │                    │ EvaluationCompleted               │
     │                    │                  │                │
     │                    │                  │ 8. 异步消费    │
     │                    │                  ├───────────────→│
     │                    │                  │           通知服务
     │                    │                  │           统计服务
     │                    │                  │                │
```

---

## 六、数据持久化设计

### 6.1 MySQL 表结构

#### assessments 表

```sql
CREATE TABLE assessments (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    org_id BIGINT UNSIGNED NOT NULL COMMENT '机构ID',
    
    -- 关联引用
    testee_id BIGINT UNSIGNED NOT NULL COMMENT '受试者ID',
    questionnaire_code VARCHAR(50) NOT NULL COMMENT '问卷编码',
    questionnaire_version VARCHAR(20) NOT NULL COMMENT '问卷版本',
    answer_sheet_id VARCHAR(50) NOT NULL COMMENT '答卷ID',
    medical_scale_code VARCHAR(50) COMMENT '量表编码（可选）',
    
    -- 业务来源
    origin_type VARCHAR(20) NOT NULL COMMENT '来源类型：adhoc/plan/screening',
    origin_ref_id VARCHAR(50) COMMENT '来源关联ID',
    
    -- 状态与结果
    status VARCHAR(20) NOT NULL COMMENT '状态：pending/submitted/interpreted/failed',
    total_score DECIMAL(10,2) COMMENT '总分',
    risk_level VARCHAR(20) COMMENT '风险等级：low/moderate/high',
    
    -- 时间戳
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    submitted_at TIMESTAMP NULL COMMENT '提交时间',
    interpreted_at TIMESTAMP NULL COMMENT '解读完成时间',
    failed_at TIMESTAMP NULL COMMENT '失败时间',
    
    -- 失败信息
    failure_reason TEXT COMMENT '失败原因',
    
    -- 索引
    INDEX idx_testee_status (testee_id, status),
    INDEX idx_origin (origin_type, origin_ref_id),
    INDEX idx_status (status),
    INDEX idx_created_at (created_at),
    INDEX idx_scale (medical_scale_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='测评表';
```

#### assessment_scores 表

```sql
CREATE TABLE assessment_scores (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    assessment_id BIGINT UNSIGNED NOT NULL COMMENT '测评ID',
    
    -- 因子信息（NULL 表示总分）
    factor_code VARCHAR(50) COMMENT '因子编码',
    factor_name VARCHAR(100) COMMENT '因子名称',
    
    -- 得分
    score DECIMAL(10,2) NOT NULL COMMENT '得分值',
    percentage DECIMAL(5,2) COMMENT '百分比',
    
    -- 解读
    level VARCHAR(50) COMMENT '风险等级',
    interpretation TEXT COMMENT '解读文本',
    
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    -- 索引
    FOREIGN KEY (assessment_id) REFERENCES assessments(id) ON DELETE CASCADE,
    INDEX idx_assessment (assessment_id),
    INDEX idx_factor (factor_code),
    UNIQUE KEY uk_assessment_factor (assessment_id, factor_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='测评得分表';
```

### 6.2 仓储接口

```go
// 代码位置：internal/apiserver/domain/evaluation/assessment/repository.go

// Repository Assessment 仓储接口
type Repository interface {
    // Save 保存测评（创建或更新）
    Save(ctx context.Context, assessment *Assessment) error
    
    // FindByID 根据ID查询
    FindByID(ctx context.Context, id ID) (*Assessment, error)
    
    // FindByTestee 查询受试者的测评列表
    FindByTestee(ctx context.Context, testeeID testee.ID, opts ...QueryOption) ([]*Assessment, error)
    
    // FindByOrigin 根据来源查询
    FindByOrigin(ctx context.Context, origin Origin) ([]*Assessment, error)
    
    // CountByStatus 按状态统计数量
    CountByStatus(ctx context.Context, status Status) (int64, error)
}

// ScoreRepository Score 仓储接口
type ScoreRepository interface {
    // SaveBatch 批量保存得分
    SaveBatch(ctx context.Context, scores []*Score) error
    
    // FindByAssessment 查询测评的所有得分
    FindByAssessment(ctx context.Context, assessmentID ID) ([]*Score, error)
    
    // FindTotalScore 查询总分
    FindTotalScore(ctx context.Context, assessmentID ID) (*Score, error)
}
```

---

## 七、使用示例

### 7.1 创建并提交测评

```go
// 1. 创建 AssessmentCreator
creator := assessment.NewDefaultAssessmentCreator(
    testeeValidator,
    questionnaireValidator,
    answerSheetValidator,
    scaleValidator,
)

// 2. 构建创建请求
req := assessment.NewCreateAssessmentRequest(
    orgID,
    testeeID,
    questionnaireRef,
    answerSheetRef,
    assessment.NewAdhocOrigin(),
).WithMedicalScale(medicalScaleRef)

// 3. 创建测评（自动提交）
assess, err := creator.Create(ctx, req)
if err != nil {
    return err
}

// 4. 持久化
if err := assessmentRepo.Save(ctx, assess); err != nil {
    return err
}

// 5. 发布领域事件
events := assess.GetEvents()
for _, event := range events {
    eventBus.Publish(ctx, event)
}

// assess.Status() == StatusSubmitted
// assess.SubmittedAt() != nil
```

### 7.2 应用评估结果

```go
// 1. 加载 Assessment
assess, err := assessmentRepo.FindByID(ctx, assessmentID)
if err != nil {
    return err
}

// 2. 检查状态
if !assess.Status().IsSubmitted() {
    return errors.New("测评状态不正确")
}

// 3. 构建评估结果
result := &assessment.EvaluationResult{
    TotalScore: 68.5,
    RiskLevel:  assessment.RiskLevelModerate,
    FactorScores: []assessment.FactorScore{
        {FactorCode: "F1", Score: 32.0},
        {FactorCode: "F2", Score: 28.5},
        {FactorCode: "F3", Score: 8.0},
    },
}

// 4. 应用评估结果
if err := assess.ApplyEvaluation(result); err != nil {
    return err
}

// 5. 保存得分
scores := buildScoresFromResult(assess.ID(), result)
if err := scoreRepo.SaveBatch(ctx, scores); err != nil {
    return err
}

// 6. 更新 Assessment
if err := assessmentRepo.Save(ctx, assess); err != nil {
    return err
}

// 7. 发布事件
events := assess.GetEvents()
for _, event := range events {
    eventBus.Publish(ctx, event)
}

// assess.Status() == StatusInterpreted
// assess.TotalScore() == 68.5
```

### 7.3 处理评估失败

```go
// 1. 加载 Assessment
assess, err := assessmentRepo.FindByID(ctx, assessmentID)
if err != nil {
    return err
}

// 2. 标记失败
assess.MarkAsFailed("计算策略执行失败: division by zero")

// 3. 持久化
if err := assessmentRepo.Save(ctx, assess); err != nil {
    return err
}

// 4. 发布事件
events := assess.GetEvents()
for _, event := range events {
    eventBus.Publish(ctx, event)
}

// assess.Status() == StatusFailed
// assess.FailureReason() == "计算策略执行失败: division by zero"
```

---

## 八、设计模式总结

### 8.1 聚合模式

**应用**：Assessment 管理 Score 实体

**优势**：

- ✅ 一致性边界：Assessment + Scores 在同一事务
- ✅ 生命周期管理：Score 随 Assessment 创建和销毁
- ✅ 访问控制：外部只能通过 Assessment 访问 Score

### 8.2 状态机模式

**应用**：Assessment 的状态管理

**优势**：

- ✅ 状态约束：代码层面保证状态迁移合法性
- ✅ 领域语义：状态迁移方法反映业务术语
- ✅ 可追溯：时间戳记录每次状态变更

### 8.3 建造者模式

**应用**：AssessmentCreator

**优势**：

- ✅ 封装复杂性：验证逻辑集中在 Creator
- ✅ 步骤可控：分步验证，任一步骤失败即停止
- ✅ 易于测试：便于 Mock 各个验证器

### 8.4 事件驱动模式

**应用**：领域事件发布

**优势**：

- ✅ 异步解耦：评估流程不阻塞主流程
- ✅ 可扩展：新增订阅者不影响核心逻辑
- ✅ 可追溯：事件记录完整的操作历史

---

## 九、关键设计决策

### 9.1 为什么使用引用对象而非直接持有聚合根？

**原因**：

1. **解耦**：不依赖其他子域的聚合根结构
2. **轻量**：只存储必要的引用信息，减少内存占用
3. **独立**：可以独立验证引用的有效性
4. **清晰**：明确跨聚合边界，避免事务膨胀

### 9.2 为什么 Score 是实体而非值对象？

**原因**：

1. **有身份**：每个 Score 有独立的 ID
2. **可变**：得分的解读信息可能后续更新
3. **生命周期**：Score 的创建和删除由 Assessment 管理

### 9.3 为什么状态机只能单向流转？

**原因**：

1. **业务不变性**：测评一旦完成，不应该回退
2. **数据一致性**：避免状态回退导致的数据不一致
3. **审计追溯**：单向流转更易于追溯操作历史

---

## 十、后续文档预告

| 文档编号 | 标题 | 核心内容 |
| --------- | ------ | --------- |
| **11-06-03** | Calculation 计算策略设计 | 8种计算策略、注册器、扩展示例 |
| **11-06-04** | Interpretation 解读策略设计 | 阈值策略、复合策略、高危识别 |
| **11-06-05** | Report 聚合设计 | 报告结构、Builder、Exporter |
| **11-06-06** | 应用服务层设计 | EvaluationService、QueryService |

---

## 十一、相关代码链接

- **聚合根**：[`assessment/assessment.go`](../../../internal/apiserver/domain/evaluation/assessment/assessment.go)
- **Score 实体**：[`assessment/score.go`](../../../internal/apiserver/domain/evaluation/assessment/score.go)
- **值对象**：[`assessment/types.go`](../../../internal/apiserver/domain/evaluation/assessment/types.go)
- **领域事件**：[`assessment/events.go`](../../../internal/apiserver/domain/evaluation/assessment/events.go)
- **创建器**：[`assessment/creator.go`](../../../internal/apiserver/domain/evaluation/assessment/creator.go)
- **仓储接口**：[`assessment/repository.go`](../../../internal/apiserver/domain/evaluation/assessment/repository.go)

---

> **作者**：Evaluation 子域设计团队  
> **审阅**：领域架构师  
> **版本历史**：
>
> - V2.0 (2025-11-29)：采用新的写作风格，增加30秒速览和问题域驱动
> - V1.0 (2025-01-29)：初始版本
