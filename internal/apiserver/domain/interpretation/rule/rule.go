package rule

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"

// LevelRule matches score ranges 到结果等级。
type LevelRule struct {
	Min        float64
	Max        float64
	RiskLevel  report.RiskLevel
	Conclusion string
	Suggestion string
}

// DimensionRule matches 维度分到 interpretive text。
type DimensionRule struct {
	Code       string
	Min        float64
	Max        float64
	Conclusion string
	Suggestion string
}

// SuggestionRule 是 strategy contract 用于 generating report suggestions。
type SuggestionRule = SuggestionStrategy
