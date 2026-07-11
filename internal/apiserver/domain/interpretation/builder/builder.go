package builder

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rule"
)

// DefaultReportBuilder 默认报告构建器实现。
type DefaultReportBuilder struct {
	suggestionGenerator rule.SuggestionGenerator
}

// NewDefaultReportBuilder 创建默认报告构建器。
func NewDefaultReportBuilder(suggestionGenerator rule.SuggestionGenerator) *DefaultReportBuilder {
	return &DefaultReportBuilder{
		suggestionGenerator: suggestionGenerator,
	}
}

// GenerateReportInput 生成报告的输入参数。
type GenerateReportInput = report.GenerateReportInput

func (b *DefaultReportBuilder) BuildDraft(input report.GenerateReportInput) (*report.Draft, error) {
	if input.AssessmentID.IsZero() {
		return nil, report.ErrInvalidArgument
	}

	conclusion := b.buildConclusion(input)
	dimensions := b.buildDimensions(input)
	suggestions := b.buildSuggestions(context.Background(), input, dimensions)

	return report.NewDraft(report.Content{
		Model:        report.ModelIdentity{Title: input.ModelName, Code: input.ModelCode},
		PrimaryScore: report.NewRawTotalScore(input.TotalScore, nil),
		Level:        report.LevelFromRisk(input.RiskLevel),
		Conclusion:   conclusion,
		Dimensions:   dimensions,
		Suggestions:  suggestions,
	}), nil
}

func (b *DefaultReportBuilder) buildConclusion(input report.GenerateReportInput) string {
	for _, fs := range input.FactorScores {
		if fs.IsTotalScore && fs.Description != "" {
			return fs.Description
		}
	}
	if input.Conclusion != "" {
		return input.Conclusion
	}
	return ""
}

func (b *DefaultReportBuilder) buildDimensions(input report.GenerateReportInput) []report.DimensionInterpret {
	if len(input.FactorScores) == 0 {
		return nil
	}

	dimensions := make([]report.DimensionInterpret, 0, len(input.FactorScores))
	for _, fs := range input.FactorScores {
		dim := report.NewDimensionInterpret(
			fs.FactorCode,
			fs.FactorName,
			fs.RawScore,
			fs.MaxScore,
			fs.RiskLevel,
			fs.Description,
			fs.Suggestion,
		)
		if fs.Role != "" || fs.ParentCode != "" || fs.HierarchyLevel > 0 || fs.SortOrder > 0 {
			dim = dim.WithHierarchy(fs.Role, fs.ParentCode, fs.HierarchyLevel, fs.SortOrder)
		}
		dimensions = append(dimensions, dim)
	}
	return dimensions
}

func (b *DefaultReportBuilder) buildSuggestions(
	ctx context.Context,
	input report.GenerateReportInput,
	dimensions []report.DimensionInterpret,
) []report.Suggestion {
	var allSuggestions []report.Suggestion

	factorStrategy := rule.NewFactorInterpretationSuggestionStrategy(input.Suggestion, input.FactorScores)
	if factorStrategy.CanHandle(report.Content{}) {
		factorSuggestions, err := factorStrategy.GenerateSuggestions(ctx, report.Content{})
		if err == nil {
			allSuggestions = append(allSuggestions, factorSuggestions...)
		}
	}

	if b.suggestionGenerator != nil {
		content := report.Content{
			Model:        report.ModelIdentity{Title: input.ModelName, Code: input.ModelCode},
			PrimaryScore: report.NewRawTotalScore(input.TotalScore, nil),
			Level:        report.LevelFromRisk(input.RiskLevel),
			Conclusion:   b.buildConclusion(input),
			Dimensions:   dimensions,
		}
		generatedSuggestions, err := b.suggestionGenerator.Generate(ctx, content)
		if err == nil {
			allSuggestions = append(allSuggestions, generatedSuggestions...)
		}
	}

	return rule.UniqueSuggestions(allSuggestions)
}
