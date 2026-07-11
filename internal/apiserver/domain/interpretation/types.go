package interpretation

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/builder"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rule"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type (
	ID                 = report.ID
	AssessmentID       = report.AssessmentID
	RiskLevel          = report.RiskLevel
	FactorCode         = report.FactorCode
	DimensionCode      = report.DimensionCode
	DimensionKind      = report.DimensionKind
	FactorScoreInput   = report.FactorScoreInput
	InterpretReport    = report.InterpretReport
	DimensionInterpret = report.DimensionInterpret
	ScoreValue         = report.ScoreValue
	ResultLevel        = report.ResultLevel
	ModelIdentity      = report.ModelIdentity
	ModelExtra         = report.ModelExtra
	ModelRarity        = report.ModelRarity
	SuggestionCategory = report.SuggestionCategory
	Suggestion         = report.Suggestion
	ReportStatus       = report.Status

	ReportBuilder                          = report.ReportBuilder
	DefaultReportBuilder                   = builder.DefaultReportBuilder
	GenerateReportInput                    = report.GenerateReportInput
	SuggestionStrategy                     = rule.SuggestionStrategy
	SuggestionGenerator                    = rule.SuggestionGenerator
	SuggestionInput                        = rule.SuggestionInput
	RuleBasedSuggestionGenerator           = rule.RuleBasedSuggestionGenerator
	FactorInterpretationSuggestionStrategy = rule.FactorInterpretationSuggestionStrategy
)

const (
	ReportStatusPending    = report.StatusPending
	ReportStatusGenerating = report.StatusGenerating
	ReportStatusGenerated  = report.StatusGenerated
	ReportStatusFailed     = report.StatusFailed

	RiskLevelNone   = report.RiskLevelNone
	RiskLevelLow    = report.RiskLevelLow
	RiskLevelMedium = report.RiskLevelMedium
	RiskLevelHigh   = report.RiskLevelHigh
	RiskLevelSevere = report.RiskLevelSevere

	DimensionKindFactor  = report.DimensionKindFactor
	DimensionKindPole    = report.DimensionKindPole
	DimensionKindTrait   = report.DimensionKindTrait
	DimensionKindIndex   = report.DimensionKindIndex
	DimensionKindAbility = report.DimensionKindAbility

	ScoreKindRawTotal     = report.ScoreKindRawTotal
	ScoreKindMatchPercent = report.ScoreKindMatchPercent

	SuggestionCategoryGeneral   = report.SuggestionCategoryGeneral
	SuggestionCategoryFamily    = report.SuggestionCategoryFamily
	SuggestionCategoryStudy     = report.SuggestionCategoryStudy
	SuggestionCategorySocial    = report.SuggestionCategorySocial
	SuggestionCategoryHealth    = report.SuggestionCategoryHealth
	SuggestionCategoryDimension = report.SuggestionCategoryDimension
)

// NewID 创建报告ID（根包兼容导出）。
func NewID(id uint64) ID {
	return meta.FromUint64(id)
}

// ParseID 解析报告ID（根包兼容导出）。
func ParseID(s string) (ID, error) {
	return meta.ParseID(s)
}

func NewPendingInterpretReport(id ID, outcomeID meta.ID, at time.Time) (*InterpretReport, error) {
	return report.NewPendingInterpretReport(id, outcomeID, at)
}
