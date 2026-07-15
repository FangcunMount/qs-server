package builder

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rule"
)

// DefaultReportBuilder 默认报告构建器实现。
type DefaultReportBuilder struct{}

// NewDefaultReportBuilder 创建默认报告构建器。
func NewDefaultReportBuilder() *DefaultReportBuilder {
	return &DefaultReportBuilder{}
}

func (b *DefaultReportBuilder) BuildDraft(input report.GenerateReportInput) (*report.Draft, error) {
	if input.AssessmentID.IsZero() {
		return nil, report.ErrInvalidArgument
	}

	conclusion := b.buildConclusion(input)
	dimensions := b.buildDimensions(input)
	suggestions := rule.CollectConfiguredSuggestions(input.Suggestion, input.FactorScores)

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
		dim = dim.WithScoreContext(fs.DerivedScores, fs.Level, fs.NormReference)
		if fs.Role != "" || fs.ParentCode != "" || fs.HierarchyLevel > 0 || fs.SortOrder > 0 {
			dim = dim.WithHierarchy(fs.Role, fs.ParentCode, fs.HierarchyLevel, fs.SortOrder)
		}
		dimensions = append(dimensions, dim)
	}
	return dimensions
}
