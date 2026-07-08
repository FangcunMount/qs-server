package builder

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rule"
	reportscore "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/scoring"
	typology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ReportBuilder 报告构建器接口。
type ReportBuilder = report.ReportBuilder

// DefaultReportBuilder 默认报告构建器实现。
type DefaultReportBuilder struct {
	suggestionGenerator rule.SuggestionGenerator
}

// NewDefaultReportBuilder 创建默认报告构建器。
func NewDefaultReportBuilder(suggestionGenerator rule.SuggestionGenerator) *DefaultReportBuilder {
	return NewDefaultInterpretReportBuilder(suggestionGenerator)
}

// NewDefaultInterpretReportBuilder 创建默认解读报告构建器。
func NewDefaultInterpretReportBuilder(suggestionGenerator rule.SuggestionGenerator) *DefaultReportBuilder {
	return &DefaultReportBuilder{
		suggestionGenerator: suggestionGenerator,
	}
}

// NewScaleReportBuilder 创建默认报告构建器。
//
// Deprecated: 使用 NewDefaultInterpretReportBuilder。
func NewScaleReportBuilder(suggestionGenerator rule.SuggestionGenerator) *DefaultReportBuilder {
	return NewDefaultInterpretReportBuilder(suggestionGenerator)
}

// GenerateReportInput 生成报告的输入参数。
type GenerateReportInput = report.GenerateReportInput

// MechanismFamily 标识 which 报告构建器机制 to use。
type MechanismFamily = modelcatalog.AlgorithmFamily

const (
	MechanismFactorScoring        = modelcatalog.AlgorithmFamilyFactorScoring
	MechanismFactorClassification = modelcatalog.AlgorithmFamilyFactorClassification
	MechanismFactorNorm           = modelcatalog.AlgorithmFamilyFactorNorm
	MechanismTaskPerformance      = modelcatalog.AlgorithmFamilyTaskPerformance
)

// FactorScoringBuilder 构建 score-range reports。
func FactorScoringBuilder(composer ReportBuilder, input reportscore.ScaleReportInput) (*report.InterpretReport, error) {
	return reportscore.BuildScaleReport(composer, input)
}

// TypologyBuilder 构建因子-分类 reports via 机制 templates。
var (
	BuildPersonalityTypeReport = typology.BuildPersonalityTypeReport
	BuildTraitProfileReport    = typology.BuildTraitProfileReport
)

// Registry 解析机制 builders 按算法家族。
type Registry struct {
	byFamily map[MechanismFamily]ReportBuilder
}

// NewRegistry 创建空机制 builder 注册表。
func NewRegistry() *Registry {
	return &Registry{byFamily: make(map[MechanismFamily]ReportBuilder)}
}

// Register 添加 builder 用于机制家族。
func (r *Registry) Register(family MechanismFamily, builder ReportBuilder) {
	if r == nil {
		return
	}
	r.byFamily[family] = builder
}

// Resolve 返回 builder 用于机制家族。
func (r *Registry) Resolve(family MechanismFamily) (ReportBuilder, bool) {
	if r == nil {
		return nil, false
	}
	builder, ok := r.byFamily[family]
	return builder, ok
}

type interpretReportDraft struct {
	assessmentID report.ID
	model        report.ModelIdentity
	primaryScore *report.ScoreValue
	level        *report.ResultLevel
	modelName    string
	modelCode    string
	totalScore   float64
	riskLevel    report.RiskLevel
	conclusion   string
	dimensions   []report.DimensionInterpret
	suggestions  []report.Suggestion
	modelExtra   *report.ModelExtra
}

func (d interpretReportDraft) build() *report.InterpretReport {
	r := report.NewInterpretReport(
		d.assessmentID,
		d.modelName,
		d.modelCode,
		d.totalScore,
		d.riskLevel,
		d.conclusion,
		d.dimensions,
		d.suggestions,
		d.modelExtra,
	)
	return report.AttachOutcomeSummary(r, d.model, d.primaryScore, d.level)
}

func (b *DefaultReportBuilder) Build(input report.GenerateReportInput) (*report.InterpretReport, error) {
	if input.AssessmentID.IsZero() {
		return nil, report.ErrInvalidArgument
	}

	conclusion := b.buildConclusion(input)
	dimensions := b.buildDimensions(input)
	suggestions := b.buildSuggestions(context.Background(), input, dimensions)

	return interpretReportDraft{
		assessmentID: input.AssessmentID,
		modelName:    input.ModelName,
		modelCode:    input.ModelCode,
		totalScore:   input.TotalScore,
		riskLevel:    input.RiskLevel,
		conclusion:   conclusion,
		dimensions:   dimensions,
		suggestions:  suggestions,
	}.build(), nil
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
	if factorStrategy.CanHandle(nil) {
		factorSuggestions, err := factorStrategy.GenerateSuggestions(ctx, nil)
		if err == nil {
			allSuggestions = append(allSuggestions, factorSuggestions...)
		}
	}

	if b.suggestionGenerator != nil {
		tempReport := interpretReportDraft{
			assessmentID: input.AssessmentID,
			modelName:    input.ModelName,
			modelCode:    input.ModelCode,
			totalScore:   input.TotalScore,
			riskLevel:    input.RiskLevel,
			conclusion:   b.buildConclusion(input),
			dimensions:   dimensions,
		}.build()

		generatedSuggestions, err := b.suggestionGenerator.Generate(ctx, tempReport)
		if err == nil {
			allSuggestions = append(allSuggestions, generatedSuggestions...)
		}
	}

	return rule.UniqueSuggestions(allSuggestions)
}
