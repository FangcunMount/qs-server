package scaleinterpretation

import rulesetscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale"

func (*Evaluator) classifyRisk(model ScaleInterpretationModel, factorScores []ScaleFactorScore) ([]ScaleFactorScore, RiskLevel) {
	updatedScores := make([]ScaleFactorScore, 0, len(factorScores))
	for _, fs := range factorScores {
		fs.RiskLevel = calculateFactorRiskLevel(model, fs.FactorCode, fs.RawScore)
		updatedScores = append(updatedScores, fs)
	}
	return updatedScores, calculateOverallRiskLevel(model, updatedScores)
}

func calculateFactorRiskLevel(model ScaleInterpretationModel, factorCode string, score float64) RiskLevel {
	if factor, found := findFactor(model, factorCode); found {
		if rule := findInterpretRule(factor, score); rule != nil {
			return RiskLevel(rule.RiskLevel)
		}
	}
	return defaultRiskLevelByScore(score)
}

func calculateOverallRiskLevel(model ScaleInterpretationModel, factorScores []ScaleFactorScore) RiskLevel {
	for _, fs := range factorScores {
		if fs.IsTotalScore {
			if factor, found := findFactor(model, fs.FactorCode); found {
				if rule := findInterpretRule(factor, fs.RawScore); rule != nil {
					return RiskLevel(rule.RiskLevel)
				}
			}
		}
	}

	maxRisk := RiskLevelNone
	for _, fs := range factorScores {
		if riskLevelOrder(fs.RiskLevel) > riskLevelOrder(maxRisk) {
			maxRisk = fs.RiskLevel
		}
	}
	return maxRisk
}

func defaultRiskLevelByScore(score float64) RiskLevel {
	switch {
	case score >= 80:
		return RiskLevelSevere
	case score >= 60:
		return RiskLevelHigh
	case score >= 40:
		return RiskLevelMedium
	case score >= 20:
		return RiskLevelLow
	default:
		return RiskLevelNone
	}
}

func riskLevelOrder(level RiskLevel) int {
	switch level {
	case RiskLevelNone:
		return 0
	case RiskLevelLow:
		return 1
	case RiskLevelMedium:
		return 2
	case RiskLevelHigh:
		return 3
	case RiskLevelSevere:
		return 4
	default:
		return 0
	}
}

func findFactor(model ScaleInterpretationModel, factorCode string) (rulesetscale.FactorSnapshot, bool) {
	for _, factor := range model.Factors {
		if factor.Code == factorCode {
			return factor, true
		}
	}
	return rulesetscale.FactorSnapshot{}, false
}
