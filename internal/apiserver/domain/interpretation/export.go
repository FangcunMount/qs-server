package interpretation

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/builder"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rule"
)

var (
	NewInterpretReport           = report.NewInterpretReport
	NewDimensionInterpret        = report.NewDimensionInterpret
	NewNeutralDimensionInterpret = report.NewNeutralDimensionInterpret
	NewFactorCode                = report.NewFactorCode
	NewDimensionCode             = report.NewDimensionCode
	NewRawTotalScore             = report.NewRawTotalScore
	NewMatchPercentScore         = report.NewMatchPercentScore
	LevelFromRisk                = report.LevelFromRisk
	IsHighRisk                   = report.IsHighRisk
	IsHighSeverity               = report.IsHighSeverity

	IsRiskLevelCode = rule.IsRiskLevelCode

	NewDefaultReportBuilder         = builder.NewDefaultReportBuilder
	NewRuleBasedSuggestionGenerator = rule.NewRuleBasedSuggestionGenerator
)

// NewFactorInterpretationSuggestionStrategy 创建基于因子解读配置的建议策略（根包兼容签名）。
func NewFactorInterpretationSuggestionStrategy(input GenerateReportInput) *FactorInterpretationSuggestionStrategy {
	return rule.NewFactorInterpretationSuggestionStrategy(input.Suggestion, input.FactorScores)
}

// AttentionRiskLevel 映射 v2 等级投影到旧版 risk_level 供 attention sync。
func AttentionRiskLevel(level *EventResultLevel) string {
	return report.AttentionRiskLevel(level)
}
