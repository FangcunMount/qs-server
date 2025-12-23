package report

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// SuggestionCategory 建议分类
type SuggestionCategory string

const (
	// SuggestionCategoryGeneral 总体/默认建议
	SuggestionCategoryGeneral SuggestionCategory = "general"
	// SuggestionCategoryFamily 家庭维度
	SuggestionCategoryFamily SuggestionCategory = "family"
	// SuggestionCategoryStudy 学习/学校维度
	SuggestionCategoryStudy SuggestionCategory = "study"
	// SuggestionCategorySocial 社交维度
	SuggestionCategorySocial SuggestionCategory = "social"
	// SuggestionCategoryHealth 健康维度
	SuggestionCategoryHealth SuggestionCategory = "health"
	// SuggestionCategoryDimension 按因子维度（默认）
	SuggestionCategoryDimension SuggestionCategory = "dimension"
)

// Suggestion 结构化建议
type Suggestion struct {
	Category   SuggestionCategory
	Content    string
	FactorCode *FactorCode // 可选：关联具体因子
}

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
	//   - []Suggestion: 建议列表
	//   - error: 生成失败时返回错误
	Generate(ctx context.Context, report *InterpretReport) ([]Suggestion, error)
}

// ==================== 建议生成策略 ====================

// SuggestionStrategy 建议生成策略接口
type SuggestionStrategy interface {
	// Name 策略名称
	Name() string

	// CanHandle 是否可以处理该报告
	CanHandle(report *InterpretReport) bool

	// GenerateSuggestions 生成建议
	GenerateSuggestions(ctx context.Context, report *InterpretReport) ([]Suggestion, error)
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
func (g *RuleBasedSuggestionGenerator) Generate(ctx context.Context, report *InterpretReport) ([]Suggestion, error) {
	var allSuggestions []Suggestion

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
func uniqueSuggestions(suggestions []Suggestion) []Suggestion {
	type key struct {
		category SuggestionCategory
		content  string
		factor   string
	}
	seen := make(map[key]bool)
	var result []Suggestion
	for _, s := range suggestions {
		k := key{
			category: s.Category,
			content:  s.Content,
		}
		if s.FactorCode != nil {
			k.factor = s.FactorCode.String()
		}
		if s.Content == "" {
			continue
		}
		if !seen[k] {
			seen[k] = true
			result = append(result, s)
		}
	}
	return result
}

// ==================== 内置策略实现 ====================

// FactorInterpretationSuggestionStrategy 基于因子解读配置的建议策略
// 从因子解读规则配置中收集 suggestion 数据
type FactorInterpretationSuggestionStrategy struct {
	// evaluationResult 评估结果，包含所有因子的建议
	evaluationResult *assessment.EvaluationResult
}

// NewFactorInterpretationSuggestionStrategy 创建基于因子解读配置的建议策略
func NewFactorInterpretationSuggestionStrategy(evaluationResult *assessment.EvaluationResult) *FactorInterpretationSuggestionStrategy {
	return &FactorInterpretationSuggestionStrategy{
		evaluationResult: evaluationResult,
	}
}

// Name 策略名称
func (s *FactorInterpretationSuggestionStrategy) Name() string {
	return "factor_interpretation_strategy"
}

// CanHandle 是否可以处理
func (s *FactorInterpretationSuggestionStrategy) CanHandle(report *InterpretReport) bool {
	// 只要有评估结果就可以处理
	return s.evaluationResult != nil && len(s.evaluationResult.FactorScores) > 0
}

// GenerateSuggestions 生成建议
// 从因子解读规则配置中收集 suggestion 数据
func (s *FactorInterpretationSuggestionStrategy) GenerateSuggestions(_ context.Context, _ *InterpretReport) ([]Suggestion, error) {
	if s.evaluationResult == nil {
		return []Suggestion{}, nil
	}

	var suggestions []Suggestion

	// 收集总体建议
	if s.evaluationResult.Suggestion != "" {
		suggestions = append(suggestions, Suggestion{
			Category: SuggestionCategoryGeneral,
			Content:  s.evaluationResult.Suggestion,
		})
	}

	// 收集所有因子的建议（来自因子解读规则配置）
	// 优先收集总分因子的建议，然后收集其他因子的建议
	for _, fs := range s.evaluationResult.FactorScores {
		if fs.Suggestion == "" {
			continue
		}
		// 如果是总分因子，且与总体建议不同，则添加
		if fs.IsTotalScore {
			if fs.Suggestion != s.evaluationResult.Suggestion {
				suggestions = append(suggestions, Suggestion{
					Category: SuggestionCategoryGeneral,
					Content:  fs.Suggestion,
				})
			}
		} else {
			// 非总分因子的建议也收集
			factorCode := fs.FactorCode
			suggestions = append(suggestions, Suggestion{
				Category:   SuggestionCategoryDimension,
				Content:    fs.Suggestion,
				FactorCode: &factorCode,
			})
		}
	}

	return suggestions, nil
}
