package report

// SuggestionCategory 建议分类。
type SuggestionCategory string

const (
	SuggestionCategoryGeneral   SuggestionCategory = "general"
	SuggestionCategoryDimension SuggestionCategory = "dimension"
)

// Suggestion 结构化建议。
type Suggestion struct {
	Category   SuggestionCategory
	Content    string
	FactorCode *FactorCode
}
