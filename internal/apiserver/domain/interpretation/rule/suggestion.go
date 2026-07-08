package rule

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"

type (
	SuggestionCategory = report.SuggestionCategory
	Suggestion         = report.Suggestion
	RiskLevel          = report.RiskLevel
	FactorCode         = report.FactorCode
)

const (
	SuggestionCategoryGeneral   = report.SuggestionCategoryGeneral
	SuggestionCategoryFamily    = report.SuggestionCategoryFamily
	SuggestionCategoryStudy     = report.SuggestionCategoryStudy
	SuggestionCategorySocial    = report.SuggestionCategorySocial
	SuggestionCategoryHealth    = report.SuggestionCategoryHealth
	SuggestionCategoryDimension = report.SuggestionCategoryDimension
)

// SuggestionInput 建议生成输入。
type SuggestionInput struct {
	RiskLevel          RiskLevel
	HighRiskFactors    []report.FactorScoreInput
	OriginalSuggestion string
}
