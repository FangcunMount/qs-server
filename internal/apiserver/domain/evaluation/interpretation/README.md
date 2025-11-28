# Interpretation 解读功能域

## 概述

Interpretation（解读）是一个**无状态的功能性领域**，专门负责根据计算得分生成解读结论。

## 设计原则

### 单一职责

- 仅负责**解读逻辑**：根据得分判断风险等级、生成解读文本
- 不负责计分（由 calculation 域处理）
- 不负责报告持久化（由 assessment 域处理）

### 无状态

- 所有解读函数都是**纯函数**
- 输入：得分 + 解读规则配置
- 输出：解读结果
- 无副作用，便于测试和并发

### 策略模式

- 支持多种解读策略：阈值解读、区间解读、组合解读等
- 通过策略注册表管理
- 便于扩展新的解读方式

## 核心类型

### InterpretStrategy 解读策略接口

```go
type InterpretStrategy interface {
    Interpret(score float64, config *InterpretConfig) (*InterpretResult, error)
    StrategyType() StrategyType
}
```

### InterpretConfig 解读配置

```go
type InterpretConfig struct {
    Rules      []InterpretRule  // 解读规则列表
    FactorCode string           // 因子编码
    Params     map[string]string // 额外参数
}
```

### InterpretResult 解读结果

```go
type InterpretResult struct {
    RiskLevel   RiskLevel  // 风险等级
    Label       string     // 简短标签
    Description string     // 详细描述
    Suggestion  string     // 建议
}
```

## 支持的解读策略

| 策略类型 | 说明 | 使用场景 |
|---------|------|---------|
| threshold | 阈值解读 | 得分超过某个阈值则为高风险 |
| range | 区间解读 | 根据得分所在区间确定等级 |
| composite | 组合解读 | 多个因子组合判断风险等级 |

## 使用示例

```go
// 1. 获取解读策略
strategy := GetStrategy(StrategyTypeRange)

// 2. 配置解读规则
config := &InterpretConfig{
    FactorCode: "depression",
    Rules: []InterpretRule{
        {Min: 0, Max: 10, RiskLevel: RiskLevelNone, Label: "正常"},
        {Min: 11, Max: 20, RiskLevel: RiskLevelMild, Label: "轻度"},
        {Min: 21, Max: 30, RiskLevel: RiskLevelModerate, Label: "中度"},
        {Min: 31, Max: 100, RiskLevel: RiskLevelSevere, Label: "重度"},
    },
}

// 3. 执行解读
result, err := strategy.Interpret(25.0, config)
// result.RiskLevel = RiskLevelModerate
// result.Label = "中度"
```

## 与其他域的关系

```text
[Scale 子域] → 提供解读规则配置
     ↓
[Calculation 域] → 提供计算得分
     ↓
[Interpretation 域] → 生成解读结果
     ↓
[Assessment 域] → 持久化报告
```
