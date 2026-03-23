# 11-06-04 Interpretation 解读策略设计

> **版本**：V2.0  
> **最后更新**：2025-11-29  
> **状态**：✅ 已实现并验证

---

## 📋 文档导航

**当前位置**：Interpretation 解读策略设计（你在这里）  
**前置阅读**：[11-06-03 Calculation计算策略设计](./11-06-03-Calculation计算策略设计.md)  
**后续阅读**：[11-06-05 Report聚合设计](./11-06-05-Report聚合设计.md)

---

## 🎯 核心设计思想（30秒速览）

> **如果只有30秒，你需要知道这些：**

```text
┌────────────────────────────────────────────────────────────────────────────┐
│                                                                            │
│   Interpretation = 解读引擎 = "把分数变成结论"                             │
│                                                                            │
│   ┌─────────────┐     ┌─────────────────┐     ┌─────────────────────┐      │
│   │   因子得分   │ ──▶ │   解读策略引擎   │ ──▶ │  风险等级 + 结论    │      │
│   │  score=68   │     │   (3种策略)      │     │  "中度风险"         │      │
│   └─────────────┘     └─────────────────┘     └─────────────────────┘      │
│                              │                                             │
│              ┌───────────────┼───────────────┐                             │
│              ▼               ▼               ▼                             │
│       ┌──────────┐    ┌──────────┐    ┌──────────┐                         │
│       │ 阈值策略 │    │ 区间策略 │    │ 组合策略 │                         │
│       │threshold │    │  range   │    │composite │                         │
│       └──────────┘    └──────────┘    └──────────┘                         │
│                                                                            │
├────────────────────────────────────────────────────────────────────────────┤
│  核心设计模式：                                                            │
│    ✓ 策略模式 - 3种解读策略，适应不同量表需求                             │
│    ✓ 规则引擎 - 配置化规则，支持区间、阈值、组合条件                      │
│    ✓ 无状态设计 - 纯函数式解读，线程安全                                  │
│    ✓ 值对象 - ScoreRange、InterpretResult 不可变                          │
└────────────────────────────────────────────────────────────────────────────┘
```

---

## 一、为什么需要 Interpretation？（问题域）

### 1.1 业务场景：小明的测评结果解读

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│  场景：小明完成抑郁自评量表(SDS)，计算得到各因子得分                       │
│                                                                             │
│  输入（来自 Calculation）：                                                 │
│    ┌─────────────────┬──────────┐                                          │
│    │     因子名称     │   得分   │                                          │
│    ├─────────────────┼──────────┤                                          │
│    │ 精神情感症状     │   28     │                                          │
│    │ 躯体症状         │   22     │                                          │
│    │ 精神运动障碍     │   18     │                                          │
│    │ 抑郁心理障碍     │   20     │                                          │
│    │ 标准分（总分）   │   68     │                                          │
│    └─────────────────┴──────────┘                                          │
│                                                                             │
│  问题：这些数字对小明来说毫无意义！                                         │
│    - 68分是高还是低？                                                      │
│    - 哪个因子需要关注？                                                    │
│    - 应该怎么办？                                                          │
│                                                                             │
│  期望输出（有业务含义）：                                                   │
│    ┌─────────────────┬──────────┬──────────┬─────────────────────────────┐ │
│    │     因子名称     │   得分   │ 风险等级 │           结论             │ │
│    ├─────────────────┼──────────┼──────────┼─────────────────────────────┤ │
│    │ 精神情感症状     │   28     │ 🔴 高    │ 存在明显情感困扰，建议咨询 │ │
│    │ 躯体症状         │   22     │ 🟡 中    │ 轻微躯体不适，注意休息     │ │
│    │ 精神运动障碍     │   18     │ 🟢 低    │ 正常范围                   │ │
│    │ 抑郁心理障碍     │   20     │ 🟡 中    │ 存在消极思维，建议关注     │ │
│    │ 标准分（总分）   │   68     │ 🔴 中度  │ 中度抑郁倾向               │ │
│    └─────────────────┴──────────┴──────────┴─────────────────────────────┘ │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 1.2 问题拆解与解决方案

| 问题 | 挑战 | Interpretation 的解决方案 |
| ------ | ------ | -------------------------- |
| **解读规则多样** | 不同量表有不同解读标准 | **策略模式** - 阈值/区间/组合 |
| **区间判断复杂** | 分数落在哪个区间？边界如何处理？ | **ScoreRange 值对象** - 左闭右开 |
| **多因子联动** | 单因子正常但组合异常 | **组合策略** - AND/OR 逻辑 |
| **结果结构化** | 输出需包含等级、标签、建议 | **InterpretResult** - 完整结论 |

### 1.3 职责边界

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                      Interpretation 功能域边界                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ✅ 我负责的：                        ❌ 我不关心的：                       │
│    • 分数到风险等级的映射               • 分数如何计算（Calculation）       │
│    • 区间判断与阈值比较                 • 结果如何存储（Assessment）        │
│    • 多因子组合解读                     • 报告如何生成（Report）            │
│    • 输出结构化解读结论                 • 规则配置来自哪里（Scale）         │
│                                                                             │
│  数据流：                                                                   │
│    Calculation ──▶ 【Interpretation】 ──▶ Report                           │
│     (因子得分)        (解读结论)           (报告内容)                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 二、在评估流程中的位置（是什么？）

### 2.1 评估流程全景图

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                          评估流程全景图                                     │
│                                                                             │
│   Phase 1: 数据准备                                                         │
│   ─────────────────────────────────────────────────────────────────────────│
│     AnswerSheet ────▶ 答案数据                                             │
│     MedicalScale ───▶ 因子配置 + 解读规则                                  │
│                                                                             │
│                              ▼                                              │
│   Phase 2: 分数计算 (Calculation)                                          │
│   ─────────────────────────────────────────────────────────────────────────│
│     答案 + 计算策略 ────▶ 因子得分列表                                     │
│     [2,3,1,4,2] + sum ──▶ [{F1:28}, {F2:22}, ...]                         │
│                                                                             │
│                              ▼                                              │
│   Phase 3: 结果解读 ◀─────── 【你在这里：Interpretation】                  │
│   ─────────────────────────────────────────────────────────────────────────│
│                                                                             │
│     for each FactorScore:                                                   │
│       1. 获取该因子的解读配置（规则列表）                                   │
│       2. 获取对应解读策略（阈值/区间/组合）                                │
│       3. 执行解读，输出风险等级 + 结论                                     │
│                                                                             │
│     输出：                                                                  │
│       [{F1:28, "高风险", "存在明显抑郁倾向"},                             │
│        {F2:22, "中风险", "轻微不适，建议关注"}, ...]                       │
│                                                                             │
│                              ▼                                              │
│   Phase 4: 报告生成 (Report)                                               │
│   ─────────────────────────────────────────────────────────────────────────│
│     解读结果 ────▶ 结构化报告 ────▶ 导出（PDF/Word）                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Interpretation 内部结构

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                       interpretation/（功能域）                             │
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐  │
│   │  strategy.go                          策略接口 + 注册表              │  │
│   │  ┌───────────────────────────────────────────────────────────────┐  │  │
│   │  │  type InterpretStrategy interface {                           │  │  │
│   │  │      Interpret(score, config) (*InterpretResult, error)       │  │  │
│   │  │      StrategyType() StrategyType                              │  │  │
│   │  │  }                                                            │  │  │
│   │  │                                                               │  │  │
│   │  │  type CompositeStrategy interface {                           │  │  │
│   │  │      InterpretMultiple(scores[], config) (*CompositeResult)   │  │  │
│   │  │  }                                                            │  │  │
│   │  └───────────────────────────────────────────────────────────────┘  │  │
│   └─────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐  │
│   │  types.go                             类型与值对象                   │  │
│   │  ┌───────────────────────────────────────────────────────────────┐  │  │
│   │  │  StrategyTypeThreshold   = "threshold"   // 阈值策略          │  │  │
│   │  │  StrategyTypeRange       = "range"       // 区间策略          │  │  │
│   │  │  StrategyTypeComposite   = "composite"   // 组合策略          │  │  │
│   │  │                                                               │  │  │
│   │  │  type InterpretRule { Min, Max, RiskLevel, Label, ... }       │  │  │
│   │  │  type InterpretResult { Score, RiskLevel, Description, ... }  │  │  │
│   │  │  type ScoreRange { minScore, maxScore }                       │  │  │
│   │  └───────────────────────────────────────────────────────────────┘  │  │
│   └─────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│   ┌────────────────────┬────────────────────┬────────────────────────┐    │
│   │   threshold.go     │     range.go       │     composite.go       │    │
│   │   阈值策略         │     区间策略        │     组合策略           │    │
│   │   score > 阈值?    │   score ∈ [a,b)?   │  F1>x AND F2>y?       │    │
│   └────────────────────┴────────────────────┴────────────────────────┘    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 三、策略模式详解（怎么做？）

### 3.1 设计模式：策略模式 + 规则引擎

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                          策略模式 UML 类图                                  │
│                                                                             │
│             ┌─────────────────────────┐                                     │
│             │    <<interface>>        │                                     │
│             │   InterpretStrategy     │                                     │
│             ├─────────────────────────┤                                     │
│             │ +Interpret(score,config)│                                     │
│             │ +StrategyType()         │                                     │
│             └───────────┬─────────────┘                                     │
│                         │                                                   │
│         ┌───────────────┼───────────────┐                                   │
│         │               │               │                                   │
│         ▼               ▼               ▼                                   │
│   ┌───────────┐   ┌───────────┐   ┌───────────────┐                        │
│   │ Threshold │   │   Range   │   │  Composite    │                        │
│   │ Strategy  │   │ Strategy  │   │  Strategy     │                        │
│   ├───────────┤   ├───────────┤   ├───────────────┤                        │
│   │score>阈值?│   │score∈区间?│   │多因子组合判断 │                        │
│   └───────────┘   └───────────┘   └───────────────┘                        │
│                                                                             │
│   配置驱动（规则引擎）：                                                    │
│   ┌─────────────────────────────────────────────────────────────────────┐  │
│   │  InterpretConfig                                                    │  │
│   │  ┌───────────────────────────────────────────────────────────────┐  │  │
│   │  │  Rules: [                                                     │  │  │
│   │  │    { Min: 0,  Max: 53, RiskLevel: "none",   Label: "正常" },  │  │  │
│   │  │    { Min: 53, Max: 63, RiskLevel: "low",    Label: "轻度" },  │  │  │
│   │  │    { Min: 63, Max: 73, RiskLevel: "medium", Label: "中度" },  │  │  │
│   │  │    { Min: 73, Max: 100,RiskLevel: "high",   Label: "重度" },  │  │  │
│   │  │  ]                                                            │  │  │
│   │  └───────────────────────────────────────────────────────────────┘  │  │
│   └─────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.2 策略接口定义

```go
// 代码位置：internal/apiserver/domain/evaluation/interpretation/strategy.go

// InterpretStrategy 解读策略接口
// 设计原则：无状态，纯函数式解读
type InterpretStrategy interface {
    // Interpret 执行解读
    // score: 待解读的得分
    // config: 解读配置（包含规则列表）
    Interpret(score float64, config *InterpretConfig) (*InterpretResult, error)
    
    // StrategyType 返回策略类型标识
    StrategyType() StrategyType
}

// CompositeStrategy 组合解读策略接口（多因子）
type CompositeStrategy interface {
    // InterpretMultiple 多因子组合解读
    InterpretMultiple(scores []FactorScore, config *CompositeConfig) (*CompositeResult, error)
    StrategyType() StrategyType
}
```

> 📎 **完整代码**：[strategy.go](../../../internal/apiserver/domain/evaluation/interpretation/strategy.go)

---

## 四、3种解读策略详解

### 4.1 策略一览表

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                           3种解读策略总览                                   │
│                                                                             │
│  ┌───────────┬────────────────────────┬────────────────────────────────┐   │
│  │   策略    │        判断逻辑         │          典型场景              │   │
│  ├───────────┼────────────────────────┼────────────────────────────────┤   │
│  │ threshold │ score > 阈值 ? 高 : 低  │ 简单二分法（如：是否及格）     │   │
│  │   range   │ score ∈ [a,b) → 等级   │ 多级别划分（如：SDS四级）      │   │
│  │ composite │ F1>x AND F2>y → 结论   │ 多因子联动（如：焦虑+抑郁共病）│   │
│  └───────────┴────────────────────────┴────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 阈值策略 (Threshold)

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                           阈值策略示意图                                    │
│                                                                             │
│   配置：threshold = 60                                                      │
│                                                                             │
│   得分轴：                                                                  │
│     0 ──────────────────── 60 ──────────────────── 100                     │
│           │                 ▲                 │                             │
│           │                 │                 │                             │
│           ▼                 │                 ▼                             │
│      ┌─────────┐       阈值线            ┌─────────┐                       │
│      │ 正常    │                         │ 高风险  │                       │
│      │ RiskNone│                         │ RiskHigh│                       │
│      └─────────┘                         └─────────┘                       │
│                                                                             │
│   输入：score = 68                                                         │
│   判断：68 > 60 → true                                                     │
│   输出：RiskLevel = "high", Label = "高风险"                               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

```go
// ThresholdStrategy 阈值解读策略
type ThresholdStrategy struct{}

func (s *ThresholdStrategy) Interpret(score float64, config *InterpretConfig) (*InterpretResult, error) {
    threshold := parseThreshold(config.Params) // 从配置获取阈值
    
    if score > threshold {
        return buildResult(config.Rules[1]) // 高风险规则
    }
    return buildResult(config.Rules[0]) // 正常规则
}
```

> 📎 **完整代码**：[threshold.go](../../../internal/apiserver/domain/evaluation/interpretation/threshold.go)

### 4.3 区间策略 (Range) ⭐ 最常用

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                           区间策略示意图                                    │
│                                                                             │
│   配置（SDS抑郁自评量表标准分）：                                           │
│                                                                             │
│   得分轴：                                                                  │
│     0 ────── 53 ────── 63 ────── 73 ────── 100                             │
│     │        │         │         │         │                                │
│     │ [0,53) │ [53,63) │ [63,73) │ [73,100]│                               │
│     │        │         │         │         │                                │
│     ▼        ▼         ▼         ▼         ▼                                │
│   ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐                                      │
│   │ 正常 │ │ 轻度 │ │ 中度 │ │ 重度 │                                      │
│   │ none │ │ low  │ │medium│ │ high │                                      │
│   └──────┘ └──────┘ └──────┘ └──────┘                                      │
│                                                                             │
│   输入：score = 68                                                         │
│   判断：68 ∈ [63, 73) → true                                               │
│   输出：RiskLevel = "medium", Label = "中度抑郁"                           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

```go
// RangeStrategy 区间解读策略
type RangeStrategy struct{}

func (s *RangeStrategy) Interpret(score float64, config *InterpretConfig) (*InterpretResult, error) {
    // 遍历规则列表，找到分数所在区间
    for _, rule := range config.Rules {
        if rule.Contains(score) {  // score >= Min && score < Max
            return &InterpretResult{
                FactorCode:  config.FactorCode,
                Score:       score,
                RiskLevel:   rule.RiskLevel,
                Label:       rule.Label,
                Description: rule.Description,
                Suggestion:  rule.Suggestion,
            }, nil
        }
    }
    // 未匹配，使用最后一条规则
    return buildResult(config.Rules[len(config.Rules)-1])
}
```

> 📎 **完整代码**：[threshold.go](../../../internal/apiserver/domain/evaluation/interpretation/threshold.go)（RangeStrategy 在同一文件）

### 4.4 组合策略 (Composite)

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                          组合策略示意图                                     │
│                                                                             │
│   场景：焦虑-抑郁共病判断                                                   │
│                                                                             │
│   规则配置：                                                                │
│     Rule 1: 焦虑因子 > 60 AND 抑郁因子 > 60  →  "共病高风险"               │
│     Rule 2: 焦虑因子 > 60 OR  抑郁因子 > 60  →  "单项风险"                 │
│     Rule 3: 默认                            →  "正常"                      │
│                                                                             │
│   ┌────────────────────────────────────────────────────────────────────┐   │
│   │  输入：                                                            │   │
│   │    焦虑因子 = 72                                                   │   │
│   │    抑郁因子 = 68                                                   │   │
│   │                                                                    │   │
│   │  判断流程：                                                        │   │
│   │    Rule 1: 72 > 60 ✓ AND 68 > 60 ✓  →  匹配！                     │   │
│   │                                                                    │   │
│   │  输出：                                                            │   │
│   │    RiskLevel = "high"                                              │   │
│   │    Label = "焦虑-抑郁共病"                                         │   │
│   │    Description = "同时存在明显焦虑和抑郁症状，建议及时就医"        │   │
│   │                                                                    │   │
│   └────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

```go
// CompositeStrategyImpl 组合解读策略
type CompositeStrategyImpl struct{}

func (s *CompositeStrategyImpl) InterpretMultiple(
    scores []FactorScore, 
    config *CompositeConfig,
) (*CompositeResult, error) {
    // 构建因子得分映射
    scoreMap := make(map[string]float64)
    for _, fs := range scores {
        scoreMap[fs.FactorCode] = fs.Score
    }
    
    // 遍历规则，找到第一个匹配的
    for _, rule := range config.Rules {
        if s.matchRule(rule, scoreMap) {
            return &CompositeResult{
                RiskLevel:   rule.RiskLevel,
                Label:       rule.Label,
                Description: rule.Description,
            }, nil
        }
    }
    return defaultResult()
}

// matchRule 检查规则是否匹配
func (s *CompositeStrategyImpl) matchRule(rule CompositeRule, scoreMap map[string]float64) bool {
    switch rule.Operator {
    case "and":
        // 所有条件都必须满足
        for _, cond := range rule.Conditions {
            if !s.matchCondition(cond, scoreMap) {
                return false
            }
        }
        return true
    case "or":
        // 任一条件满足即可
        for _, cond := range rule.Conditions {
            if s.matchCondition(cond, scoreMap) {
                return true
            }
        }
        return false
    }
    return false
}
```

> 📎 **完整代码**：[composite.go](../../../internal/apiserver/domain/evaluation/interpretation/composite.go)

---

## 五、核心值对象设计

### 5.1 ScoreRange（分数区间）

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                        ScoreRange 值对象设计                                │
│                                                                             │
│   设计决策：采用【左闭右开】区间 [min, max)                                 │
│                                                                             │
│   原因：                                                                    │
│     ✓ 区间天然连续，无重叠无遗漏                                           │
│     ✓ 边界判断清晰：score >= min && score < max                           │
│     ✓ 业界惯例（如 Go 的 slice、Python 的 range）                          │
│                                                                             │
│   示例：                                                                    │
│     [0, 53)  [53, 63)  [63, 73)  [73, 100)                                 │
│        │        │         │         │                                       │
│        └────────┴─────────┴─────────┘                                       │
│              完美衔接，无缝覆盖 0-100                                       │
│                                                                             │
│   ⚠️ 注意最后一个区间：如果需要包含100，使用 [73, 101) 或特殊处理          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

```go
// ScoreRange 分数范围值对象
type ScoreRange struct {
    minScore float64
    maxScore float64
}

// Contains 检查分数是否在范围内（左闭右开）
func (sr ScoreRange) Contains(score float64) bool {
    return score >= sr.minScore && score < sr.maxScore
}

// IsOverlapping 检查是否与另一区间重叠
func (sr ScoreRange) IsOverlapping(other ScoreRange) bool {
    if sr.maxScore == other.minScore || sr.minScore == other.maxScore {
        return false // 边界相邻不算重叠
    }
    return sr.minScore < other.maxScore && sr.maxScore > other.minScore
}
```

> 📎 **完整代码**：[types.go](../../../internal/apiserver/domain/evaluation/interpretation/types.go)

### 5.2 InterpretResult（解读结果）

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                       InterpretResult 结构设计                              │
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐  │
│   │  InterpretResult {                                                  │  │
│   │      FactorCode:  "sds_total"         // 因子编码                   │  │
│   │      Score:       68                  // 原始得分                   │  │
│   │      RiskLevel:   "medium"            // 风险等级                   │  │
│   │      Label:       "中度抑郁"          // 简短标签（用于UI展示）     │  │
│   │      Description: "存在明显抑郁..."   // 详细描述                   │  │
│   │      Suggestion:  "建议寻求专业..."   // 建议措施                   │  │
│   │  }                                                                  │  │
│   └─────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│   设计要点：                                                                │
│     • 不可变值对象，一旦创建不可修改                                       │
│     • 包含完整解读信息，可直接用于报告生成                                 │
│     • IsHighRisk() 方法判断是否需要预警                                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 六、运行时调用流程

### 6.1 单因子解读流程

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                        单因子解读时序图                                     │
│                                                                             │
│   EvaluationService        Interpretation         RangeStrategy            │
│         │                       │                      │                    │
│         │  1. 获取策略          │                      │                    │
│         │──────────────────────▶│                      │                    │
│         │   GetStrategy("range")│                      │                    │
│         │                       │                      │                    │
│         │  2. 返回策略实例      │                      │                    │
│         │◀──────────────────────│                      │                    │
│         │   &RangeStrategy{}    │                      │                    │
│         │                       │                      │                    │
│         │  3. 执行解读                                 │                    │
│         │──────────────────────────────────────────────▶                    │
│         │   Interpret(68, config)                      │                    │
│         │                                              │                    │
│         │  4. 遍历规则，匹配区间                       │                    │
│         │                       68 ∈ [63,73) → 中度    │                    │
│         │                                              │                    │
│         │  5. 返回解读结果                             │                    │
│         │◀──────────────────────────────────────────────                    │
│         │   {RiskLevel: "medium", Label: "中度抑郁"}   │                    │
│         │                                              │                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.2 完整评估中的调用

```go
// 伪代码：评估服务中调用解读策略
func (s *EvaluationService) interpretFactorScores(
    factorScores []FactorScore,
    scale *MedicalScale,
) []*InterpretResult {
    
    results := []*InterpretResult{}
    
    // 1. 遍历每个因子得分
    for _, fs := range factorScores {
        
        // 2. 获取因子的解读配置（来自量表配置）
        config := scale.GetInterpretConfig(fs.FactorCode)
        // config.Rules = [{Min:0,Max:53,Level:"none"}, ...]
        
        // 3. 获取解读策略
        strategy := interpretation.GetStrategy(config.StrategyType)
        // strategy = &RangeStrategy{}
        
        // 4. 执行解读
        result, _ := strategy.Interpret(fs.Score, config)
        // result = {RiskLevel:"medium", Label:"中度抑郁"}
        
        results = append(results, result)
    }
    
    // 5. 可选：执行组合解读（多因子联动）
    if scale.HasCompositeRule() {
        compositeStrategy := interpretation.GetCompositeStrategy("composite")
        compositeResult, _ := compositeStrategy.InterpretMultiple(factorScores, scale.CompositeConfig())
        // 合并组合解读结果
    }
    
    return results
}
```

---

## 七、风险等级体系

### 7.1 五级风险定义

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                          风险等级体系                                       │
│                                                                             │
│  ┌────────┬──────────┬────────────────────────────────────────────────────┐│
│  │  等级  │   标识   │                    业务含义                        ││
│  ├────────┼──────────┼────────────────────────────────────────────────────┤│
│  │  none  │   🟢     │ 正常范围，无需干预                                 ││
│  │  low   │   🟡     │ 轻度风险，建议关注                                 ││
│  │ medium │   🟠     │ 中度风险，建议咨询专业人员                         ││
│  │  high  │   🔴     │ 高度风险，建议及时就医                             ││
│  │ severe │   🔴🔴   │ 严重风险，紧急干预                                 ││
│  └────────┴──────────┴────────────────────────────────────────────────────┘│
│                                                                             │
│  高危判断：                                                                 │
│    IsHighRisk() = (RiskLevel == "high" || RiskLevel == "severe")           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 八、扩展指南

### 8.1 添加新解读策略

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                    添加新策略：百分位解读 (Percentile)                      │
│                                                                             │
│  Step 1: 定义策略类型                                                       │
│  ─────────────────────────────────────────────────────────────────────────  │
│    const StrategyTypePercentile StrategyType = "percentile"                │
│                                                                             │
│  Step 2: 实现策略接口                                                       │
│  ─────────────────────────────────────────────────────────────────────────  │
│    type PercentileStrategy struct{}                                        │
│                                                                             │
│    func (s *PercentileStrategy) Interpret(score float64, config ...) {     │
│        percentile := calculatePercentile(score, config.NormData)           │
│        // 根据百分位确定风险等级                                            │
│        if percentile > 95 { return HighRisk }                              │
│        if percentile > 75 { return MediumRisk }                            │
│        // ...                                                              │
│    }                                                                       │
│                                                                             │
│  Step 3: 注册策略                                                           │
│  ─────────────────────────────────────────────────────────────────────────  │
│    func init() {                                                           │
│        RegisterStrategy(&PercentileStrategy{})                             │
│    }                                                                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 九、设计思想总结

### 9.1 核心设计决策

| 设计决策 | 选择 | 理由 |
| --------- | ------ | ------ |
| **策略模式** | 3种策略实现 | 适应不同量表的解读需求 |
| **规则引擎** | 配置化规则 | 解读规则来自量表配置，非硬编码 |
| **左闭右开区间** | [min, max) | 区间连续无缝，边界判断清晰 |
| **值对象** | InterpretResult 不可变 | 线程安全，可直接传递 |
| **组合策略** | AND/OR 逻辑 | 支持多因子联动解读 |

### 9.2 与其他模块的协作

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                      Interpretation 协作关系图                              │
│                                                                             │
│                       ┌────────────────────┐                                │
│                       │   Interpretation   │                                │
│                       │   (解读策略引擎)   │                                │
│                       └─────────┬──────────┘                                │
│                                 │                                           │
│            ┌────────────────────┼────────────────────┐                      │
│            │                    │                    │                      │
│            ▼                    ▼                    ▼                      │
│   ┌────────────────┐   ┌────────────────┐   ┌────────────────┐             │
│   │  Calculation   │   │   Assessment   │   │     Report     │             │
│   │  (提供得分)    │   │  (存储结果)    │   │  (生成报告)    │             │
│   └────────────────┘   └────────────────┘   └────────────────┘             │
│                                                                             │
│   数据流向：                                                                │
│     Calculation ──▶ Interpretation ──▶ Assessment ──▶ Report               │
│      (因子分)         (解读结论)       (持久化)       (报告)                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 十、代码索引

| 文件 | 职责 | 路径 |
| ------ | ------ | ------ |
| `strategy.go` | 接口定义 + 注册表 | `internal/apiserver/domain/evaluation/interpretation/` |
| `types.go` | 类型定义 + 值对象 | 同上 |
| `threshold.go` | 阈值策略 + 区间策略 | 同上 |
| `composite.go` | 组合解读策略 | 同上 |
| `errors.go` | 错误定义 | 同上 |

---

## 📖 延伸阅读

- **前一篇**：[11-06-03 Calculation计算策略设计](./11-06-03-Calculation计算策略设计.md) - 理解分数如何计算
- **后一篇**：[11-06-05 Report聚合设计](./11-06-05-Report聚合设计.md) - 学习报告如何生成
- **相关概念**：规则引擎、策略模式、值对象
