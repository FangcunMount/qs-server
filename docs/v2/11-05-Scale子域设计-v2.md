# 11-05 Scale 子域设计（V2）

> **版本**：V2.0  
> **范围**：问卷&量表 BC 中的 scale 子域  
> **目标**：阐述量表子域的核心职责、MedicalScale 聚合根设计、策略模式+职责链模式实现计分与解读的可扩展设计

---

## 1. Scale 子域的定位与职责

### 1.1 子域边界

**scale 子域关注的核心问题**：

* "量表语义"：量表是什么、测量什么心理/行为维度
* "因子结构"：量表包含哪些维度/因子，每个因子包含哪些题目
* "计分规则"：如何从题目答案计算出原始分、因子分、总分
* "解读规则"：不同分数区间对应什么风险等级、结论文案、建议

**scale 子域不关心的问题**：

* "问卷怎么展示"：这是 survey 子域的职责
* "谁在什么时候做了这个量表"：这是 assessment 子域的职责
* "数据存储细节"：这是基础设施层的职责

### 1.2 核心聚合

scale 子域只有一个核心聚合：

**MedicalScale 聚合**：量表定义
* 量表基本信息（编码、名称、版本）
* 因子结构定义（Factor）
* 计分策略配置（题目级、因子级）
* 解读规则配置（InterpretationRule）

### 1.3 与其他子域的关系

* **依赖** survey 子域：需要读取 Question 和 AnswerSheet 的视图作为计算输入
* **被依赖** 于 assessment 子域：assessment 引用 MedicalScaleID，调用 Evaluator 执行评估

**依赖方向**：

```text
survey (独立)
   ↓ (只读依赖)
 scale (依赖 survey 的 Question/AnswerSheet 视图)
   ↓
assessment (依赖 scale 的 MedicalScale 和 Evaluator)
```

---

## 2. MedicalScale 聚合根

### 2.1 聚合根定位

MedicalScale 是 scale 子域的唯一聚合根，代表一个具体的量表定义（如 SDS、SAS、SCL-90、Conners 等）。

**核心职责**：

1. 管理量表元数据（编码、名称、版本、发布状态）
2. 定义因子结构（维度列表、每个维度包含哪些题目）
3. 配置计分策略（题目级计分方式、因子级聚合方式）
4. 配置解读规则（分数区间 → 风险等级 + 结论文案 + 建议文案）

### 2.2 聚合根结构

```go
// MedicalScale 量表聚合根
type MedicalScale struct {
    // 基本信息
    id          MedicalScaleID
    code        string
    name        string
    version     string
    status      ScaleStatus // draft / published / archived
    
    // 因子结构
    factors     []Factor
    
    // 解读规则
    interpretRules []InterpretationRule
    
    // 元数据
    createdAt   time.Time
    publishedAt *time.Time
    archivedAt  *time.Time
}

// ScaleStatus 量表状态
type ScaleStatus string

const (
    ScaleStatusDraft     ScaleStatus = "draft"     // 草稿
    ScaleStatusPublished ScaleStatus = "published" // 已发布
    ScaleStatusArchived  ScaleStatus = "archived"  // 已归档
)
```

### 2.3 因子（Factor）实体

Factor 是 MedicalScale 内部的实体，代表量表的一个维度/因子。

```go
// Factor 因子（维度）
type Factor struct {
    code           FactorCode
    name           string
    questionCodes  []meta.Code // 该因子包含的题目编码列表
    
    // 因子级计分策略配置
    scoringStrategy FactorScoreStrategyCode
    params          map[string]string
}

// FactorCode 因子编码
type FactorCode string

// FactorScoreStrategyCode 因子计分策略编码
type FactorScoreStrategyCode string

const (
    FactorScoreStrategySum    FactorScoreStrategyCode = "sum"    // 求和
    FactorScoreStrategyAvg    FactorScoreStrategyCode = "avg"    // 平均
    FactorScoreStrategyCustom FactorScoreStrategyCode = "custom" // 自定义（很少用）
)
```

**设计要点**：

* Factor 通过 `questionCodes` 关联题目，是逻辑引用，不直接持有 Question 对象
* Factor 配置自己的计分策略（sum/avg/custom），由工厂模式在运行时转换为策略实例
* Factor 不负责实际计算，只负责存储配置

### 2.4 解读规则（InterpretationRule）值对象

InterpretationRule 是值对象，定义"分数区间 → 风险等级 + 文案"的映射规则。

```go
// InterpretationRule 解读规则
type InterpretationRule struct {
    code       InterpretRuleCode
    factorCode *FactorCode // nil=总分规则, 非nil=因子规则
    scoreRange ScoreRange
    riskLevel  RiskLevel
    conclusion string
    suggestion string
}

// ScoreRange 分数区间 [min, max)
type ScoreRange struct {
    min float64
    max float64
}

// RiskLevel 风险等级
type RiskLevel string

const (
    RiskLevelNone   RiskLevel = "none"   // 无风险
    RiskLevelLow    RiskLevel = "low"    // 低风险
    RiskLevelMedium RiskLevel = "medium" // 中风险
    RiskLevelHigh   RiskLevel = "high"   // 高风险
    RiskLevelSevere RiskLevel = "severe" // 严重
)
```

**设计要点**：

* InterpretationRule 是纯配置，不包含计算逻辑
* 通过 `factorCode` 区分总分规则和因子规则：
  * `factorCode == nil`：针对总分的解读规则
  * `factorCode != nil`：针对特定因子的解读规则
* 分数区间采用左闭右开 `[min, max)` 以避免边界重叠

### 2.5 MedicalScale 核心行为

```go
// 创建量表（工厂方法）
func NewMedicalScale(
    code string,
    name string,
    version string,
    factors []Factor,
    interpretRules []InterpretationRule,
) *MedicalScale

// 发布量表
func (s *MedicalScale) Publish() error

// 归档量表
func (s *MedicalScale) Archive() error

// 查询方法
func (s *MedicalScale) ID() MedicalScaleID
func (s *MedicalScale) Code() string
func (s *MedicalScale) Name() string
func (s *MedicalScale) Version() string
func (s *MedicalScale) Status() ScaleStatus
func (s *MedicalScale) Factors() []Factor
func (s *MedicalScale) InterpretRules() []InterpretationRule

// 查找因子
func (s *MedicalScale) FindFactor(code FactorCode) (*Factor, error)

// 根据分数查找解读规则
func (s *MedicalScale) FindInterpretRule(factorCode *FactorCode, score float64) (*InterpretationRule, error)
```

---

## 3. 策略模式：题目级计分

### 3.1 设计目标

* **80% 通用**：大多数题目使用几种通用策略+参数配置搞定
* **20% 特殊**：极少数特殊量表可以添加新策略实现
* **纯函数**：所有策略都是无状态的，只吃参数，不访问外部系统

### 3.2 题目计分配置（ScoringConfig）

题目的计分配置挂在 survey 子域的 Question 上：

```go
// internal/domain/survey/scoring_config.go
package survey

type ScoreStrategyCode string

const (
    ScoreStrategyNone        ScoreStrategyCode = "none"         // 不计分
    ScoreStrategyOptionMap   ScoreStrategyCode = "option_map"   // 选项映射（Likert量表）
    ScoreStrategyNumberValue ScoreStrategyCode = "number_value" // 数值题直接取数值
    // 未来可扩展...
)

type ScoringConfig struct {
    strategy ScoreStrategyCode
    params   map[string]string // 如 option_scores:"A:0,B:1,C:2,D:3"
}
```

**关键点**：

* ScoringConfig 是值对象，存储在 survey.Question 中
* 配置和执行分离：配置在 survey 子域，执行在 scale 子域

### 3.3 题目计分策略接口

```go
// internal/domain/scale/question_scoring.go
package scale

import "qs-server/internal/domain/survey"

// QuestionScoringStrategy 题目计分策略接口
type QuestionScoringStrategy interface {
    Code() survey.ScoreStrategyCode
    Score(q *survey.Question, answer *survey.Answer) (float64, error)
}
```

**关键点**：

* 策略接口定义在 scale 子域（因为计分是 scale 的职责）
* 依赖 survey 子域的 Question 和 Answer 视图（只读）
* 策略实现必须是纯函数，无状态

### 3.4 通用策略实现

#### 3.4.1 选项映射策略（OptionMapScoring）

适用于 Likert 量表等选项题，每个选项对应一个分数。

```go
// internal/domain/scale/question_scoring_option_map.go
package scale

import (
    "fmt"
    "qs-server/internal/domain/survey"
)

type OptionMapScoring struct {
    scores map[string]float64 // option code -> score
}

func NewOptionMapScoring(scores map[string]float64) *OptionMapScoring {
    return &OptionMapScoring{scores: scores}
}

func (s *OptionMapScoring) Code() survey.ScoreStrategyCode {
    return survey.ScoreStrategyOptionMap
}

func (s *OptionMapScoring) Score(q *survey.Question, answer *survey.Answer) (float64, error) {
    if answer == nil || len(answer.Values()) == 0 {
        return 0, nil
    }
    
    // 单选：取第一个选项的分数
    optionCode := answer.Values()[0]
    score, ok := s.scores[optionCode]
    if !ok {
        return 0, fmt.Errorf("option %s not found in scoring map", optionCode)
    }
    
    return score, nil
}
```

**使用场景**：

* SDS、SAS 等 Likert 量表
* 选项固定对应分数（如 A=1, B=2, C=3, D=4）

#### 3.4.2 数值策略（NumberValueScoring）

适用于数值填空题，直接取用户填写的数值。

```go
// internal/domain/scale/question_scoring_number_value.go
package scale

import (
    "strconv"
    "qs-server/internal/domain/survey"
)

type NumberValueScoring struct{}

func NewNumberValueScoring() *NumberValueScoring {
    return &NumberValueScoring{}
}

func (s *NumberValueScoring) Code() survey.ScoreStrategyCode {
    return survey.ScoreStrategyNumberValue
}

func (s *NumberValueScoring) Score(q *survey.Question, answer *survey.Answer) (float64, error) {
    if answer == nil || len(answer.Values()) == 0 {
        return 0, nil
    }
    
    // 取第一个值并转换为 float64
    valueStr := answer.Values()[0]
    score, err := strconv.ParseFloat(valueStr, 64)
    if err != nil {
        return 0, err
    }
    
    return score, nil
}
```

**使用场景**：

* BMI、年龄、身高、体重等数值型题目

### 3.5 题目计分工厂

```go
// internal/domain/scale/question_scoring_factory.go
package scale

import (
    "fmt"
    "strconv"
    "strings"
    "qs-server/internal/domain/survey"
)

// QuestionScoringFactory 题目计分策略工厂
type QuestionScoringFactory interface {
    FromConfig(cfg *survey.ScoringConfig) (QuestionScoringStrategy, error)
}

type defaultQuestionScoringFactory struct{}

func NewDefaultQuestionScoringFactory() QuestionScoringFactory {
    return &defaultQuestionScoringFactory{}
}

func (f *defaultQuestionScoringFactory) FromConfig(cfg *survey.ScoringConfig) (QuestionScoringStrategy, error) {
    if cfg == nil {
        return nil, nil
    }
    
    switch cfg.Strategy() {
    case survey.ScoreStrategyOptionMap:
        // 解析 option_scores 参数："A:0,B:1,C:2,D:3"
        raw := cfg.Params()["option_scores"]
        scores := make(map[string]float64)
        
        for _, seg := range strings.Split(raw, ",") {
            kv := strings.SplitN(seg, ":", 2)
            if len(kv) != 2 {
                continue
            }
            fv, err := strconv.ParseFloat(kv[1], 64)
            if err != nil {
                continue
            }
            scores[strings.TrimSpace(kv[0])] = fv
        }
        
        return NewOptionMapScoring(scores), nil
        
    case survey.ScoreStrategyNumberValue:
        return NewNumberValueScoring(), nil
        
    case survey.ScoreStrategyNone:
        return nil, nil // 不计分
        
    default:
        return nil, fmt.Errorf("unsupported scoring strategy: %s", cfg.Strategy())
    }
}
```

**关键点**：

* 工厂负责把配置（ScoringConfig）转换为策略实例
* 参数解析逻辑封装在工厂中，策略实现保持纯净
* 新增策略时只需在工厂中添加 case 分支

---

## 4. 策略模式：因子级计分

### 4.1 因子计分策略接口

```go
// internal/domain/scale/factor_scoring.go
package scale

// FactorScoringStrategy 因子计分策略接口
type FactorScoringStrategy interface {
    Code() FactorScoreStrategyCode
    Aggregate(factor Factor, itemScores map[meta.Code]float64) (float64, error)
}
```

**关键点**：

* 输入：Factor 配置 + 该因子下所有题目的分数映射
* 输出：因子总分
* 无状态、纯函数

### 4.2 通用策略实现

#### 4.2.1 求和策略（SumFactorStrategy）

```go
// internal/domain/scale/factor_scoring_sum.go
package scale

type SumFactorStrategy struct{}

func NewSumFactorStrategy() *SumFactorStrategy {
    return &SumFactorStrategy{}
}

func (s *SumFactorStrategy) Code() FactorScoreStrategyCode {
    return FactorScoreStrategySum
}

func (s *SumFactorStrategy) Aggregate(factor Factor, itemScores map[meta.Code]float64) (float64, error) {
    var total float64
    
    for _, qCode := range factor.QuestionCodes() {
        if score, ok := itemScores[qCode]; ok {
            total += score
        }
    }
    
    return total, nil
}
```

#### 4.2.2 平均策略（AvgFactorStrategy）

```go
// internal/domain/scale/factor_scoring_avg.go
package scale

type AvgFactorStrategy struct{}

func NewAvgFactorStrategy() *AvgFactorStrategy {
    return &AvgFactorStrategy{}
}

func (s *AvgFactorStrategy) Code() FactorScoreStrategyCode {
    return FactorScoreStrategyAvg
}

func (s *AvgFactorStrategy) Aggregate(factor Factor, itemScores map[meta.Code]float64) (float64, error) {
    var total float64
    var count int
    
    for _, qCode := range factor.QuestionCodes() {
        if score, ok := itemScores[qCode]; ok {
            total += score
            count++
        }
    }
    
    if count == 0 {
        return 0, nil
    }
    
    return total / float64(count), nil
}
```

### 4.3 因子计分工厂

```go
// internal/domain/scale/factor_scoring_factory.go
package scale

import "fmt"

// FactorScoringFactory 因子计分策略工厂
type FactorScoringFactory interface {
    FromConfig(factor Factor) (FactorScoringStrategy, error)
}

type defaultFactorScoringFactory struct{}

func NewDefaultFactorScoringFactory() FactorScoringFactory {
    return &defaultFactorScoringFactory{}
}

func (f *defaultFactorScoringFactory) FromConfig(factor Factor) (FactorScoringStrategy, error) {
    switch factor.ScoringStrategy() {
    case FactorScoreStrategySum:
        return NewSumFactorStrategy(), nil
        
    case FactorScoreStrategyAvg:
        return NewAvgFactorStrategy(), nil
        
    case FactorScoreStrategyCustom:
        // 自定义策略需要根据 factor.Params() 进一步判断
        // 暂时不支持
        return nil, fmt.Errorf("custom factor scoring strategy not implemented")
        
    default:
        return nil, fmt.Errorf("unsupported factor scoring strategy: %s", factor.ScoringStrategy())
    }
}
```

---

## 5. 职责链模式：Evaluator 评估管道

### 5.1 设计目标

* **可插拔**：评估流程分为多个步骤，每个步骤独立实现
* **顺序执行**：步骤按顺序执行，前一步的结果作为后一步的输入
* **统一上下文**：所有步骤共享同一个上下文对象（EvalContext）
* **无状态**：每个步骤都是无状态的领域服务

### 5.2 评估上下文（EvalContext）

```go
// internal/domain/scale/eval_context.go
package scale

import (
    "qs-server/internal/domain/survey"
    "qs-server/internal/pkg/meta"
)

// EvalContext 评估上下文 - 贯穿整个评估流程
type EvalContext struct {
    // 输入
    Scale         *MedicalScale
    Questionnaire *survey.Questionnaire
    AnswerSheet   *survey.AnswerSheet
    
    // 中间结果
    ItemScores   map[meta.Code]float64 // 每题原始分
    TotalScore   float64               // 总分
    FactorScores []FactorScore         // 因子分列表
    
    // 最终结果
    Result *EvaluationResult
}

// FactorScore 因子分
type FactorScore struct {
    FactorCode FactorCode
    RawScore   float64
    RiskLevel  RiskLevel
}

// EvaluationResult 评估结果
type EvaluationResult struct {
    TotalScore   float64
    RiskLevel    RiskLevel
    FactorScores []FactorScore
    Conclusion   string
    Suggestion   string
}
```

**关键点**：

* EvalContext 是贯穿整个评估流程的共享上下文
* 包含输入（量表、问卷、答卷）、中间结果（题目分、因子分）、最终结果
* 各个步骤通过修改 EvalContext 来传递数据

### 5.3 评估步骤接口（EvalStep）

```go
// internal/domain/scale/eval_step.go
package scale

import "context"

// EvalStep 评估步骤接口
type EvalStep interface {
    Name() string
    Handle(ctx context.Context, evalCtx *EvalContext) error
}
```

**关键点**：

* 每个步骤实现 EvalStep 接口
* Handle 方法接收 context.Context（用于超时控制）和 EvalContext（共享数据）
* 步骤之间通过修改 EvalContext 来通信

### 5.4 步骤 1：原始分计算（RawScoreStep）

```go
// internal/domain/scale/step_raw_score.go
package scale

import (
    "context"
    "qs-server/internal/domain/survey"
)

type RawScoreStep struct {
    qScoreFactory QuestionScoringFactory
}

func NewRawScoreStep(f QuestionScoringFactory) *RawScoreStep {
    return &RawScoreStep{qScoreFactory: f}
}

func (s *RawScoreStep) Name() string {
    return "raw_score"
}

func (s *RawScoreStep) Handle(ctx context.Context, evalCtx *EvalContext) error {
    var total float64
    itemScores := make(map[meta.Code]float64)
    
    // 遍历问卷中的所有题目
    for _, q := range evalCtx.Questionnaire.Questions() {
        // 查找该题目的答案
        answer := evalCtx.AnswerSheet.FindAnswer(q.Code())
        if answer == nil {
            continue // 未作答，跳过
        }
        
        // 获取题目的计分配置
        scoringCfg := q.ScoringConfig()
        if scoringCfg == nil {
            continue // 无计分配置，跳过
        }
        
        // 通过工厂获取计分策略
        strategy, err := s.qScoreFactory.FromConfig(scoringCfg)
        if err != nil {
            return err
        }
        if strategy == nil {
            continue // 不计分策略，跳过
        }
        
        // 执行计分
        score, err := strategy.Score(&q, answer)
        if err != nil {
            return err
        }
        
        // 记录题目分数
        itemScores[q.Code()] = score
        total += score
    }
    
    // 写入上下文
    evalCtx.ItemScores = itemScores
    evalCtx.TotalScore = total
    
    return nil
}
```

**职责**：

* 遍历所有题目，根据题目的 ScoringConfig 选择计分策略
* 计算每道题的原始分
* 汇总总分
* 将结果写入 EvalContext

### 5.5 步骤 2：因子分计算（FactorScoreStep）

```go
// internal/domain/scale/step_factor_score.go
package scale

import "context"

type FactorScoreStep struct {
    factorFactory FactorScoringFactory
}

func NewFactorScoreStep(f FactorScoringFactory) *FactorScoreStep {
    return &FactorScoreStep{factorFactory: f}
}

func (s *FactorScoreStep) Name() string {
    return "factor_score"
}

func (s *FactorScoreStep) Handle(ctx context.Context, evalCtx *EvalContext) error {
    var factorScores []FactorScore
    
    // 遍历量表中的所有因子
    for _, factor := range evalCtx.Scale.Factors() {
        // 通过工厂获取因子计分策略
        strategy, err := s.factorFactory.FromConfig(factor)
        if err != nil {
            return err
        }
        if strategy == nil {
            // 默认使用求和策略
            strategy = NewSumFactorStrategy()
        }
        
        // 聚合该因子下所有题目的分数
        score, err := strategy.Aggregate(factor, evalCtx.ItemScores)
        if err != nil {
            return err
        }
        
        // 判断因子风险等级
        risk := s.judgeFactorRisk(evalCtx.Scale, factor.Code(), score)
        
        // 记录因子分数
        factorScores = append(factorScores, FactorScore{
            FactorCode: factor.Code(),
            RawScore:   score,
            RiskLevel:  risk,
        })
    }
    
    // 写入上下文
    evalCtx.FactorScores = factorScores
    
    return nil
}

func (s *FactorScoreStep) judgeFactorRisk(scale *MedicalScale, factorCode FactorCode, score float64) RiskLevel {
    // 查找该因子的解读规则
    rule, err := scale.FindInterpretRule(&factorCode, score)
    if err != nil || rule == nil {
        return RiskLevelNone
    }
    return rule.RiskLevel
}
```

**职责**：

* 遍历量表的所有因子
* 根据因子的 ScoringStrategy 选择聚合策略
* 计算每个因子的分数
* 根据解读规则判断因子风险等级
* 将结果写入 EvalContext

### 5.6 步骤 3：总体解读（OverallInterpretStep）

```go
// internal/domain/scale/step_overall_interpret.go
package scale

import "context"

type OverallInterpretStep struct{}

func NewOverallInterpretStep() *OverallInterpretStep {
    return &OverallInterpretStep{}
}

func (s *OverallInterpretStep) Name() string {
    return "overall_interpret"
}

func (s *OverallInterpretStep) Handle(ctx context.Context, evalCtx *EvalContext) error {
    // 根据总分查找总体解读规则
    rule, err := evalCtx.Scale.FindInterpretRule(nil, evalCtx.TotalScore)
    if err != nil || rule == nil {
        // 未找到规则，使用默认
        evalCtx.Result = &EvaluationResult{
            TotalScore:   evalCtx.TotalScore,
            RiskLevel:    RiskLevelNone,
            FactorScores: evalCtx.FactorScores,
            Conclusion:   "未找到对应的解读规则",
            Suggestion:   "",
        }
        return nil
    }
    
    // 构建最终评估结果
    evalCtx.Result = &EvaluationResult{
        TotalScore:   evalCtx.TotalScore,
        RiskLevel:    rule.RiskLevel,
        FactorScores: evalCtx.FactorScores,
        Conclusion:   rule.Conclusion,
        Suggestion:   rule.Suggestion,
    }
    
    return nil
}
```

**职责**：

* 根据总分查找总体解读规则
* 获取风险等级、结论文案、建议文案
* 构建最终的 EvaluationResult
* 将结果写入 EvalContext

### 5.7 职责链组装：ChainEvaluator

```go
// internal/domain/scale/evaluator_chain.go
package scale

import (
    "context"
    "qs-server/internal/domain/survey"
)

// Evaluator 评估器接口
type Evaluator interface {
    Evaluate(
        ctx context.Context,
        scale *MedicalScale,
        questionnaire *survey.Questionnaire,
        answerSheet *survey.AnswerSheet,
    ) (*EvaluationResult, error)
}

// ChainEvaluator 职责链评估器
type ChainEvaluator struct {
    steps []EvalStep
}

func NewChainEvaluator(steps ...EvalStep) *ChainEvaluator {
    return &ChainEvaluator{steps: steps}
}

func (e *ChainEvaluator) Evaluate(
    ctx context.Context,
    scale *MedicalScale,
    questionnaire *survey.Questionnaire,
    answerSheet *survey.AnswerSheet,
) (*EvaluationResult, error) {
    // 初始化评估上下文
    evalCtx := &EvalContext{
        Scale:         scale,
        Questionnaire: questionnaire,
        AnswerSheet:   answerSheet,
        ItemScores:    make(map[meta.Code]float64),
    }
    
    // 依次执行所有步骤
    for _, step := range e.steps {
        if err := step.Handle(ctx, evalCtx); err != nil {
            return nil, err
        }
    }
    
    // 返回最终结果
    return evalCtx.Result, nil
}
```

**关键点**：

* ChainEvaluator 实现 Evaluator 接口
* 内部持有一个步骤列表（steps）
* Evaluate 方法按顺序执行所有步骤
* 各步骤通过共享的 EvalContext 传递数据

### 5.8 Evaluator 组装示例

```go
// 在应用层或容器层组装 Evaluator
func NewDefaultEvaluator() scale.Evaluator {
    qScoreFactory := scale.NewDefaultQuestionScoringFactory()
    factorFactory := scale.NewDefaultFactorScoringFactory()
    
    return scale.NewChainEvaluator(
        scale.NewRawScoreStep(qScoreFactory),
        scale.NewFactorScoreStep(factorFactory),
        scale.NewOverallInterpretStep(),
    )
}
```

---

## 6. 扩展性保证

### 6.1 题目计分扩展

**新增题目计分策略的步骤**：

1. 在 survey 子域增加新的 `ScoreStrategyCode` 常量
2. 在 scale 子域实现新的 `QuestionScoringStrategy` 接口
3. 在 `QuestionScoringFactory` 的 `FromConfig` 方法中添加新策略的创建逻辑

**示例：新增多选题计分策略**

```go
// 1. 在 survey 子域定义策略编码
const ScoreStrategyMultiOption ScoreStrategyCode = "multi_option"

// 2. 在 scale 子域实现策略
type MultiOptionScoring struct {
    scores map[string]float64
}

func (s *MultiOptionScoring) Score(q *survey.Question, answer *survey.Answer) (float64, error) {
    var total float64
    for _, optionCode := range answer.Values() {
        if score, ok := s.scores[optionCode]; ok {
            total += score
        }
    }
    return total, nil
}

// 3. 在工厂中添加创建逻辑
case survey.ScoreStrategyMultiOption:
    // 解析参数并创建策略实例
    return NewMultiOptionScoring(scores), nil
```

### 6.2 因子计分扩展

**新增因子计分策略的步骤**：

1. 定义新的 `FactorScoreStrategyCode` 常量
2. 实现新的 `FactorScoringStrategy` 接口
3. 在 `FactorScoringFactory` 中添加新策略的创建逻辑

**示例：新增加权求和策略**

```go
// 1. 定义策略编码
const FactorScoreStrategyWeightedSum FactorScoreStrategyCode = "weighted_sum"

// 2. 实现策略
type WeightedSumFactorStrategy struct {
    weights map[meta.Code]float64
}

func (s *WeightedSumFactorStrategy) Aggregate(factor Factor, itemScores map[meta.Code]float64) (float64, error) {
    var total float64
    for _, qCode := range factor.QuestionCodes() {
        if score, ok := itemScores[qCode]; ok {
            weight := s.weights[qCode]
            if weight == 0 {
                weight = 1.0
            }
            total += score * weight
        }
    }
    return total, nil
}

// 3. 在工厂中添加
case FactorScoreStrategyWeightedSum:
    // 从 factor.Params() 中解析权重配置
    return NewWeightedSumFactorStrategy(weights), nil
```

### 6.3 评估步骤扩展

**新增评估步骤的步骤**：

1. 实现新的 `EvalStep` 接口
2. 在组装 ChainEvaluator 时插入新步骤

**示例：新增标准分转换步骤**

```go
// 1. 实现步骤
type StandardScoreStep struct{}

func (s *StandardScoreStep) Name() string {
    return "standard_score"
}

func (s *StandardScoreStep) Handle(ctx context.Context, evalCtx *EvalContext) error {
    // 将原始分转换为标准分（Z分数、T分数等）
    for i, fs := range evalCtx.FactorScores {
        // 根据常模计算标准分
        standardScore := convertToStandardScore(fs.RawScore, norm)
        // 更新因子分数（如果需要扩展 FactorScore 结构）
    }
    return nil
}

// 2. 在组装时插入
func NewDefaultEvaluator() scale.Evaluator {
    return scale.NewChainEvaluator(
        scale.NewRawScoreStep(qScoreFactory),
        scale.NewFactorScoreStep(factorFactory),
        scale.NewStandardScoreStep(),        // 新增步骤
        scale.NewOverallInterpretStep(),
    )
}
```

---

## 7. 与其他子域的协作

### 7.1 与 survey 子域的协作

scale 子域需要读取 survey 子域的数据作为计算输入：

```go
// scale 依赖 survey 的视图
type Question interface {
    Code() meta.Code
    ScoringConfig() *ScoringConfig
    // ...
}

type Answer interface {
    QuestionCode() meta.Code
    Values() []string
    // ...
}

type AnswerSheet interface {
    FindAnswer(questionCode meta.Code) *Answer
    // ...
}
```

**关键点**：

* scale 只读取 survey 的数据，不修改
* 依赖接口而非具体实现，保持松耦合

### 7.2 与 assessment 子域的协作

assessment 子域调用 scale 的 Evaluator 完成评估：

```go
// 在 assessment 应用层或领域服务中
func (s *AssessmentService) EvaluateAssessment(ctx context.Context, assessmentID AssessmentID) error {
    // 1. 加载数据
    assessment, _ := s.assessmentRepo.FindByID(ctx, assessmentID)
    scale, _ := s.scaleRepo.FindByID(ctx, assessment.MedicalScaleID())
    questionnaire, _ := s.questionnaireRepo.FindByID(ctx, assessment.QuestionnaireID())
    answerSheet, _ := s.answerSheetRepo.FindByID(ctx, assessment.AnswerSheetID())
    
    // 2. 调用 Evaluator
    result, err := s.evaluator.Evaluate(ctx, scale, questionnaire, answerSheet)
    if err != nil {
        return err
    }
    
    // 3. 更新 Assessment
    assessment.ApplyEvaluation(result.TotalScore, result.RiskLevel)
    s.assessmentRepo.Save(ctx, assessment)
    
    // 4. 生成 AssessmentScore 和 InterpretReport
    // ...
    
    return nil
}
```

**关键点**：

* assessment 子域编排整个评估流程
* scale 子域只负责"给定输入，计算输出"
* 结果的持久化由 assessment 子域负责

---

## 8. 目录结构

```text
internal/domain/scale/
├── medical_scale.go              // MedicalScale 聚合根
├── factor.go                     // Factor 实体
├── interpretation_rule.go        // InterpretationRule 值对象
├── types.go                      // 类型定义（ID、枚举等）
├── repository.go                 // Repository 接口
│
├── question_scoring.go           // 题目计分策略接口
├── question_scoring_option_map.go    // 选项映射策略
├── question_scoring_number_value.go  // 数值策略
├── question_scoring_factory.go       // 题目计分工厂
│
├── factor_scoring.go             // 因子计分策略接口
├── factor_scoring_sum.go         // 求和策略
├── factor_scoring_avg.go         // 平均策略
├── factor_scoring_factory.go    // 因子计分工厂
│
├── evaluator.go                  // Evaluator 接口
├── evaluator_chain.go            // ChainEvaluator 实现
├── eval_context.go               // EvalContext
├── eval_step.go                  // EvalStep 接口
├── step_raw_score.go             // 原始分步骤
├── step_factor_score.go          // 因子分步骤
├── step_overall_interpret.go     // 总体解读步骤
```

---

## 9. 总结

本文档详细阐述了 scale 子域的设计，核心要点包括：

### 9.1 MedicalScale 聚合根

* 唯一聚合根，代表量表定义
* 包含因子结构（Factor）和解读规则（InterpretationRule）
* 提供查询接口，支持查找因子和解读规则

### 9.2 策略模式：题目级计分

* **配置和执行分离**：ScoringConfig 存储在 survey.Question 中，策略实现在 scale 子域
* **QuestionScoringStrategy 接口**：定义题目计分策略的统一接口
* **通用策略实现**：OptionMapScoring（选项映射）、NumberValueScoring（数值）
* **QuestionScoringFactory**：负责把配置转换为策略实例

### 9.3 策略模式：因子级计分

* **FactorScoringStrategy 接口**：定义因子聚合策略的统一接口
* **通用策略实现**：SumFactorStrategy（求和）、AvgFactorStrategy（平均）
* **FactorScoringFactory**：负责把因子配置转换为策略实例

### 9.4 职责链模式：Evaluator 评估管道

* **EvalContext**：贯穿整个评估流程的共享上下文
* **EvalStep 接口**：定义评估步骤的统一接口
* **三个核心步骤**：
  1. RawScoreStep：计算每题原始分
  2. FactorScoreStep：聚合因子分
  3. OverallInterpretStep：生成总体解读
* **ChainEvaluator**：按顺序执行所有步骤，输出 EvaluationResult

### 9.5 扩展性保证

* **题目计分**：新增策略只需实现接口并在工厂中注册
* **因子计分**：新增策略只需实现接口并在工厂中注册
* **评估步骤**：新增步骤只需实现 EvalStep 接口并插入职责链
* **80/20 原则**：80% 场景用通用策略+参数，20% 特殊场景扩展新策略

### 9.6 设计原则

* **配置和执行分离**：配置存储在聚合中，执行逻辑在策略中
* **纯函数/无状态**：所有策略都是纯函数，不访问外部系统
* **单向依赖**：scale 依赖 survey（只读），不产生循环依赖
* **职责单一**：scale 只负责"计算和解读"，不负责"收集和持久化"

---

## 附录：与相关文档的关系

* **《11-01-问卷&量表 BC 领域模型总览-v2.md》**：定义了 scale 子域在整个 BC 中的定位
* **《11-02-qs-apiserver 领域层代码结构设计-v2.md》**：定义了 scale 子域的目录结构
* **《11-04-Survey 子域设计-v2.md》**：定义了 survey 子域的 Question 和 AnswerSheet，是 scale 的输入
* **《12-03-评估工作流与 qs-worker 设计-v2.md》**：描述了 assessment 子域如何调用 scale.Evaluator 完成评估
