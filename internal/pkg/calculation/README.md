# Calculation 计算功能域

## 概述

`calculation` 是一个**无状态的功能域**，专注于"计分"这个纯粹的计算逻辑。

## 设计原则

1. **无状态**：纯函数式计算，不依赖外部状态
2. **单一职责**：只负责计算，不涉及解读、存储等
3. **策略模式**：支持多种计分策略
4. **可复用**：被多个业务域调用
   - `survey/answersheet`：计算单个 answer 的得分
   - `assessment`：计算 factor 的汇总得分

## 目录结构

```text
calculation/
├── README.md           # 本文档
├── types.go            # 类型定义（策略类型、可计算值接口）
├── strategy.go         # 策略接口与注册表
├── calculator.go       # 计算器（组合策略执行计算）
│
├── sum.go              # 求和策略
├── average.go          # 平均分策略
├── weighted_sum.go     # 加权求和策略
├── max.go              # 最大值策略
├── min.go              # 最小值策略
└── reverse.go          # 反向计分策略
```

## 核心接口

### ScoringStrategy（计分策略）

```go
type ScoringStrategy interface {
    Calculate(values []float64, params map[string]string) (float64, error)
    StrategyType() StrategyType
}
```

### Calculator（计算器）

```go
type Calculator interface {
    // 计算单个值（如选项得分）
    CalculateOptionScore(optionCode string, scoreMapping map[string]float64) float64
    
    // 计算聚合值（如因子得分）
    CalculateAggregateScore(values []float64, strategy StrategyType, params map[string]string) (float64, error)
}
```

## 使用示例

```go
// 获取计算器
calc := calculation.NewCalculator()

// 计算选项得分
optionScore := calc.CalculateOptionScore("A", map[string]float64{"A": 1, "B": 2, "C": 3})

// 计算因子得分（求和）
factorScore, _ := calc.CalculateAggregateScore(
    []float64{1, 2, 3, 4},
    calculation.StrategyTypeSum,
    nil,
)

// 计算因子得分（加权求和）
weightedScore, _ := calc.CalculateAggregateScore(
    []float64{1, 2, 3},
    calculation.StrategyTypeWeightedSum,
    map[string]string{"weights": "[0.5, 0.3, 0.2]"},
)
```

## 与其他域的关系

```text
         calculation（无状态计算）
              ↑
    ┌─────────┴─────────┐
    │                   │
answersheet          assessment
（计算答案得分）      （计算因子得分）
```
