package interpretation

// ReportBuilder 报告构建器接口。
type ReportBuilder interface {
	Build(input GenerateReportInput) (*InterpretReport, error)
}

// DefaultReportBuilder 默认报告构建器实现。
type DefaultReportBuilder struct {
	suggestionGenerator SuggestionGenerator
}

// NewDefaultReportBuilder 创建默认报告构建器。
func NewDefaultReportBuilder(suggestionGenerator SuggestionGenerator) *DefaultReportBuilder {
	return NewDefaultInterpretReportBuilder(suggestionGenerator)
}

// NewDefaultInterpretReportBuilder 创建默认解读报告构建器。
func NewDefaultInterpretReportBuilder(suggestionGenerator SuggestionGenerator) *DefaultReportBuilder {
	return &DefaultReportBuilder{
		suggestionGenerator: suggestionGenerator,
	}
}

// NewScaleReportBuilder 创建默认报告构建器。
//
// Deprecated: use NewDefaultInterpretReportBuilder.
func NewScaleReportBuilder(suggestionGenerator SuggestionGenerator) *DefaultReportBuilder {
	return NewDefaultInterpretReportBuilder(suggestionGenerator)
}
