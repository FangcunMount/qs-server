# Assessment 子域领域层设计总结

## 目录结构

```text
internal/apiserver/domain/
├── assessment/               # Assessment 子域（测评记录与报告）
│   ├── assessment.go         # Assessment 聚合根
│   ├── score.go              # AssessmentScore 实体 + FactorScore 值对象
│   ├── report.go             # InterpretReport 聚合根
│   ├── creator.go            # AssessmentCreator 领域服务（创建+验证+提交）
│   ├── report_builder.go     # ReportBuilder 领域服务（报告构建）
│   ├── types.go              # 类型定义（ID、状态、来源、值对象等）
│   ├── events.go             # 领域事件定义
│   ├── repository.go         # Repository 接口定义
│   └── errors.go             # 领域错误定义（哨兵错误）
│
├── calculation/              # Calculation 功能域（无状态计算）
│   ├── README.md             # 模块说明文档
│   ├── types.go              # 策略类型定义
│   ├── strategy.go           # 策略接口与注册表
│   ├── sum.go                # 求和策略
│   ├── average.go            # 平均值策略
│   ├── weighted_sum.go       # 加权求和策略
│   ├── extremum.go           # 最大值/最小值策略
│   └── auxiliary.go          # 辅助策略（计数、首值、末值）
│
└── interpretation/           # Interpretation 功能域（无状态解读）
    ├── README.md             # 模块说明文档
    ├── types.go              # 类型定义（规则、结果等）
    ├── strategy.go           # 策略接口与注册表
    ├── threshold.go          # 阈值解读策略
    ├── composite.go          # 组合解读策略
    └── errors.go             # 领域错误定义
```

## 架构设计原则

### 单一职责分离

本设计遵循单一职责原则，将评估功能拆分为三个独立的域：

1. **Calculation（计算功能域）**：无状态的纯计算
2. **Interpretation（解读功能域）**：无状态的解读逻辑
3. **Assessment（测评子域）**：有状态的测评生命周期管理

### 无状态功能域设计

`calculation` 和 `interpretation` 设计为**无状态功能域**：

- 纯函数式：输入 → 计算/解读 → 输出
- 无副作用：不持久化、不发事件
- 便于测试：可单独进行单元测试
- 高复用性：survey 域和 assessment 域都可使用

## 核心设计

### 1. Assessment 聚合根

```go
// Assessment 测评聚合根
type Assessment struct {
    id               ID
    testeeRef        testee.ID
    questionnaireRef QuestionnaireRef
    answerSheetRef   AnswerSheetRef
    medicalScaleRef  *MedicalScaleRef
    origin           Origin
    status           Status           // pending → submitted → interpreted/failed
    totalScore       *float64
    riskLevel        *RiskLevel
    // ...
}
```

**职责**：

- 记录一次测评行为的完整生命周期
- 管理状态迁移：pending → submitted → interpreted/failed
- 发布领域事件

**领域事件**：

| 事件 | 触发时机 | 用途 |
|-----|---------|------|
| AssessmentSubmittedEvent | 答卷提交时 | 触发评估流程 |
| AssessmentInterpretedEvent | 评估完成时 | 通知、预警、统计 |
| AssessmentFailedEvent | 评估失败时 | 日志、监控 |

### 2. AssessmentScore 实体

**职责**：

- 记录一次测评的完整得分信息
- 包含总分、风险等级、所有因子得分
- 支持按维度查询和趋势分析

**结构**：

```go
// AssessmentScore 测评得分实体（从属于 Assessment）
type AssessmentScore struct {
    assessmentID  meta.ID           // 所属测评 ID
    totalScore    float64           // 测评总分
    riskLevel     scale.RiskLevel   // 整体风险等级
    factorScores  []FactorScore     // 因子得分列表
}

// FactorScore 因子得分值对象
type FactorScore struct {
    factorCode    scale.FactorCode  // 因子编码
    factorName    string            // 因子名称
    rawScore      float64           // 原始得分
    riskLevel     scale.RiskLevel   // 风险等级
    isTotalScore  bool              // 是否为总分因子
}
```

**存储**：MySQL（便于 SQL 聚合查询）

**关系**：从属于 Assessment，1:1 对应

### 3. InterpretReport 聚合根

**职责**：

- 对外展示的解读报告
- 存储在 MongoDB（灵活的文档结构）
- 1:1 对应 Assessment

### 4. Calculation 功能域

**定位**：无状态的计分功能域

**策略模式实现**：

```go
type ScoringStrategy interface {
    Calculate(values []float64, params map[string]string) (float64, error)
    StrategyType() StrategyType
}
```

**支持的策略**：

| 策略 | 说明 | 使用场景 |
|-----|------|---------|
| sum | 求和 | 总分计算 |
| average | 平均值 | 均分因子 |
| weighted_sum | 加权求和 | 不同题目权重不同 |
| max | 最大值 | 取最高分 |
| min | 最小值 | 取最低分 |
| count | 计数 | 统计题目数 |
| first | 首值 | 取第一个答案 |
| last | 末值 | 取最后一个答案 |

**使用场景**：

- Survey 域：计算答卷中每个 answer 的得分
- Assessment 域：计算每个 factor 的得分

### 5. Interpretation 功能域

**定位**：无状态的解读功能域

**策略模式实现**：

```go
type InterpretStrategy interface {
    Interpret(score float64, config *InterpretConfig) (*InterpretResult, error)
    StrategyType() StrategyType
}
```

**支持的策略**：

| 策略 | 说明 | 使用场景 |
|-----|------|---------|
| threshold | 阈值解读 | 得分超过阈值则为高风险 |
| range | 区间解读 | 根据得分区间确定等级 |
| composite | 组合解读 | 多因子组合判断 |

### 6. 领域服务

#### 6.1 AssessmentCreator（测评创建器）

**职责**：创建测评聚合根，封装跨聚合的验证逻辑，执行答卷提交

```go
type AssessmentCreator interface {
    // Create 创建并提交测评
    // 内部执行：验证 → 创建 Assessment → 提交 → 返回
    Create(ctx context.Context, req *CreateAssessmentRequest) (*Assessment, error)
}

type CreateAssessmentRequest struct {
    OrgID            meta.ID
    TesteeID         testee.ID
    QuestionnaireRef QuestionnaireRef
    AnswerSheetRef   AnswerSheetRef
    MedicalScaleRef  *MedicalScaleRef
    Origin           Origin
    PlanID           *meta.ID
    ProjectID        *meta.ID
}
```

**验证内容**：

- 受试者是否存在
- 问卷是否存在且已发布
- 答卷是否存在且属于该问卷
- 量表是否存在且与问卷关联

#### 6.2 ReportBuilder（报告构建器）

**职责**：将评估结果转换为解读报告

```go
type ReportBuilder interface {
    Build(assessment *Assessment, medicalScale *scale.MedicalScale, result *EvaluationResult) (*InterpretReport, error)
}
```

**设计说明**：

- 这是一个领域服务，因为它需要协调多个聚合根（Assessment、MedicalScale）
- Builder 模式封装了报告创建的复杂逻辑
- 接口定义允许不同的实现策略（如不同量表的报告格式）

### 7. 值对象

- **QuestionnaireRef**：问卷引用
- **AnswerSheetRef**：答卷引用
- **MedicalScaleRef**：量表引用
- **Origin**：业务来源（adhoc/plan/screening）
- **EvaluationResult**：评估结果
- **FactorScoreResult**：因子得分结果
- **RiskWarning**：风险预警信息

### 8. Repository 接口

- **Repository**：Assessment 仓储
- **ScoreRepository**：AssessmentScore 仓储
- **ReportRepository**：InterpretReport 仓储

## 与其他子域的协作

```text
[Survey 子域]         [Scale 子域]        [Actor 子域]
     ↓                     ↓                  ↓
     └──── [Calculation 功能域] ←─────────────┘
                  ↓
           [Interpretation 功能域]
                  ↓
           [Assessment 子域]
             (桥接所有子域)
```

### 数据流示意

```text
1. Survey 域产生答卷 (AnswerSheet)
2. 应用服务调用 Calculation 计算每个 answer 的得分
3. 应用服务调用 Calculation 计算每个 factor 的得分
4. 应用服务调用 Interpretation 生成因子解读
5. Assessment 域记录评估结果 (Assessment + AssessmentScore)
6. Assessment 域生成报告 (InterpretReport)
```

### 引用关系

- 引用 `survey.QuestionnaireID` 和 `survey.AnswerSheetID`
- 引用 `scale.MedicalScaleID`
- 引用 `actor.TesteeID`
- 使用 `calculation` 域进行计分
- 使用 `interpretation` 域进行解读

## 设计原则

1. **单一职责**：计算、解读、测评管理各自独立
2. **无状态设计**：calculation 和 interpretation 为无状态功能域
3. **聚合边界清晰**：三个聚合各司其职，不越界
4. **单向依赖**：依赖其他子域，不被其他子域依赖
5. **事件驱动**：通过领域事件实现异步解耦
6. **策略模式**：支持多种计分和解读方式
7. **类型复用**：复用 scale 子域的 RiskLevel、FactorCode 等类型

## 扩展性

### 新增计分策略

```go
// 1. 创建新策略文件 my_strategy.go
type MyStrategy struct{}

func (s *MyStrategy) Calculate(values []float64, params map[string]string) (float64, error) {
    // 实现计算逻辑
}

func (s *MyStrategy) StrategyType() StrategyType {
    return StrategyTypeMyStrategy
}

// 2. 在 types.go 中添加策略类型常量
const StrategyTypeMyStrategy StrategyType = "my_strategy"

// 3. 在 strategy.go 的 init() 中注册
func init() {
    RegisterStrategy(&MyStrategy{})
}
```

### 新增解读策略

同理，实现 `InterpretStrategy` 接口并注册。

### 新增业务来源

1. 增加 `OriginType` 枚举值
2. 新增对应的创建测评快捷方法

### 新增报告格式

利用 MongoDB 灵活性，可在 InterpretReport 中添加新字段。
