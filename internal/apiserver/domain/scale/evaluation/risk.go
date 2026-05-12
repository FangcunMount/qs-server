package evaluation

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
)

func (_ *Evaluator) classifyRisk(model ScaleEvaluationModel, factorScores []ScaleFactorScore) ([]ScaleFactorScore, scale.RiskLevel) {
	updatedScores := make([]ScaleFactorScore, 0, len(factorScores))
	for _, fs := range factorScores {
		fs.RiskLevel = calculateFactorRiskLevel(model, fs.FactorCode, fs.RawScore)
		updatedScores = append(updatedScores, fs)
	}
	return updatedScores, calculateOverallRiskLevel(model, updatedScores)
}

func calculateFactorRiskLevel(model ScaleEvaluationModel, factorCode scale.FactorCode, score float64) scale.RiskLevel {
	if factor, found := findFactor(model, factorCode); found {
		if rule := findInterpretRule(factor, score); rule != nil {
			return rule.GetRiskLevel()
		}
	}
	return defaultRiskLevelByScore(score)
}

func calculateOverallRiskLevel(model ScaleEvaluationModel, factorScores []ScaleFactorScore) scale.RiskLevel {
	for _, fs := range factorScores {
		if fs.IsTotalScore {
			if factor, found := findFactor(model, fs.FactorCode); found {
				if rule := findInterpretRule(factor, fs.RawScore); rule != nil {
					return rule.GetRiskLevel()
				}
			}
		}
	}

	maxRisk := scale.RiskLevelNone
	for _, fs := range factorScores {
		if riskLevelOrder(fs.RiskLevel) > riskLevelOrder(maxRisk) {
			maxRisk = fs.RiskLevel
		}
	}
	return maxRisk
}

func defaultRiskLevelByScore(score float64) scale.RiskLevel {
	switch {
	case score >= 80:
		return scale.RiskLevelSevere
	case score >= 60:
		return scale.RiskLevelHigh
	case score >= 40:
		return scale.RiskLevelMedium
	case score >= 20:
		return scale.RiskLevelLow
	default:
		return scale.RiskLevelNone
	}
}

func riskLevelOrder(level scale.RiskLevel) int {
	switch level {
	case scale.RiskLevelNone:
		return 0
	case scale.RiskLevelLow:
		return 1
	case scale.RiskLevelMedium:
		return 2
	case scale.RiskLevelHigh:
		return 3
	case scale.RiskLevelSevere:
		return 4
	default:
		return 0
	}
}

func findFactor(model ScaleEvaluationModel, factorCode scale.FactorCode) (scale.FactorSnapshot, bool) {
	for _, factor := range model.Factors {
		if factor.Code == factorCode {
			return factor, true
		}
	}
	return scale.FactorSnapshot{}, false
}

func findInterpretRule(factor scale.FactorSnapshot, score float64) *scale.InterpretationRule {
	rules := toScoreRangeRules(factor.InterpretRules)
	matched := interpretation.MatchRule(score, rules)
	if matched == nil {
		return nil
	}
	rule := scale.NewInterpretationRule(scale.NewScoreRange(matched.Min, matched.Max), scale.RiskLevel(matched.Level), matched.Conclusion, matched.Suggestion)
	return &rule
}

func findInterpretRuleWithRangeFallback(factor scale.FactorSnapshot, score float64) *scale.InterpretationRule {
	rules := toScoreRangeRules(factor.InterpretRules)
	matched := interpretation.MatchRuleWithRangeFallback(score, rules)
	if matched == nil {
		return nil
	}
	rule := scale.NewInterpretationRule(scale.NewScoreRange(matched.Min, matched.Max), scale.RiskLevel(matched.Level), matched.Conclusion, matched.Suggestion)
	return &rule
}

func toScoreRangeRules(rules []scale.InterpretationRule) []interpretation.ScoreRangeRule {
	converted := make([]interpretation.ScoreRangeRule, 0, len(rules))
	for _, rule := range rules {
		converted = append(converted, interpretation.ScoreRangeRule{
			Min:        rule.GetScoreRange().Min(),
			Max:        rule.GetScoreRange().Max(),
			Level:      string(rule.GetRiskLevel()),
			Conclusion: rule.GetConclusion(),
			Suggestion: rule.GetSuggestion(),
		})
	}
	return converted
}
