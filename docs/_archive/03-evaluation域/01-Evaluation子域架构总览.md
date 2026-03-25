# 11-06-01 Evaluation 子域架构总览

> **版本**：V2.0  
> **最后更新**：2025-11-29  
> **状态**：✅ 已实现并验证

---

## 📋 文档导航

**当前位置**：架构总览（你在这里）  
**阅读顺序**：

1. 👉 **本文档** - 先理解整体设计思想和运行机制
2. [11-06-02-Assessment聚合设计](./11-06-02-Assessment聚合设计.md) - 再深入聚合根实现
3. [11-06-03-计算策略设计](./11-06-03-Calculation计算策略设计.md) - 学习策略模式应用
4. [11-06-04-解读策略设计](./11-06-04-Interpretation解读策略设计.md) - 理解规则引擎设计
5. [11-06-05-报告构建设计](./11-06-05-Report聚合设计.md) - 掌握报告生成机制

---

## 🎯 核心设计思想（30秒速览）

> **如果只有30秒，你需要知道这些：**

```text
┌────────────────────────────────────────────────────────────┐
│  Evaluation 子域 = 评估引擎                                │
│                                                            │
│  输入：答卷数据 + 量表规则                                 │
│   ↓                                                        │
│  处理：计算 → 解读 → 报告                                 │
│   ↓                                                        │
│  输出：结构化测评报告                                      │
└────────────────────────────────────────────────────────────┘

核心设计模式：
  ✓ 聚合模式 - Assessment 管流程，Report 管结果
  ✓ 策略模式 - 8种计算策略 + 多种解读策略，可插拔
  ✓ 事件驱动 - 异步评估，解耦业务流程
  ✓ 建造者模式 - 复杂对象创建过程清晰可控
```

---

## 一、为什么需要 Evaluation 子域？（问题域）

### 1.1 我们要解决什么问题？

在心理测评业务中，**收集答案只是第一步**，真正的价值在于**从答案中提取洞察**：

```text
┌─────────────────────────────────────────────────────────────┐
│  业务场景：小明完成了"抑郁自评量表"的20道题              │
│                                                             │
│  原始数据（无意义）：                                       │
│    Q1: 选了 B (2分)                                        │
│    Q2: 选了 C (3分)                                        │
│    ... 共20题                                              │
│                                                             │
│  期望结果（有意义）：                                       │
│    总分：68 分                                             │
│    风险等级：中度风险                                      │
│    结论：存在明显抑郁倾向，建议寻求专业帮助               │
│    维度分析：                                               │
│      - 认知因子：32分（高风险）                           │
│      - 情感因子：28分（中风险）                           │
│      - 躯体因子：8分（低风险）                            │
└─────────────────────────────────────────────────────────────┘
```

**问题拆解**：

| 问题 | 挑战 | Evaluation 的解决方案 |
| ------ | ------ | --------------------- |
| 如何计分？ | 不同量表有不同算法（求和、加权、平均...） | **策略模式** - 8种可扩展计算策略 |
| 如何解读？ | 分数需要转换为有意义的结论 | **策略模式** - 阈值、组合等解读策略 |
| 如何管理流程？ | 评估过程复杂（提交→计算→解读→报告） | **聚合模式** - Assessment 管理状态机 |
| 如何存储结果？ | 结构化得分 + 灵活的报告文档 | **混合存储** - MySQL + MongoDB |
| 如何解耦业务？ | 评估耗时，不能阻塞主流程 | **事件驱动** - 异步评估 |

### 1.2 职责边界（我管什么，我不管什么）

```text
┌─────────────────────────────────────────────────────────────┐
│                   Evaluation 子域边界                        │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ✅ 我负责的（核心职责）：                                  │
│    1. 管理评估流程（Pending → Submitted → Interpreted）   │
│    2. 执行计分计算（调度计算策略）                         │
│    3. 执行结果解读（调度解读策略）                         │
│    4. 生成测评报告（结构化存储 + 导出）                   │
│                                                             │
│  ❌ 我不关心的（其他子域负责）：                            │
│    1. 问卷长什么样 → Survey 子域                           │
│    2. 量表规则如何配置 → Scale 子域                        │
│    3. 用户是谁 → Actor 子域                                │
│    4. 答卷如何填写 → Survey 子域                           │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## 二、整体架构设计（解决方案域）

### 2.1 四层架构视图

```text
┌─────────────────────────────────────────────────────────────┐
│  Interface Layer (接口层)                                   │
│  ┌──────────────┐           ┌──────────────┐               │
│  │ RESTful API  │           │   gRPC API   │               │
│  │  (B端管理)   │           │  (C端用户)   │               │
│  └──────┬───────┘           └──────┬───────┘               │
└─────────┼──────────────────────────┼─────────────────────────┘
          │                          │
┌─────────┼──────────────────────────┼─────────────────────────┐
│  Application Layer (应用层) - 编排领域对象                  │
│         ↓                          ↓                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Evaluation   │  │ Submission   │  │ ReportQuery  │      │
│  │   Service    │  │   Service    │  │   Service    │      │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘      │
└─────────┼──────────────────┼──────────────────┼─────────────┘
          │                  │                  │
┌─────────┼──────────────────┼──────────────────┼─────────────┐
│  Domain Layer (领域层) - 业务逻辑核心                       │
│         ↓                  ↓                  ↓              │
│  ┌────────────────────────────────────────────────┐         │
│  │  Assessment Aggregate (评估聚合)              │         │
│  │  - 状态机：Pending → Submitted → Interpreted  │         │
│  │  - 得分记录：Score Entity                     │         │
│  │  - 事件发布：AssessmentCreated...             │         │
│  └────────────────────────────────────────────────┘         │
│                                                              │
│  ┌────────────────────────────────────────────────┐         │
│  │  Report Aggregate (报告聚合)                  │         │
│  │  - 维度解读：DimensionInterpret Entity        │         │
│  │  - 建议生成：Suggestion Value Object          │         │
│  │  - 报告导出：Exporter Strategy                 │         │
│  └────────────────────────────────────────────────┘         │
│                                                              │
│  ┌────────────────┐    ┌────────────────┐                  │
│  │  Calculation   │    │ Interpretation │                  │
│  │  Domain Service│    │ Domain Service │                  │
│  │  (策略注册器)  │    │  (策略注册器)  │                  │
│  └────────────────┘    └────────────────┘                  │
└──────────────┬───────────────────┬───────────────────────────┘
               │                   │
┌──────────────┼───────────────────┼───────────────────────────┐
│  Infrastructure Layer (基础设施层)                          │
│               ↓                   ↓                          │
│  ┌──────────────────┐    ┌──────────────────┐              │
│  │  MySQL Storage   │    │ MongoDB Storage  │              │
│  │  - assessments   │    │ - reports 集合   │              │
│  │  - scores        │    │  (文档存储)      │              │
│  │  (事务性)        │    │  (灵活性)        │              │
│  └──────────────────┘    └──────────────────┘              │
└─────────────────────────────────────────────────────────────┘
```

**设计要点**：

1. **接口层**：双协议支持（RESTful 后台管理 + gRPC C端查询）
2. **应用层**：编排多个聚合，无业务逻辑
3. **领域层**：核心业务逻辑，聚合 + 领域服务
4. **基础设施层**：混合存储，各取所长

### 2.2 核心组件与职责

```text
┌─────────────────────────────────────────────────────────────┐
│  组件                职责                     存储策略      │
├─────────────────────────────────────────────────────────────┤
│  Assessment 聚合    管理评估生命周期          MySQL        │
│    - ID             唯一标识                               │
│    - Status         状态机                                 │
│    - Score Entity   得分实体                               │
│    - Events         领域事件                               │
├─────────────────────────────────────────────────────────────┤
│  Report 聚合        管理报告内容              MongoDB      │
│    - Dimensions     维度解读列表                           │
│    - Suggestions    建议列表                               │
│    - Exporter       导出策略                               │
├─────────────────────────────────────────────────────────────┤
│  Calculation        执行计分逻辑              无状态       │
│    - 8种策略        Sum/Average/Weighted...                │
│    - 注册器模式     动态注册和查找                         │
├─────────────────────────────────────────────────────────────┤
│  Interpretation     执行解读逻辑              无状态       │
│    - 阈值策略       基于分数区间                           │
│    - 组合策略       多因子条件判断                         │
│    - 注册器模式     动态注册和查找                         │
└─────────────────────────────────────────────────────────────┘
```

### 2.3 与其他子域的依赖关系

```text
┌──────────────────────────────────────────────────────────────┐
│  依赖方向：自底向上（Evaluation 位于顶层）                  │
└──────────────────────────────────────────────────────────────┘

     Survey 子域                Scale 子域              Actor 子域
  (问卷&答卷数据)            (量表规则配置)           (用户信息)
         ↓                        ↓                       ↓
    ┌────────────────────────────────────────────────────────┐
    │              引用对象（Reference Objects）            │
    │  QuestionnaireRef  MedicalScaleRef  TesteeRef        │
    └────────────────────────────────────────────────────────┘
                            ↓
    ┌────────────────────────────────────────────────────────┐
    │            Evaluation 子域（当前子域）                   │
    │  - Assessment (测评实例)                                │
    │  - Score (得分)                                         │
    │  - Report (报告)                                        │
    └────────────────────────────────────────────────────────┘
                            ↓
                      无向上依赖
```

**依赖规则**：

| 被依赖子域 | 依赖内容 | 访问方式 | 防腐策略 |
| ---------- | --------- | --------- | --------- |
| Survey | 问卷结构、答卷数据 | 只读查询 | 使用 `QuestionnaireRef` + `AnswerSheetRef` |
| Scale | 量表定义、计算规则 | 只读查询 | 使用 `MedicalScaleRef` |
| Actor | 受试者信息 | 只读查询 | 使用 `TesteeRef` (testee.ID) |

### 2.4 防腐层设计（引用对象模式）

**为什么不直接持有其他聚合？**

```go
// ❌ 错误做法：直接依赖其他子域的聚合根
type Assessment struct {
    questionnaire *questionnaire.Questionnaire  // 直接持有
    answerSheet   *answersheet.AnswerSheet      // 直接持有
    medicalScale  *scale.MedicalScale           // 直接持有
}
// 问题：
// 1. 强耦合：修改 questionnaire 结构会影响 assessment
// 2. 生命周期混乱：questionnaire 的加载和持久化与 assessment 纠缠
// 3. 事务边界不清：跨聚合事务难以控制
```

```go
// ✅ 正确做法：使用引用对象（Reference）
type Assessment struct {
    questionnaireRef QuestionnaireRef  // 值对象：Code + Version
    answerSheetRef   AnswerSheetRef    // 值对象：ID
    medicalScaleRef  *MedicalScaleRef  // 值对象：Code（可选）
}

// QuestionnaireRef 问卷引用值对象
type QuestionnaireRef struct {
    code    string  // 问卷编码
    version string  // 问卷版本
}

// 代码位置：internal/apiserver/domain/evaluation/assessment/types.go
```

**优势**：

* ✅ **解耦**：只存储必要的引用信息，不依赖聚合根结构
* ✅ **轻量**：值对象易于序列化和传输
* ✅ **独立**：可以独立验证引用的有效性
* ✅ **清晰**：明确跨聚合边界，避免事务膨胀

---

## 三、核心流程设计（它如何运转？）

### 3.1 评估流程全景图（端到端）

```text
┌────────────────────────────────────────────────────────────────────────────┐
│  阶段1: 创建测评（同步，快速响应）                                         │
│  ────────────────────────────────────────────────────────────────────────  │
│  Actor: B端管理员 / C端用户                                               │
│  ┌─────────┐  创建请求   ┌───────────────┐  验证+创建  ┌──────────────┐  │
│  │ Client  │ ────────→  │ SubmissionSvc │ ─────────→ │  Assessment  │  │
│  └─────────┘             └───────────────┘             │  (Pending)   │  │
│                                                         └──────┬───────┘  │
│                                                                │           │
│                                                    发布 AssessmentCreated  │
│                                                                │           │
└────────────────────────────────────────────────────────────────┼───────────┘
                                                                 │
┌────────────────────────────────────────────────────────────────┼───────────┐
│  阶段2: 异步评估（后台处理，耗时操作）                         │           │
│  ────────────────────────────────────────────────────────────  │           │
│  Actor: qs-worker (事件消费者)                                 ↓           │
│                                                   ┌───────────────────┐    │
│  ┌──────────┐  监听事件  ┌───────────────┐      │   EventBus        │    │
│  │qs-worker │ ←──────── │  EventBus     │      │   (RabbitMQ/      │    │
│  └────┬─────┘            └───────────────┘      │    NSQ等)         │    │
│       │                                          └───────────────────┘    │
│       │ 调用                                                               │
│       ↓                                                                    │
│  ┌─────────────────────────────────────────────────────────────┐         │
│  │  EvaluationService.Evaluate(assessmentID)                   │         │
│  │  ─────────────────────────────────────────────────────────  │         │
│  │                                                              │         │
│  │  步骤1: 加载数据                                             │         │
│  │    ├─ Assessment (MySQL)                                   │         │
│  │    ├─ MedicalScale (MySQL)                                 │         │
│  │    └─ AnswerSheet (通过 Survey 子域接口)                  │         │
│  │                                                              │         │
│  │  步骤2: 计算分数 ━━━━━━━━━━━━━━━━━┓                        │         │
│  │    for each Factor:                ┃                        │         │
│  │      ├─ 提取题目答案               ┃ 策略模式              │         │
│  │      ├─ 选择计算策略 ─────────────→┃ Calculation          │         │
│  │      │   (Sum/Average/Weighted)   ┃ Strategy Registry    │         │
│  │      └─ 计算因子得分               ┃                        │         │
│  │                                     ┗━━━━━━━━━━━━━━━━━━━━  │         │
│  │  步骤3: 生成解读 ━━━━━━━━━━━━━━━━━┓                        │         │
│  │    for each FactorScore:           ┃                        │         │
│  │      ├─ 选择解读策略 ─────────────→┃ 策略模式              │         │
│  │      │   (Threshold/Composite)    ┃ Interpretation       │         │
│  │      └─ 生成解读文本               ┃ Strategy Registry    │         │
│  │                                     ┗━━━━━━━━━━━━━━━━━━━━  │         │
│  │  步骤4: 构建报告 ━━━━━━━━━━━━━━━━━┓                        │         │
│  │    ReportBuilder                   ┃ 建造者模式            │         │
│  │      .WithScale(...)               ┃                        │         │
│  │      .WithTotalScore(...)          ┃                        │         │
│  │      .AddDimension(...)            ┃                        │         │
│  │      .AddSuggestion(...)           ┃                        │         │
│  │      .Build()                      ┗━━━━━━━━━━━━━━━━━━━━  │         │
│  │                                                              │         │
│  │  步骤5: 持久化                                               │         │
│  │    ├─ Update Assessment.Status → Interpreted              │         │
│  │    ├─ Save Scores → MySQL                                 │         │
│  │    └─ Save Report → MongoDB                               │         │
│  │                                                              │         │
│  │  步骤6: 发布事件                                             │         │
│  │    └─ Publish EvaluationCompletedEvent                    │         │
│  │                                                              │         │
│  └─────────────────────────────────────────────────────────────┘         │
│                                                                            │
└────────────────────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────────────────────┐
│  阶段3: 结果查询（快速响应）                                               │
│  ────────────────────────────────────────────────────────────────────────  │
│  Actor: B端管理员 / C端用户                                               │
│                                                                            │
│  ┌─────────┐  查询请求  ┌──────────────┐   读取   ┌──────────────┐      │
│  │ Client  │ ────────→ │ QueryService │ ───────→ │   Report     │      │
│  │         │ ←──────── │              │ ←─────── │  (MongoDB)   │      │
│  └─────────┘  返回报告  └──────────────┘          └──────────────┘      │
│                                                                            │
└────────────────────────────────────────────────────────────────────────────┘
```

**代码入口**：

* 创建测评：`internal/apiserver/application/evaluation/assessment/submission_service.go`
* 异步评估：`internal/apiserver/application/evaluation/assessment/evaluation_service.go`
* 查询报告：`internal/apiserver/application/evaluation/report/query_service.go`

### 3.2 Assessment 状态机设计

```text
┌──────────────────────────────────────────────────────────────┐
│  Assessment 生命周期状态机                                   │
└──────────────────────────────────────────────────────────────┘

     ┌─────────┐
     │ Pending │ 待提交（初始状态）
     └────┬────┘
          │
          │ Submit() - 提交测评
          │ 触发：AssessmentCreatedEvent
          ↓
    ┌──────────┐
    │Submitted │ 已提交（等待评估）
    └────┬─────┘
         │
         │ StartEvaluation() - 开始评估（异步）
         │ 由 qs-worker 触发
         ↓
    ┌────────────┐
    │ Evaluating │ 评估中
    └─┬────────┬─┘
      │        │
      │        │ MarkAsFailed() - 标记失败
      │        │ 原因：计算错误、解读失败等
      │        ↓
      │    ┌────────┐
      │    │ Failed │ 失败（终态）
      │    └────────┘
      │    事件：EvaluationFailedEvent
      │
      │ ApplyEvaluation() - 应用评估结果
      │ 记录：totalScore + riskLevel
      ↓
┌─────────────┐
│ Interpreted │ 已解读（终态）
└─────────────┘
事件：EvaluationCompletedEvent
```

**状态转换规则**（代码位置：`internal/apiserver/domain/evaluation/assessment/types.go`）：

```go
// Status 状态枚举
type Status string

const (
    StatusPending     Status = "pending"      // 待提交
    StatusSubmitted   Status = "submitted"    // 已提交
    StatusInterpreted Status = "interpreted"  // 已解读
    StatusFailed      Status = "failed"       // 失败
)

// 状态转换方法（部分伪代码）
func (a *Assessment) Submit() error {
    if a.status != StatusPending {
        return errors.New("只有 Pending 状态才能提交")
    }
    a.status = StatusSubmitted
    a.submittedAt = time.Now()
    a.addEvent(NewAssessmentCreatedEvent(...))
    return nil
}

func (a *Assessment) ApplyEvaluation(result *EvaluationResult) error {
    if a.status != StatusSubmitted {
        return errors.New("只有 Submitted 状态才能应用评估结果")
    }
    a.status = StatusInterpreted
    a.totalScore = result.TotalScore
    a.riskLevel = result.RiskLevel
    a.interpretedAt = time.Now()
    a.addEvent(NewEvaluationCompletedEvent(...))
    return nil
}

func (a *Assessment) MarkAsFailed(reason string) {
    a.status = StatusFailed
    a.failureReason = &reason
    a.failedAt = time.Now()
    a.addEvent(NewEvaluationFailedEvent(...))
}
```

---

## 四、核心聚合设计

### 4.1 Assessment 聚合总览

Evaluation 子域包含四个核心领域概念：

```text
┌──────────────────────────────────────────────────────────────┐
│                      Evaluation 子域                         │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌─────────────────────────────────────────────────────┐   │
│  │           ① Assessment 聚合（测评实例）              │   │
│  ├─────────────────────────────────────────────────────┤   │
│  │  职责：管理测评生命周期、记录评估结果               │   │
│  │  状态：Pending → Submitted → Interpreted / Failed   │   │
│  │  存储：MySQL（关系型，支持复杂查询）                │   │
│  └───────────────────┬─────────────────────────────────┘   │
│                      │ 协调                                 │
│                      ▼                                       │
│  ┌─────────────────────────────────────────────────────┐   │
│  │      ② Calculation 领域服务（计算策略族）            │   │
│  ├─────────────────────────────────────────────────────┤   │
│  │  职责：将答案转化为分数                             │   │
│  │  策略：求和、平均、加权和、极值、计数等             │   │
│  │  模式：策略模式 + 注册器模式                        │   │
│  └─────────────────────────────────────────────────────┘   │
│                      │                                       │
│                      ▼                                       │
│  ┌─────────────────────────────────────────────────────┐   │
│  │    ③ Interpretation 领域服务（解读策略族）           │   │
│  ├─────────────────────────────────────────────────────┤   │
│  │  职责：将分数转化为业务结论                         │   │
│  │  策略：阈值解读、复合条件解读                       │   │
│  │  模式：策略模式 + 注册器模式                        │   │
│  └─────────────────────────────────────────────────────┘   │
│                      │                                       │
│                      ▼                                       │
│  ┌─────────────────────────────────────────────────────┐   │
│  │         ④ Report 聚合（解读报告）                    │   │
│  ├─────────────────────────────────────────────────────┤   │
│  │  职责：结构化存储解读结果、支持多格式导出           │   │
│  │  内容：维度解读、建议列表、风险等级                 │   │
│  │  存储：MongoDB（文档型，灵活结构）                  │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

### 2.2 核心聚合详解

#### 2.2.1 Assessment 聚合（测评实例）

**核心职责**：记录"一次具体的测评行为"

```go
// 伪代码：Assessment 聚合根
type Assessment struct {
    // 身份标识
    ID        ID
    OrgID     int64
    
    // 关联实体引用
    TesteeRef        testee.ID         // 谁做的测评？
    QuestionnaireRef QuestionnaireRef  // 用什么问卷？
    AnswerSheetRef   AnswerSheetRef    // 答卷在哪里？
    MedicalScaleRef  *MedicalScaleRef  // 用什么量表？（可选）
    
    // 业务来源
    Origin Origin  // adhoc（一次性）| plan（测评计划）| screening（入校筛查）
    
    // 状态与结果
    Status     Status      // pending → submitted → interpreted / failed
    TotalScore *float64    // 总分（评估后填充）
    RiskLevel  *RiskLevel  // 风险等级（评估后填充）
    
    // 时间戳
    CreatedAt     time.Time
    SubmittedAt   *time.Time
    InterpretedAt *time.Time
    FailedAt      *time.Time
    
    // 领域事件
    Events []DomainEvent
}
```

**代码位置**：[`assessment/assessment.go`](../../../internal/apiserver/domain/evaluation/assessment/assessment.go)

**状态机**：

```text
┌──────────┐   Submit()   ┌────────────┐   ApplyEvaluation()   ┌──────────────┐
│ Pending  ├─────────────▶│ Submitted  ├──────────────────────▶│ Interpreted  │
└──────────┘              └────────────┘                        └──────────────┘
                               │
                               │ MarkAsFailed()
                               ▼
                          ┌──────────┐
                          │  Failed  │
                          └──────────┘
```

**关键设计**：

* ✅ 使用**值对象引用**（QuestionnaireRef、AnswerSheetRef）而非直接持有对象，保持聚合边界
* ✅ 状态机保证状态迁移合法性（只能单向流转）
* ✅ 发布领域事件（AssessmentSubmittedEvent、AssessmentInterpretedEvent）驱动异步评估

#### 2.2.2 Calculation 领域服务（计算策略）

**核心职责**：将答案转化为分数

```go
// 伪代码：计算策略接口
type CalculationStrategy interface {
    Calculate(values []float64, params map[string]string) (float64, error)
    StrategyType() StrategyType
}

// 内置策略
- SumStrategy        // 求和：∑values
- AverageStrategy    // 平均：∑values / len(values)
- WeightedSumStrategy // 加权和：∑(values[i] * weights[i])
- MaxStrategy        // 最大值：max(values)
- MinStrategy        // 最小值：min(values)
- CountStrategy      // 计数：len(values)
- FirstValueStrategy // 首值：values[0]
```

**代码位置**：[`calculation/strategy.go`](../../../internal/apiserver/domain/evaluation/calculation/strategy.go)

**使用示例**：

```go
// 获取策略并计算
strategy := calculation.GetStrategy(calculation.TypeSum)
score, err := strategy.Calculate([]float64{1, 2, 3}, nil)
// score = 6
```

**设计模式**：

* ✅ **策略模式**：封装算法族，运行时动态选择
* ✅ **注册器模式**：支持动态注册自定义策略

#### 2.2.3 Interpretation 领域服务（解读策略）

**核心职责**：将分数转化为业务结论

```go
// 伪代码：解读策略接口
type InterpretationStrategy interface {
    Interpret(score float64, config *InterpretConfig) (*InterpretResult, error)
    StrategyType() StrategyType
}

// 内置策略
- ThresholdStrategy  // 阈值策略：根据分数区间解读
- CompositeStrategy  // 复合策略：多维度条件组合
```

**代码位置**：[`interpretation/strategy.go`](../../../internal/apiserver/domain/evaluation/interpretation/strategy.go)

**阈值策略示例**：

```go
// 配置解读规则
config := &InterpretConfig{
    Ranges: []ScoreRange{
        {Min: 0, Max: 50, Level: "正常", Conclusion: "状态良好"},
        {Min: 51, Max: 70, Level: "轻度", Conclusion: "需要关注"},
        {Min: 71, Max: 100, Level: "重度", Conclusion: "建议干预"},
    },
}

// 执行解读
strategy := interpretation.GetStrategy(interpretation.TypeThreshold)
result, err := strategy.Interpret(65, config)
// result: { Level: "轻度", Conclusion: "需要关注" }
```

**设计模式**：

* ✅ **策略模式**：封装解读规则，支持多种解读方式
* ✅ **复合条件模式**：支持 AND/OR 逻辑组合

#### 2.2.4 Report 聚合（解读报告）

**核心职责**：结构化存储解读结果

```go
// 伪代码：Report 聚合根
type InterpretReport struct {
    ID       ID  // 与 AssessmentID 一致
    
    // 量表信息
    ScaleName string
    ScaleCode string
    
    // 评估结果汇总
    TotalScore float64
    RiskLevel  RiskLevel
    Conclusion string
    
    // 维度解读列表
    Dimensions []DimensionInterpret {
        FactorCode  string   // 因子编码
        FactorName  string   // 因子名称
        Score       float64  // 因子得分
        Level       string   // 风险等级
        Conclusion  string   // 因子结论
    }
    
    // 建议列表
    Suggestions []string
    
    // 时间戳
    CreatedAt time.Time
    UpdatedAt *time.Time
}
```

**代码位置**：[`report/report.go`](../../../internal/apiserver/domain/evaluation/report/report.go)

**设计决策**：

* ✅ 使用 MongoDB 存储（灵活的文档结构，适合解读报告的多样性）
* ✅ 与 Assessment 1:1 关系（ID 与 AssessmentID 一致）
* ✅ 分离存储（Assessment 关注流程状态，Report 关注内容呈现）

---

## 3. 架构分层设计

### 3.1 六边形架构视图

```text
┌─────────────────────────────────────────────────────────────────┐
│                      Interface Layer                            │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │ REST Handler │  │ gRPC Service │  │ Event Handler│          │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘          │
└─────────┼──────────────────┼──────────────────┼─────────────────┘
          │                  │                  │
          ▼                  ▼                  ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Application Layer                            │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────┐  │
│  │EvaluationService │  │ ReportExportSvc  │  │ QueryService │  │
│  │ (测评评估编排)    │  │  (报告导出)      │  │  (查询聚合)  │  │
│  └────────┬─────────┘  └────────┬─────────┘  └──────┬───────┘  │
└───────────┼─────────────────────┼─────────────────────┼─────────┘
            │  协调领域服务         │                    │
            ▼                     ▼                    ▼
┌─────────────────────────────────────────────────────────────────┐
│                       Domain Layer                              │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Assessment 聚合                                         │   │
│  │  ├─ Assessment (聚合根)                                 │   │
│  │  ├─ Score (实体)                                        │   │
│  │  ├─ AssessmentCreator (创建器 - Builder 模式)          │   │
│  │  └─ AssessmentRepository (仓储接口)                    │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Calculation 领域服务                                    │   │
│  │  ├─ CalculationStrategy (策略接口)                      │   │
│  │  ├─ SumStrategy / AverageStrategy / ...                 │   │
│  │  └─ StrategyRegistry (注册器)                           │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Interpretation 领域服务                                 │   │
│  │  ├─ InterpretationStrategy (策略接口)                   │   │
│  │  ├─ ThresholdStrategy / CompositeStrategy               │   │
│  │  └─ StrategyRegistry (注册器)                           │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Report 聚合                                             │   │
│  │  ├─ InterpretReport (聚合根)                            │   │
│  │  ├─ DimensionInterpret (实体)                           │   │
│  │  ├─ ReportBuilder (构建器 - Builder 模式)              │   │
│  │  ├─ ReportExporter (导出器 - Strategy 模式)            │   │
│  │  └─ ReportRepository (仓储接口)                         │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                  Infrastructure Layer                           │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────┐  │
│  │MySQL Repository  │  │MongoDB Repository│  │ Event Bus    │  │
│  │(Assessment/Score)│  │     (Report)     │  │  (Kafka)     │  │
│  └──────────────────┘  └──────────────────┘  └──────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### 3.2 分层职责说明

| 层级 | 职责 | 关键类 |
| ----- | ------ | -------- |
| **Interface** | 协议适配（REST/gRPC/Event） | AssessmentHandler、ReportHandler |
| **Application** | 业务流程编排、事务边界管理 | EvaluationService、QueryService |
| **Domain** | 核心业务逻辑、领域规则 | Assessment、Calculation、Interpretation、Report |
| **Infrastructure** | 持久化、消息队列、外部服务 | MySQLRepository、MongoRepository、EventBus |

---

## 4. 核心流程设计

### 4.1 测评评估完整流程

```text
┌─────────┐                                                      
│  User   │                                                      
└────┬────┘                                                      
     │ 1. 提交答卷                                              
     ▼                                                           
┌─────────────────┐                                             
│ Interface Layer │                                             
│  (REST API)     │                                             
└────────┬────────┘                                             
         │ 2. 调用创建测评服务                                   
         ▼                                                       
┌─────────────────────────┐                                     
│   Application Layer     │                                     
│  SubmissionService      │                                     
└────────┬────────────────┘                                     
         │ 3. 验证 & 创建 Assessment                            
         ▼                                                       
┌─────────────────────────┐                                     
│      Domain Layer       │                                     
│  AssessmentCreator      │                                     
└────────┬────────────────┘                                     
         │ 4. 状态：Pending → Submitted                         
         │ 5. 发布 AssessmentSubmittedEvent                     
         ▼                                                       
┌─────────────────────────┐                                     
│  Infrastructure Layer   │                                     
│   MySQL + EventBus      │                                     
└────────┬────────────────┘                                     
         │                                                       
         │ 6. Event: AssessmentSubmittedEvent                   
         ▼                                                       
┌─────────────────────────┐                                     
│    qs-worker (异步)      │                                     
│   EvaluationService     │                                     
└────────┬────────────────┘                                     
         │ 7. 加载 Assessment、Scale、AnswerSheet               
         │ 8. 执行计算策略（Calculation）                        
         │ 9. 执行解读策略（Interpretation）                     
         │ 10. 构建报告（ReportBuilder）                         
         │ 11. 更新 Assessment 状态 → Interpreted               
         │ 12. 保存 Report 到 MongoDB                           
         ▼                                                       
┌─────────────────────────┐                                     
│   MySQL + MongoDB       │                                     
│  (持久化完成)            │                                     
└─────────────────────────┘                                     
```

### 4.2 关键步骤详解

#### 步骤 3-5：创建测评（同步）

```go
// 伪代码：SubmissionService.Submit()
func (s *SubmissionService) Submit(ctx context.Context, req SubmitRequest) (*AssessmentDTO, error) {
    // 1. 跨聚合验证（受试者、问卷、答卷、量表）
    creator := assessment.NewAssessmentCreator(
        testeeValidator,
        questionnaireValidator,
        answerSheetValidator,
        scaleValidator,
    )
    
    // 2. 创建测评（自动提交）
    req := assessment.NewCreateAssessmentRequest(
        orgID, testeeID, questionnaireRef, answerSheetRef, origin,
    ).WithMedicalScale(scaleRef)
    
    assess, err := creator.Create(ctx, req)
    if err != nil {
        return nil, err
    }
    
    // 3. 持久化（事务边界）
    if err := s.assessmentRepo.Save(ctx, assess); err != nil {
        return nil, err
    }
    
    // 4. 发布领域事件（触发异步评估）
    events := assess.GetEvents()
    for _, event := range events {
        s.eventBus.Publish(ctx, event)
    }
    
    return toDTO(assess), nil
}
```

**代码位置**：[`application/evaluation/assessment/submission_service.go`](../../../internal/apiserver/application/evaluation/assessment/submission_service.go)

#### 步骤 7-12：评估测评（异步）

```go
// 伪代码：EvaluationService.Evaluate()
func (s *EvaluationService) Evaluate(ctx context.Context, assessmentID uint64) error {
    // 1. 加载 Assessment
    assess, err := s.assessmentRepo.FindByID(ctx, assessmentID)
    if err != nil || !assess.Status().IsSubmitted() {
        return err
    }
    
    // 2. 加载 MedicalScale
    scale, err := s.scaleRepo.FindByCode(ctx, assess.MedicalScaleRef().Code())
    if err != nil {
        return err
    }
    
    // 3. 加载 AnswerSheet（从 Survey 子域）
    answerSheet, err := s.answerSheetRepo.FindByID(ctx, assess.AnswerSheetRef().ID())
    if err != nil {
        return err
    }
    
    // 4. 执行计算策略
    factorScores := []assessment.FactorScore{}
    for _, factor := range scale.Factors() {
        strategy := calculation.GetStrategy(factor.ScoringStrategy())
        score, _ := strategy.Calculate(factor.GetValues(answerSheet), factor.Params())
        factorScores = append(factorScores, assessment.FactorScore{
            FactorCode: factor.Code(),
            Score:      score,
        })
    }
    
    // 5. 执行解读策略
    interpretResults := []assessment.InterpretResult{}
    for _, fs := range factorScores {
        strategy := interpretation.GetStrategy(factor.InterpretStrategy())
        result, _ := strategy.Interpret(fs.Score, factor.InterpretConfig())
        interpretResults = append(interpretResults, result)
    }
    
    // 6. 组装评估结果
    evalResult := &assessment.EvaluationResult{
        TotalScore:   calculateTotal(factorScores),
        RiskLevel:    determineRiskLevel(interpretResults),
        FactorScores: factorScores,
    }
    
    // 7. 应用评估结果到 Assessment
    assess.ApplyEvaluation(evalResult)
    
    // 8. 构建报告
    reportBuilder := report.NewDefaultReportBuilder()
    interpretReport, _ := reportBuilder.Build(assess, scale, evalResult)
    
    // 9. 持久化（事务边界）
    if err := s.assessmentRepo.Save(ctx, assess); err != nil {
        return err
    }
    if err := s.reportRepo.Save(ctx, interpretReport); err != nil {
        return err
    }
    
    return nil
}
```

**代码位置**：[`application/evaluation/assessment/evaluation_service.go`](../../../internal/apiserver/application/evaluation/assessment/evaluation_service.go)

---

## 5. 核心设计模式

### 5.1 策略模式（Strategy Pattern）

**应用场景**：

1. **计算策略**（Calculation）：封装不同的计分算法
2. **解读策略**（Interpretation）：封装不同的解读规则
3. **导出策略**（Report Exporter）：封装不同的导出格式

**设计结构**：

```text
┌─────────────────────────────────────┐
│     CalculationStrategy (接口)      │
├─────────────────────────────────────┤
│ + Calculate(values, params): score │
│ + StrategyType(): string            │
└──────────────┬──────────────────────┘
               │
     ┌─────────┼─────────┬─────────┐
     │         │         │         │
     ▼         ▼         ▼         ▼
┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐
│   Sum   │ │ Average │ │Weighted │ │   Max   │
│Strategy │ │Strategy │ │  Sum    │ │Strategy │
└─────────┘ └─────────┘ └─────────┘ └─────────┘
```

**优势**：

* ✅ 算法封装：每种策略独立封装，职责单一
* ✅ 动态选择：运行时根据配置选择策略
* ✅ 易于扩展：新增策略无需修改现有代码（开闭原则）

**代码示例**：

```go
// 注册策略
calculation.RegisterStrategy(&CustomMedianStrategy{})

// 使用策略
strategy := calculation.GetStrategy("median")
score, err := strategy.Calculate(values, params)
```

### 5.2 构建器模式（Builder Pattern）

**应用场景**：

1. **AssessmentCreator**：创建 Assessment 聚合根（需验证多个依赖）
2. **ReportBuilder**：构建 InterpretReport（复杂对象分步构建）

**设计结构**：

```text
┌──────────────────────────────────┐
│      AssessmentCreator           │
├──────────────────────────────────┤
│ + ValidateTestee()               │
│ + ValidateQuestionnaire()        │
│ + ValidateAnswerSheet()          │
│ + ValidateScale()                │
│ + Build(): Assessment            │
└──────────────────────────────────┘
```

**优势**：

* ✅ 隔离复杂性：将复杂的创建过程封装在 Creator 中
* ✅ 步骤可控：分步验证，任一步骤失败即停止
* ✅ 可测试性：便于 Mock 各个验证器进行单元测试

**代码示例**：

```go
// 创建器使用示例
creator := assessment.NewAssessmentCreator(
    testeeValidator,
    questionnaireValidator,
    answerSheetValidator,
    scaleValidator,
)

assess, err := creator.Create(ctx, request)
```

### 5.3 注册器模式（Registry Pattern）

**应用场景**：策略动态注册与获取

**设计结构**：

```go
// 全局注册表
var strategyRegistry = make(map[StrategyType]CalculationStrategy)

// 注册策略
func RegisterStrategy(strategy CalculationStrategy) {
    strategyRegistry[strategy.StrategyType()] = strategy
}

// 获取策略
func GetStrategy(strategyType StrategyType) CalculationStrategy {
    return strategyRegistry[strategyType]
}

// 初始化时自动注册
func init() {
    RegisterStrategy(&SumStrategy{})
    RegisterStrategy(&AverageStrategy{})
    // ...
}
```

**优势**：

* ✅ 解耦：策略实现与使用方解耦
* ✅ 可扩展：第三方可注册自定义策略
* ✅ 集中管理：统一的策略获取入口

### 5.4 状态机模式（State Machine）

**应用场景**：Assessment 状态管理

**状态流转**：

```text
┌──────────┐   Submit()   ┌────────────┐   ApplyEvaluation()   ┌──────────────┐
│ Pending  ├─────────────▶│ Submitted  ├──────────────────────▶│ Interpreted  │
└──────────┘              └────┬───────┘                        └──────────────┘
                               │
                               │ MarkAsFailed()
                               ▼
                          ┌──────────┐
                          │  Failed  │
                          └──────────┘
```

**实现方式**：

```go
// 状态迁移方法（领域逻辑）
func (a *Assessment) Submit() error {
    if !a.status.IsPending() {
        return ErrInvalidStatusTransition
    }
    a.status = StatusSubmitted
    a.submittedAt = &now
    a.publishEvent(NewAssessmentSubmittedEvent(...))
    return nil
}

func (a *Assessment) ApplyEvaluation(result *EvaluationResult) error {
    if !a.status.IsSubmitted() {
        return ErrInvalidStatusTransition
    }
    a.status = StatusInterpreted
    a.totalScore = &result.TotalScore
    a.riskLevel = &result.RiskLevel
    a.interpretedAt = &now
    a.publishEvent(NewAssessmentInterpretedEvent(...))
    return nil
}
```

**优势**：

* ✅ 状态约束：代码层面保证状态迁移合法性
* ✅ 领域语义：状态迁移方法反映业务术语（Submit、ApplyEvaluation）
* ✅ 事件驱动：状态迁移自动发布领域事件

### 5.5 领域事件模式（Domain Event）

**应用场景**：异步解耦、事件驱动

**事件定义**：

```go
// AssessmentSubmittedEvent - 测评已提交
type AssessmentSubmittedEvent struct {
    EventID      string
    OccurredAt   time.Time
    AssessmentID ID
    TesteeID     testee.ID
    // ...
}

// AssessmentInterpretedEvent - 测评已解读
type AssessmentInterpretedEvent struct {
    EventID      string
    OccurredAt   time.Time
    AssessmentID ID
    TotalScore   float64
    RiskLevel    RiskLevel
    // ...
}
```

**事件流**：

```text
┌─────────────┐  Submit()   ┌────────────────────────────┐
│ Assessment  ├────────────▶│ AssessmentSubmittedEvent   │
└─────────────┘             └──────────┬─────────────────┘
                                       │
                                       │ EventBus.Publish()
                                       ▼
                            ┌──────────────────────┐
                            │    qs-worker         │
                            │  (Event Consumer)    │
                            └──────────┬───────────┘
                                       │ Evaluate()
                                       ▼
                            ┌─────────────────────────────┐
                            │ AssessmentInterpretedEvent  │
                            └─────────────────────────────┘
```

**优势**：

* ✅ 异步处理：评估耗时操作不阻塞主流程
* ✅ 解耦：事件发布者不关心订阅者
* ✅ 可扩展：新增订阅者不影响现有逻辑

---

## 6. 目录结构设计

### 6.1 领域层结构

```text
internal/apiserver/domain/evaluation/
├── assessment/                     # Assessment 聚合
│   ├── assessment.go               # 聚合根（状态机、事件发布）
│   ├── score.go                    # Score 实体（因子得分）
│   ├── creator.go                  # 创建器（Builder 模式）
│   ├── types.go                    # 值对象（Status、Origin、RiskLevel）
│   ├── events.go                   # 领域事件
│   ├── errors.go                   # 领域错误
│   └── repository.go               # 仓储接口
│
├── calculation/                    # Calculation 策略
│   ├── strategy.go                 # 策略接口 + 注册器
│   ├── sum.go                      # 求和策略
│   ├── average.go                  # 平均值策略
│   ├── weighted_sum.go             # 加权和策略
│   ├── extremum.go                 # 极值策略（Max/Min）
│   ├── auxiliary.go                # 辅助策略（Count/FirstValue）
│   └── types.go                    # 通用类型
│
├── interpretation/                 # Interpretation 策略
│   ├── strategy.go                 # 策略接口 + 注册器
│   ├── threshold.go                # 阈值策略
│   ├── composite.go                # 复合策略
│   ├── types.go                    # 值对象（ScoreRange、InterpretItem）
│   └── errors.go                   # 领域错误
│
└── report/                         # Report 聚合
    ├── report.go                   # 聚合根
    ├── dimension.go                # Dimension 实体
    ├── suggestion.go               # Suggestion 值对象
    ├── builder.go                  # 构建器（Builder 模式）
    ├── exporter.go                 # 导出器（Strategy 模式）
    ├── types.go                    # 值对象
    ├── errors.go                   # 领域错误
    └── repository.go               # 仓储接口
```

### 6.2 应用服务层结构

```text
internal/apiserver/application/evaluation/
├── assessment/                     # Assessment 应用服务
│   ├── evaluation_service.go       # 评估编排服务（qs-worker 调用）
│   ├── submission_service.go       # 提交服务（API 调用）
│   ├── query_service.go            # 查询服务
│   └── dto.go                      # DTO 定义
│
└── report/                         # Report 应用服务
    ├── query_service.go            # 报告查询服务
    ├── export_service.go           # 报告导出服务
    └── dto.go                      # DTO 定义
```

---

## 7. 存储设计

### 7.1 双存储策略

| 聚合 | 存储 | 原因 | 表结构 |
| ----- | ------ | ------ | -------- |
| **Assessment** | MySQL | 需要复杂查询（按状态、时间、来源等） | `assessments` 表 |
| **Score** | MySQL | 需要趋势分析（多次测评对比） | `assessment_scores` 表 |
| **Report** | MongoDB | 灵活的文档结构（维度、建议列表） | `interpret_reports` 集合 |

### 7.2 MySQL 表设计

```sql
-- assessments 表
CREATE TABLE assessments (
    id BIGINT UNSIGNED PRIMARY KEY,
    org_id BIGINT UNSIGNED NOT NULL,
    testee_id BIGINT UNSIGNED NOT NULL,
    questionnaire_id BIGINT UNSIGNED NOT NULL,
    questionnaire_version INT NOT NULL,
    answer_sheet_id VARCHAR(50) NOT NULL,
    medical_scale_code VARCHAR(50),
    origin_type VARCHAR(20) NOT NULL,
    origin_ref_id VARCHAR(50),
    status VARCHAR(20) NOT NULL,
    total_score DECIMAL(10,2),
    risk_level VARCHAR(20),
    created_at TIMESTAMP NOT NULL,
    submitted_at TIMESTAMP,
    interpreted_at TIMESTAMP,
    failed_at TIMESTAMP,
    failure_reason TEXT,
    INDEX idx_testee_status (testee_id, status),
    INDEX idx_origin (origin_type, origin_ref_id),
    INDEX idx_created_at (created_at)
);

-- assessment_scores 表
CREATE TABLE assessment_scores (
    id BIGINT UNSIGNED PRIMARY KEY,
    assessment_id BIGINT UNSIGNED NOT NULL,
    factor_code VARCHAR(50) NOT NULL,
    factor_name VARCHAR(100) NOT NULL,
    score DECIMAL(10,2) NOT NULL,
    risk_level VARCHAR(20),
    created_at TIMESTAMP NOT NULL,
    INDEX idx_assessment (assessment_id),
    UNIQUE KEY uk_assessment_factor (assessment_id, factor_code)
);
```

### 7.3 MongoDB 文档设计

```json
// interpret_reports 集合
{
  "_id": "1234567890",  // 与 AssessmentID 一致
  "scale_name": "抑郁自评量表",
  "scale_code": "SDS",
  "total_score": 65.0,
  "risk_level": "moderate",
  "conclusion": "存在轻度抑郁症状，建议进一步关注。",
  "dimensions": [
    {
      "factor_code": "F1",
      "factor_name": "情感维度",
      "score": 22.0,
      "level": "high",
      "conclusion": "情感维度得分较高，提示情绪低落。"
    },
    {
      "factor_code": "F2",
      "factor_name": "认知维度",
      "score": 18.0,
      "level": "moderate",
      "conclusion": "认知维度得分中等，存在一定的认知偏差。"
    }
  ],
  "suggestions": [
    "建议定期进行心理咨询",
    "保持规律的作息时间",
    "适当进行体育锻炼"
  ],
  "created_at": ISODate("2025-01-29T10:00:00Z"),
  "updated_at": ISODate("2025-01-29T10:00:00Z")
}
```

---

## 8. 扩展性设计

### 8.1 如何新增计算策略

```go
// 1. 定义策略类型
const TypeMedian StrategyType = "median"

// 2. 实现策略接口
type MedianStrategy struct{}

func (s *MedianStrategy) Calculate(values []float64, params map[string]string) (float64, error) {
    // 中位数计算逻辑
    sort.Float64s(values)
    n := len(values)
    if n%2 == 0 {
        return (values[n/2-1] + values[n/2]) / 2, nil
    }
    return values[n/2], nil
}

func (s *MedianStrategy) StrategyType() StrategyType {
    return TypeMedian
}

// 3. 注册策略
func init() {
    calculation.RegisterStrategy(&MedianStrategy{})
}
```

**详见**：《11-06-08 扩展指南》

### 8.2 如何新增解读策略

```go
// 1. 定义策略类型
const TypePercentile StrategyType = "percentile"

// 2. 实现策略接口
type PercentileStrategy struct{}

func (s *PercentileStrategy) Interpret(score float64, config *InterpretConfig) (*InterpretResult, error) {
    // 百分位解读逻辑
    percentile := calculatePercentile(score, config.NormData)
    return &InterpretResult{
        Level:      determineLevel(percentile),
        Conclusion: fmt.Sprintf("您的得分超过了 %.0f%% 的人群", percentile),
    }, nil
}

func (s *PercentileStrategy) StrategyType() StrategyType {
    return TypePercentile
}

// 3. 注册策略
func init() {
    interpretation.RegisterStrategy(&PercentileStrategy{})
}
```

### 8.3 如何新增导出格式

```go
// 1. 实现导出器接口
type ExcelExporter struct{}

func (e *ExcelExporter) Export(report *InterpretReport) ([]byte, error) {
    // Excel 导出逻辑
    file := excel.NewFile()
    // ... 填充数据
    return file.WriteToBuffer()
}

func (e *ExcelExporter) Format() ExportFormat {
    return FormatExcel
}

// 2. 注册导出器
report.RegisterExporter(&ExcelExporter{})
```

---

## 9. 性能优化考虑

### 9.1 异步评估

* ✅ 提交答卷（同步）：只创建 Assessment，状态为 Submitted
* ✅ 评估计算（异步）：qs-worker 消费事件，执行耗时计算
* ✅ 用户体验：用户提交后立即返回，后台异步处理

### 9.2 缓存策略

| 缓存对象 | 缓存时间 | 失效策略 |
| --------- | --------- | --------- |
| **MedicalScale** | 1 小时 | 量表更新时主动清除 |
| **Questionnaire** | 30 分钟 | 版本变更时清除 |
| **InterpretReport** | 永久（除非重新评估） | Assessment 重新评估时清除 |

### 9.3 批量查询优化

```go
// 批量加载 Assessment + Report
func (s *QueryService) BatchGetWithReports(ctx context.Context, assessmentIDs []uint64) ([]AssessmentWithReportDTO, error) {
    // 1. 批量加载 Assessment（单次 MySQL 查询）
    assessments, _ := s.assessmentRepo.FindByIDs(ctx, assessmentIDs)
    
    // 2. 批量加载 Report（单次 MongoDB 查询）
    reportIDs := extractReportIDs(assessments)
    reports, _ := s.reportRepo.FindByIDs(ctx, reportIDs)
    
    // 3. 内存中组装
    return assembleResults(assessments, reports), nil
}
```

---

## 10. 测试策略

### 10.1 单元测试

**测试重点**：

* ✅ Assessment 状态机（状态迁移合法性）
* ✅ 计算策略（各种策略的计算正确性）
* ✅ 解读策略（分数区间解读准确性）
* ✅ 领域事件发布（事件内容完整性）

**示例**：

```go
func TestAssessment_Submit(t *testing.T) {
    // 创建 pending 状态的测评
    assess, _ := assessment.NewAssessment(...)
    
    // 提交测评
    err := assess.Submit()
    
    // 断言
    assert.NoError(t, err)
    assert.Equal(t, assessment.StatusSubmitted, assess.Status())
    assert.NotNil(t, assess.SubmittedAt())
    assert.Len(t, assess.GetEvents(), 1) // 发布了 AssessmentSubmittedEvent
}
```

### 10.2 集成测试

**测试重点**：

* ✅ 完整评估流程（提交 → 计算 → 解读 → 报告）
* ✅ 跨聚合验证（AssessmentCreator）
* ✅ 事件驱动流程（Event → qs-worker → Evaluation）

### 10.3 端到端测试

**测试场景**：

1. 用户提交答卷 → 创建测评 → 异步评估 → 查询报告
2. 重新评估 → 更新 Assessment → 更新 Report
3. 导出报告 → PDF/Word 格式验证

---

## 11. 关键设计决策总结

### 11.1 为什么使用策略模式？

**问题**：不同量表有不同的计分方法和解读规则

**方案**：

* ❌ 硬编码：在代码中 if-else 判断量表类型（不可扩展）
* ✅ 策略模式：封装算法族，配置化选择策略（开闭原则）

**收益**：

* 新增量表只需配置策略类型，无需修改代码
* 第三方可注册自定义策略（如自定义计分算法）

### 11.2 为什么使用 Builder 模式？

**问题**：创建 Assessment 需要验证多个依赖对象（受试者、问卷、答卷、量表）

**方案**：

* ❌ 直接构造：在应用服务层逐个验证（逻辑分散）
* ✅ Builder 模式：封装创建流程，集中验证逻辑（领域服务）

**收益**：

* 领域逻辑集中在领域层，应用服务层只负责编排
* 便于单元测试（Mock 各个验证器）

### 11.3 为什么使用事件驱动？

**问题**：评估计算耗时，不能阻塞用户提交

**方案**：

* ❌ 同步评估：用户提交后等待计算完成（用户体验差）
* ✅ 异步评估：发布事件，qs-worker 异步处理（解耦）

**收益**：

* 用户提交后立即返回，提升体验
* 评估失败不影响答卷提交
* 后续可扩展（如发送通知、触发其他业务）

### 11.4 为什么分离 Assessment 和 Report？

**问题**：Assessment 管理流程，Report 管理结果，职责不同

**方案**：

* ❌ 合并存储：Assessment 包含 Report 字段（聚合过大）
* ✅ 分离存储：Assessment 用 MySQL，Report 用 MongoDB（各司其职）

**收益**：

* Assessment 支持复杂查询（状态、时间、来源）
* Report 支持灵活结构（维度、建议列表）
* 可独立扩展（如 Report 支持更多导出格式）

---

## 12. 后续文档预告

| 文档编号 | 标题 | 核心内容 |
| --------- | ------ | --------- |
| **11-06-02** | Assessment 聚合设计 | 聚合根、Score 实体、状态机、领域事件 |
| **11-06-03** | Calculation 计算策略设计 | 7 种内置策略、注册器、扩展示例 |
| **11-06-04** | Interpretation 解读策略设计 | 阈值策略、复合策略、高危识别 |
| **11-06-05** | Report 聚合设计 | 报告结构、Builder、Exporter |
| **11-06-06** | 应用服务层设计 | EvaluationService、QueryService、DTO |
| **11-06-07** | 设计模式应用总结 | 模式协作、扩展性保证 |
| **11-06-08** | 扩展指南 | 自定义策略、自定义导出格式 |

---

## 13. 相关文档链接

* 《11-01-问卷&量表BC领域模型总览-v2.md》
* 《11-02-qs-apiserver领域层代码结构设计-v2.md》
* 《11-04-Survey子域设计》
* 《11-05-Scale子域设计-v2.md》

---

## 附录：术语表

| 术语 | 英文 | 说明 |
| ----- | ------ | ------ |
| 测评 | Assessment | 一次具体的测评行为（谁、用什么问卷、什么时候做的） |
| 答卷 | AnswerSheet | 用户填写的答案内容 |
| 量表 | MedicalScale | 专业评估工具（如 SDS、SAS） |
| 因子 | Factor | 量表的维度（如情感维度、认知维度） |
| 计分 | Scoring | 将答案转化为分数 |
| 解读 | Interpretation | 将分数转化为业务结论 |
| 风险等级 | RiskLevel | 低风险、中风险、高风险 |
| 聚合根 | Aggregate Root | DDD 中的核心概念，聚合的入口对象 |
| 值对象 | Value Object | 无身份的不可变对象 |
| 领域事件 | Domain Event | 领域中发生的重要业务事件 |

---

> **作者**：Evaluation 子域设计团队  
> **审阅**：领域架构师  
> **版本历史**：
>
> * V1.0 (2025-01-29)：初始版本，基于实际代码编写
