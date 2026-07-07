// Package rule owns interpretation rules independent of assessment code.
package rule

import (
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

// LevelRule matches score ranges to result levels.
type LevelRule struct {
	Min        float64
	Max        float64
	RiskLevel  domainreport.RiskLevel
	Conclusion string
	Suggestion string
}

// DimensionRule matches dimension scores to interpretive text.
type DimensionRule struct {
	Code       string
	Min        float64
	Max        float64
	Conclusion string
	Suggestion string
}

// SuggestionRule is the strategy contract for generating report suggestions.
type SuggestionRule = domainreport.SuggestionStrategy

var NewFactorInterpretationSuggestionStrategy = domainreport.NewFactorInterpretationSuggestionStrategy
