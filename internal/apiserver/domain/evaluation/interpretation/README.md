# Interpretation 解读领域服务

## 概述

interpretation 包是测评解读领域服务，提供**策略模式 + 领域服务**的完整解读能力。

**设计原则**：

- **无状态**：所有服务和策略都是无状态的，纯函数式计算
- **可扩展**：支持多种解读策略（区间、阈值、组合）
- **易用性**：提供统一的领域服务接口，隐藏策略选择细节
- **高性能**：支持批量解读和并发处理

**架构特点**：

- 与 `domain/calculation` 和 `domain/validation` 保持一致的设计模式
- 策略模式：多种解读算法可插拔
- 服务模式：提供统一的解读器接口
- 批量处理：支持串行和并发批量解读

---

## 核心组件

### 1. 策略模式 (Strategy Pattern)

**策略接口**：

```go
// InterpretStrategy 解读策略接口
type InterpretStrategy interface {
    Interpret(score float64, config *InterpretConfig) (*InterpretResult, error)
    StrategyType() StrategyType
}
```

**内置策略**：

- `RangeStrategy` - 区间解读：根据分数所在区间确定风险等级
- `ThresholdStrategy` - 阈值解读：超过阈值即为指定风险等级
- `CompositeStrategyImpl` - 组合解读：多因子组合判断

---

### 2. 领域服务 (Domain Service)

**Interpreter 接口**：

```go
type Interpreter interface {
    // 解读单个因子
    InterpretFactor(score float64, config *InterpretConfig, strategyType StrategyType) (*InterpretResult, error)
    
    // 使用单条规则快速解读
    InterpretFactorWithRule(score float64, rule InterpretRule) *InterpretResult
    
    // 多因子组合解读
    InterpretMultipleFactors(scores []FactorScore, config *CompositeConfig, strategyType StrategyType) (*CompositeResult, error)
}
```

---

### 3. 默认解读提供者 (Default Interpretation Provider)

**DefaultInterpretationProvider**：当没有配置解读规则时，提供通用的默认解读

```go
type DefaultInterpretationProvider struct{}

// 为因子提供默认解读
func (p *DefaultInterpretationProvider) ProvideFactor(
    factorName string, 
    score float64, 
    riskLevel RiskLevel,
) *InterpretResult

// 提供整体默认解读
func (p *DefaultInterpretationProvider) ProvideOverall(
    totalScore float64, 
    riskLevel RiskLevel,
) *InterpretResult

// 使用自定义模版提供解读
func (p *DefaultInterpretationProvider) ProvideFactorWithTemplate(
    template string,
    factorName string,
    score float64,
    riskLevel RiskLevel,
    suggestion string,
) *InterpretResult
```

**设计原则**：
- 符合医学常规的通用解读文本
- 根据风险等级自动生成描述和建议
- 支持自定义模版扩展

---

### 4. 批量处理 (Batch Processing)

```go
type BatchInterpreter struct {
    interpreter *DefaultInterpreter
}

// 串行批量解读
results := batchInterpreter.InterpretAll(tasks)

// 并发批量解读
results := batchInterpreter.InterpretAllConcurrent(tasks, workerCount)
```

---

## 使用示例

### 示例 1：单个因子解读（推荐）

```go
// 使用便捷函数
result, err := interpretation.InterpretFactor(
    12.0,                                    // 得分
    config,                                  // 配置
    interpretation.StrategyTypeRange,        // 策略类型
)
```

### 示例 2：使用默认解读提供者

```go
// 当没有配置解读规则时，使用默认解读
provider := interpretation.GetDefaultProvider()

// 为因子提供默认解读
result := provider.ProvideFactor(
    "注意力不集中",                    // 因子名称
    15.0,                            // 得分
    interpretation.RiskLevelHigh,    // 风险等级
)
fmt.Printf("描述: %s\n", result.Description)  // "注意力不集中得分15.0分，处于较高风险水平"
fmt.Printf("建议: %s\n", result.Suggestion)   // "建议尽快咨询专业人员，了解更多信息"

// 提供整体默认解读
overallResult := provider.ProvideOverall(
    42.0,                            // 总分
    interpretation.RiskLevelHigh,    // 风险等级
)

// 或使用便捷函数
result := interpretation.ProvideDefaultFactor("注意力不集中", 15.0, interpretation.RiskLevelHigh)
overallResult := interpretation.ProvideDefaultOverall(42.0, interpretation.RiskLevelHigh)
```

### 示例 3：使用自定义模版

```go
provider := interpretation.GetDefaultProvider()

// 使用自定义模版（%s = 因子名，%.1f = 得分）
result := provider.ProvideFactorWithTemplate(
    "%s维度评分为%.1f，需要重点关注",  // 自定义模版
    "情绪稳定性",                     // 因子名称
    8.5,                             // 得分
    interpretation.RiskLevelMedium,  // 风险等级
    "建议进行心理咨询",               // 自定义建议
)
```

### 示例 4：批量解读

```go
// 准备批量任务
tasks := []interpretation.InterpretTask{
    {ID: "factor1", Score: 12.0, Config: config1, StrategyType: interpretation.StrategyTypeRange},
    {ID: "factor2", Score: 8.0, Config: config2, StrategyType: interpretation.StrategyTypeRange},
}

// 并发解读（自动选择工作协程数）
results := interpretation.InterpretAllConcurrent(tasks, 0)
```

---

## 最佳实践

### 1. 使用领域服务，而非直接使用策略

❌ **不推荐**（直接使用策略）：

```go
strategy := interpretation.GetStrategy(interpretation.StrategyTypeRange)
result, err := strategy.Interpret(score, config)
```

✅ **推荐**（使用领域服务）：

```go
result, err := interpretation.InterpretFactor(score, config, interpretation.StrategyTypeRange)
```

### 2. 优先使用规则解读，降级使用默认解读

✅ **推荐的解读流程**：

```go
// 1. 尝试使用配置的解读规则
if config != nil && len(config.Rules) > 0 {
    result, err := interpretation.InterpretFactor(score, config, interpretation.StrategyTypeRange)
    if err == nil {
        return result
    }
}

// 2. 降级使用默认解读
result := interpretation.ProvideDefaultFactor(factorName, score, riskLevel)
return result
```

### 3. 使用默认提供者而非硬编码文本

❌ **不推荐**（硬编码）：

```go
func getDefaultInterpretation(score float64, riskLevel RiskLevel) string {
    switch riskLevel {
    case RiskLevelHigh:
        return fmt.Sprintf("得分%.1f分，风险较高", score)
    // ...
    }
}
```

✅ **推荐**（使用领域服务）：

```go
result := interpretation.ProvideDefaultFactor(factorName, score, riskLevel)
return result.Description
```

### 4. 优先使用便捷函数

```go
// 推荐：使用包级便捷函数
result, err := interpretation.InterpretFactor(score, config, strategyType)

// 而不是
interpreter := interpretation.GetDefaultInterpreter()
result, err := interpreter.InterpretFactor(score, config, strategyType)
```

---

## 与其他领域服务的对比

| 特性 | calculation | validation | interpretation |
|------|------------|-----------|---------------|
| **策略模式** | ✅ Sum/Avg/Count | ✅ Required/Range/Pattern | ✅ Range/Threshold/Composite |
| **领域服务** | ✅ Scorer | ✅ Validator | ✅ Interpreter |
| **默认提供者** | ❌ | ❌ | ✅ DefaultProvider |
| **批量处理** | ✅ BatchScorer | ✅ BatchValidator | ✅ BatchInterpreter |
| **便捷函数** | ✅ Score() | ✅ ValidateValue() | ✅ InterpretFactor() |
| **并发支持** | ✅ | ❌ | ✅ |

---

## 文件结构

```text
interpretation/
├── types.go              # 核心类型定义
├── strategy.go           # 策略接口和注册表
├── threshold.go          # 阈值和区间策略实现
├── composite.go          # 组合策略实现
├── interpreter.go        # 解读器领域服务 ⭐
├── default_provider.go   # 默认解读提供者 ⭐
├── batch.go              # 批量解读服务 ⭐
├── errors.go             # 错误定义
└── README.md             # 本文档
```

⭐ 标记的文件是新增的领域服务层
