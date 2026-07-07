// Package rule 负责interpretation rules 独立于 测评编码。
package rule

import (
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

// LevelRule matches score ranges 到 结果 等级。
type LevelRule struct {
	Min        float64
	Max        float64
	RiskLevel  domainreport.RiskLevel
	Conclusion string
	Suggestion string
}

// DimensionRule matches 维度分 到 interpretive text。
type DimensionRule struct {
	Code       string
	Min        float64
	Max        float64
	Conclusion string
	Suggestion string
}

// SuggestionRule 是strategy contract 用于 generating report suggestions。
type SuggestionRule = domainreport.SuggestionStrategy

var NewFactorInterpretationSuggestionStrategy = domainreport.NewFactorInterpretationSuggestionStrategy
