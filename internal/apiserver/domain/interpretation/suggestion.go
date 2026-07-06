package interpretation

// SuggestionCategory 建议分类。
type SuggestionCategory string

const (
	SuggestionCategoryGeneral   SuggestionCategory = "general"
	SuggestionCategoryFamily    SuggestionCategory = "family"
	SuggestionCategoryStudy     SuggestionCategory = "study"
	SuggestionCategorySocial    SuggestionCategory = "social"
	SuggestionCategoryHealth    SuggestionCategory = "health"
	SuggestionCategoryDimension SuggestionCategory = "dimension"
)

// Suggestion 结构化建议。
type Suggestion struct {
	Category   SuggestionCategory
	Content    string
	FactorCode *FactorCode
}

// SuggestionInput 建议生成输入。
type SuggestionInput struct {
	RiskLevel          RiskLevel
	HighRiskFactors    []FactorScoreInput
	OriginalSuggestion string
}
