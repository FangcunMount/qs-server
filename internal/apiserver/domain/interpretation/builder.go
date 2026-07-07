package interpretation

// ReportBuilder 报告构建器接口。
type ReportBuilder interface {
	Build(input GenerateReportInput) (*InterpretReport, error)
}

// 默认ReportBuilder 默认报告构建器实现。
type DefaultReportBuilder struct {
	suggestionGenerator SuggestionGenerator
}

// New默认ReportBuilder 创建默认报告构建器。
func NewDefaultReportBuilder(suggestionGenerator SuggestionGenerator) *DefaultReportBuilder {
	return NewDefaultInterpretReportBuilder(suggestionGenerator)
}

// New默认InterpretReportBuilder 创建默认解读报告构建器。
func NewDefaultInterpretReportBuilder(suggestionGenerator SuggestionGenerator) *DefaultReportBuilder {
	return &DefaultReportBuilder{
		suggestionGenerator: suggestionGenerator,
	}
}

// NewScaleReportBuilder 创建默认报告构建器。
//
// Deprecated: 使用 NewDefaultInterpretReportBuilder。
func NewScaleReportBuilder(suggestionGenerator SuggestionGenerator) *DefaultReportBuilder {
	return NewDefaultInterpretReportBuilder(suggestionGenerator)
}
