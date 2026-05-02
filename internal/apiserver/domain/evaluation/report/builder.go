package report

import (
	"context"
)

// ==================== ReportBuilder 领域服务 ====================

// ReportBuilder 报告构建器接口
// 职责：根据评估结果构建解读报告
// 实现方式：调用 scale 域的解读服务获取解读信息
type ReportBuilder interface {
	// Build 构建解读报告
	Build(input GenerateReportInput) (*InterpretReport, error)
}

// ==================== 默认实现 ====================

// DefaultReportBuilder 默认报告构建器实现
type DefaultReportBuilder struct {
	// suggestionGenerator 建议生成器（可选）
	// 如果不提供，则只使用评估结果中的建议
	suggestionGenerator SuggestionGenerator
}

// NewDefaultReportBuilder 创建默认报告构建器
func NewDefaultReportBuilder(suggestionGenerator SuggestionGenerator) *DefaultReportBuilder {
	return &DefaultReportBuilder{
		suggestionGenerator: suggestionGenerator,
	}
}

// Build 构建解读报告
func (b *DefaultReportBuilder) Build(input GenerateReportInput) (*InterpretReport, error) {
	if input.AssessmentID.IsZero() {
		return nil, ErrInvalidArgument
	}

	conclusion := b.buildConclusion(input)

	dimensions := b.buildDimensions(input)

	// 优先使用评估结果中的建议，然后使用 SuggestionGenerator 增强
	suggestions := b.buildSuggestions(context.Background(), input, dimensions)

	report := NewInterpretReport(
		input.AssessmentID,
		input.ScaleName,
		input.ScaleCode,
		input.TotalScore,
		input.RiskLevel,
		conclusion,
		dimensions,
		suggestions,
	)

	return report, nil
}

// buildConclusion 构建总体结论
func (b *DefaultReportBuilder) buildConclusion(input GenerateReportInput) string {
	// 优先使用总分因子的解读作为总体结论
	for _, fs := range input.FactorScores {
		if fs.IsTotalScore && fs.Description != "" {
			return fs.Description
		}
	}

	// 如果没有总分因子解读，使用评估结果的总体结论
	if input.Conclusion != "" {
		return input.Conclusion
	}

	// 如果都没有，返回空字符串
	return ""
}

// buildDimensions 构建维度解读
func (b *DefaultReportBuilder) buildDimensions(input GenerateReportInput) []DimensionInterpret {
	if len(input.FactorScores) == 0 {
		return nil
	}

	dimensions := make([]DimensionInterpret, 0, len(input.FactorScores))
	for _, fs := range input.FactorScores {
		dim := NewDimensionInterpret(
			fs.FactorCode,
			fs.FactorName,
			fs.RawScore,
			fs.MaxScore,
			fs.RiskLevel,
			fs.Description,
			fs.Suggestion,
		)
		dimensions = append(dimensions, dim)
	}

	return dimensions
}

// buildSuggestions 构建建议
// 策略：
// 1. 首先使用 FactorInterpretationSuggestionStrategy 收集因子解读配置中的建议
// 2. 如果配置了 SuggestionGenerator，使用策略生成器生成额外建议
// 3. 合并去重
func (b *DefaultReportBuilder) buildSuggestions(
	ctx context.Context,
	input GenerateReportInput,
	dimensions []DimensionInterpret,
) []Suggestion {
	var allSuggestions []Suggestion

	// 1. 首先收集因子解读配置中的建议（通过 FactorInterpretationSuggestionStrategy）
	factorStrategy := NewFactorInterpretationSuggestionStrategy(input)
	if factorStrategy.CanHandle(nil) {
		factorSuggestions, err := factorStrategy.GenerateSuggestions(ctx, nil)
		if err == nil {
			allSuggestions = append(allSuggestions, factorSuggestions...)
		}
	}

	// 2. 如果配置了 SuggestionGenerator，使用策略生成器生成额外建议
	if b.suggestionGenerator != nil {
		// 构建临时报告用于生成建议
		tempReport := NewInterpretReport(
			input.AssessmentID,
			input.ScaleName,
			input.ScaleCode,
			input.TotalScore,
			input.RiskLevel,
			b.buildConclusion(input),
			dimensions,
			nil, // 建议稍后填充
		)

		generatedSuggestions, err := b.suggestionGenerator.Generate(ctx, tempReport)
		if err == nil {
			allSuggestions = append(allSuggestions, generatedSuggestions...)
		}
		// 失败不影响报告生成，只记录错误
	}

	// 3. 去重
	return uniqueSuggestions(allSuggestions)
}
