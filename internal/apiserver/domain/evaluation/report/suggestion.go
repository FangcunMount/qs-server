package report

import "context"

// ==================== SuggestionGenerator 领域服务 ====================

// SuggestionGenerator 建议生成器接口
// 职责：根据解读报告生成个性化建议
// 实现方式：可基于规则引擎、AI 模型或专家知识库
type SuggestionGenerator interface {
	// Generate 生成建议
	// 参数：
	//   - ctx: 上下文
	//   - report: 解读报告
	// 返回：
	//   - []string: 建议列表
	//   - error: 生成失败时返回错误
	Generate(ctx context.Context, report *InterpretReport) ([]string, error)
}

// ==================== 建议生成策略 ====================

// SuggestionStrategy 建议生成策略接口
type SuggestionStrategy interface {
	// Name 策略名称
	Name() string

	// CanHandle 是否可以处理该报告
	CanHandle(report *InterpretReport) bool

	// GenerateSuggestions 生成建议
	GenerateSuggestions(ctx context.Context, report *InterpretReport) ([]string, error)
}

// ==================== 默认实现 ====================

// RuleBasedSuggestionGenerator 基于规则的建议生成器
type RuleBasedSuggestionGenerator struct {
	strategies []SuggestionStrategy
}

// NewRuleBasedSuggestionGenerator 创建规则建议生成器
func NewRuleBasedSuggestionGenerator(strategies ...SuggestionStrategy) *RuleBasedSuggestionGenerator {
	return &RuleBasedSuggestionGenerator{
		strategies: strategies,
	}
}

// Generate 生成建议
func (g *RuleBasedSuggestionGenerator) Generate(ctx context.Context, report *InterpretReport) ([]string, error) {
	var allSuggestions []string

	for _, strategy := range g.strategies {
		if strategy.CanHandle(report) {
			suggestions, err := strategy.GenerateSuggestions(ctx, report)
			if err != nil {
				// 单个策略失败不影响其他策略
				continue
			}
			allSuggestions = append(allSuggestions, suggestions...)
		}
	}

	// 去重
	return uniqueSuggestions(allSuggestions), nil
}

// uniqueSuggestions 去除重复建议
func uniqueSuggestions(suggestions []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range suggestions {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// ==================== 内置策略实现 ====================

// HighRiskSuggestionStrategy 高风险建议策略
type HighRiskSuggestionStrategy struct{}

// Name 策略名称
func (s *HighRiskSuggestionStrategy) Name() string {
	return "high_risk_strategy"
}

// CanHandle 是否可以处理
func (s *HighRiskSuggestionStrategy) CanHandle(report *InterpretReport) bool {
	return report.IsHighRisk()
}

// GenerateSuggestions 生成建议
func (s *HighRiskSuggestionStrategy) GenerateSuggestions(_ context.Context, report *InterpretReport) ([]string, error) {
	suggestions := []string{
		"建议尽快与专业心理咨询师进行一对一沟通",
		"建议学校心理健康中心进行跟进关注",
	}

	// 针对高风险维度添加具体建议
	highRiskDims := report.GetHighRiskDimensions()
	for _, dim := range highRiskDims {
		suggestion := generateDimensionSuggestion(dim)
		if suggestion != "" {
			suggestions = append(suggestions, suggestion)
		}
	}

	return suggestions, nil
}

// generateDimensionSuggestion 根据维度生成建议
func generateDimensionSuggestion(dim DimensionInterpret) string {
	// TODO: 根据因子编码从知识库获取建议
	// 这里先返回通用建议
	return "针对" + dim.FactorName() + "维度，建议进行专项辅导"
}

// DimensionSpecificSuggestionStrategy 维度特定建议策略
type DimensionSpecificSuggestionStrategy struct {
	// dimensionSuggestions 维度建议映射
	// key: 因子编码, value: 建议列表
	dimensionSuggestions map[string][]string
}

// NewDimensionSpecificSuggestionStrategy 创建维度特定建议策略
func NewDimensionSpecificSuggestionStrategy(suggestions map[string][]string) *DimensionSpecificSuggestionStrategy {
	return &DimensionSpecificSuggestionStrategy{
		dimensionSuggestions: suggestions,
	}
}

// Name 策略名称
func (s *DimensionSpecificSuggestionStrategy) Name() string {
	return "dimension_specific_strategy"
}

// CanHandle 是否可以处理
func (s *DimensionSpecificSuggestionStrategy) CanHandle(report *InterpretReport) bool {
	return report.HasDimensions()
}

// GenerateSuggestions 生成建议
func (s *DimensionSpecificSuggestionStrategy) GenerateSuggestions(_ context.Context, report *InterpretReport) ([]string, error) {
	var suggestions []string

	for _, dim := range report.Dimensions() {
		if dim.IsHighRisk() {
			if dimSuggestions, ok := s.dimensionSuggestions[string(dim.FactorCode())]; ok {
				suggestions = append(suggestions, dimSuggestions...)
			}
		}
	}

	return suggestions, nil
}

// GeneralWellbeingSuggestionStrategy 一般健康建议策略
type GeneralWellbeingSuggestionStrategy struct{}

// Name 策略名称
func (s *GeneralWellbeingSuggestionStrategy) Name() string {
	return "general_wellbeing_strategy"
}

// CanHandle 是否可以处理
func (s *GeneralWellbeingSuggestionStrategy) CanHandle(report *InterpretReport) bool {
	return !report.IsHighRisk()
}

// GenerateSuggestions 生成建议
func (s *GeneralWellbeingSuggestionStrategy) GenerateSuggestions(_ context.Context, _ *InterpretReport) ([]string, error) {
	return []string{
		"继续保持良好的心理状态",
		"建议定期参加学校组织的心理健康活动",
		"如有需要，可随时联系学校心理健康中心",
	}, nil
}
