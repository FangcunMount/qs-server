# SuggestionGenerator 策略扩展指南

## 架构概览

```text
┌─────────────────────────────────────────────────────────────┐
│                   ReportBuilder                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  1. 使用 FactorInterpretationSuggestionStrategy      │  │
│  │     收集因子解读配置中的建议                           │  │
│  │  2. 调用 SuggestionGenerator 生成额外建议（可选）    │  │
│  │  3. 合并去重返回                                       │  │
│  └───────────────────────────────────────────────────────┘  │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
         ┌───────────────────────────────┐
         │   SuggestionGenerator 接口    │
         └───────────────┬───────────────┘
                         │
                         ▼
         ┌───────────────────────────────┐
         │ RuleBasedSuggestionGenerator  │
         │  (策略模式 - 责任链)           │
         └───────────────┬───────────────┘
                         │
         ┌───────────────┴───────────────┐
         │                               │
         ▼                               ▼
┌──────────────────┐          ┌──────────────────┐
│ Strategy 1       │          │ Strategy N       │
│ - CanHandle()    │   ...    │ - CanHandle()    │
│ - Generate()     │          │ - Generate()     │
└──────────────────┘          └──────────────────┘
```

## 内置策略

### FactorInterpretationSuggestionStrategy

- **触发条件**：有评估结果且包含因子得分
- **建议类型**：从因子解读规则配置中收集的建议
- **数据来源**：
  - 评估结果的总体建议 (`EvaluationResult.Suggestion`)
  - 所有因子的建议 (`FactorScoreResult.Suggestion`)，来自因子解读规则配置
- **特点**：
  - 完全依赖因子解读配置中的建议数据
  - 自动收集所有因子的建议（包括总分因子和其他因子）
  - 自动去重（总分因子建议如果与总体建议相同则不重复添加）

## 扩展方式

### 方式一：实现新策略

```go
// 1. 创建策略结构体
type CustomSuggestionStrategy struct {
    config Config
}

// 2. 实现 SuggestionStrategy 接口
func (s *CustomSuggestionStrategy) Name() string {
    return "custom_strategy"
}

func (s *CustomSuggestionStrategy) CanHandle(report *report.InterpretReport) bool {
    // 判断是否处理该报告
    return report.ScaleCode() == "YOUR_SCALE_CODE"
}

func (s *CustomSuggestionStrategy) GenerateSuggestions(ctx context.Context, rpt *report.InterpretReport) ([]string, error) {
    var suggestions []string
    
    // 实现建议生成逻辑
    if rpt.TotalScore() > 50 {
        suggestions = append(suggestions, "建议A")
    }
    
    return suggestions, nil
}

// 3. 在容器中注册
// 注意：FactorInterpretationSuggestionStrategy 需要在构建报告时动态创建
// 因为它需要访问 EvaluationResult
suggestionGenerator := report.NewRuleBasedSuggestionGenerator(
    &CustomSuggestionStrategy{config: cfg}, // 添加自定义策略
)
```

### 方式二：AI 策略集成

```go
// internal/apiserver/infra/ai/suggestion_strategy.go

package ai

type AISuggestionStrategy struct {
    client      AIClient
    fallback    report.SuggestionStrategy
    timeout     time.Duration
    maxRetries  int
}

func NewAISuggestionStrategy(client AIClient, fallback report.SuggestionStrategy) *AISuggestionStrategy {
    return &AISuggestionStrategy{
        client:     client,
        fallback:   fallback,
        timeout:    5 * time.Second,
        maxRetries: 2,
    }
}

func (s *AISuggestionStrategy) Name() string {
    return "ai_strategy"
}

func (s *AISuggestionStrategy) CanHandle(rpt *report.InterpretReport) bool {
    // AI 可以处理所有报告
    return true
}

func (s *AISuggestionStrategy) GenerateSuggestions(ctx context.Context, rpt *report.InterpretReport) ([]string, error) {
    // 构建提示词
    prompt := s.buildPrompt(rpt)
    
    // 设置超时
    ctx, cancel := context.WithTimeout(ctx, s.timeout)
    defer cancel()
    
    // 调用 AI 生成
    var suggestions []string
    var err error
    for i := 0; i < s.maxRetries; i++ {
        suggestions, err = s.client.GenerateSuggestions(ctx, prompt)
        if err == nil {
            return suggestions, nil
        }
        time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
    }
    
    // 降级到规则策略
    if s.fallback != nil && s.fallback.CanHandle(rpt) {
        return s.fallback.GenerateSuggestions(ctx, rpt)
    }
    
    return nil, err
}

func (s *AISuggestionStrategy) buildPrompt(rpt *report.InterpretReport) string {
    return fmt.Sprintf(`作为心理健康专家，根据以下测评结果生成个性化建议：

量表：%s
总分：%.1f
风险等级：%s
总体结论：%s

要求：
1. 生成3-5条具体可操作的建议
2. 建议应专业、温和、具有指导性
3. 针对不同风险等级提供差异化建议

建议列表：`,
        rpt.ScaleName(),
        rpt.TotalScore(),
        rpt.RiskLevel(),
        rpt.Conclusion(),
    )
}
```

### 方式三：数据库驱动策略

```go
type DatabaseSuggestionStrategy struct {
    repo SuggestionRepository
}

func (s *DatabaseSuggestionStrategy) GenerateSuggestions(ctx context.Context, rpt *report.InterpretReport) ([]string, error) {
    // 从数据库查询建议模板
    templates, err := s.repo.FindTemplatesByScaleAndRisk(
        ctx,
        rpt.ScaleCode(),
        string(rpt.RiskLevel()),
    )
    if err != nil {
        return nil, err
    }
    
    // 根据模板渲染建议
    var suggestions []string
    for _, tmpl := range templates {
        suggestion := s.renderTemplate(tmpl, rpt)
        suggestions = append(suggestions, suggestion)
    }
    
    return suggestions, nil
}
```

## 策略优先级

策略按注册顺序执行，所有 `CanHandle` 返回 true 的策略都会被调用：

```go
suggestionGenerator := report.NewRuleBasedSuggestionGenerator(
    strategy1, // 最先执行
    strategy2,
    strategy3, // 最后执行
)
```

**建议顺序**：

1. FactorInterpretationSuggestionStrategy（自动收集因子解读配置中的建议）
2. 自定义策略（按注册顺序执行）
3. AI 增强策略（可选）

## 配置示例

```yaml
# configs/apiserver.yaml

suggestion:
  # 启用的策略列表
  enabled_strategies:
    - factor_interpretation  # 自动启用，从因子解读配置收集建议
    - ai_enhancement  # 可选
  
  # AI 配置（如果启用）
  ai:
    enabled: true
    provider: openai
    api_key: sk-xxx
    model: gpt-4o-mini
    temperature: 0.7
    timeout: 5s
    max_retries: 2
```

## 测试示例

```go
func TestCustomStrategy(t *testing.T) {
    strategy := &CustomSuggestionStrategy{}
    
    report := &report.InterpretReport{
        // 构造测试报告
    }
    
    // 测试 CanHandle
    assert.True(t, strategy.CanHandle(report))
    
    // 测试生成
    suggestions, err := strategy.GenerateSuggestions(context.Background(), report)
    assert.NoError(t, err)
    assert.NotEmpty(t, suggestions)
}
```

## 最佳实践

1. **单一职责**：每个策略只处理特定场景
2. **降级机制**：AI 策略应有规则策略作为 fallback
3. **性能考虑**：CanHandle 应该快速返回，避免复杂计算
4. **错误处理**：单个策略失败不应影响其他策略
5. **可配置性**：策略应支持外部配置，避免硬编码
6. **可测试性**：策略应易于单元测试

## 未来扩展方向

- [ ] 多语言支持策略
- [ ] 基于用户画像的个性化策略
- [ ] 历史数据分析策略
- [ ] 群体对比策略
- [ ] 时序变化趋势策略
