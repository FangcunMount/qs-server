package scoring

func classifyRisk(model Model, factorScores []FactorScore) ([]FactorScore, RiskLevel) {
	updatedScores := make([]FactorScore, 0, len(factorScores))
	for _, fs := range factorScores {
		fs.RiskLevel = calculateFactorRiskLevel(model, fs.FactorCode, fs.RawScore)
		updatedScores = append(updatedScores, fs)
	}
	return updatedScores, calculateOverallRiskLevel(model, updatedScores)
}

func calculateFactorRiskLevel(model Model, factorCode string, score float64) RiskLevel {
	if factor, found := findFactor(model, factorCode); found {
		if rule := findInterpretRule(factor, score); rule != nil {
			return RiskLevel(rule.RiskLevel)
		}
	}
	return defaultRiskLevelByScore(score)
}

func calculateOverallRiskLevel(model Model, factorScores []FactorScore) RiskLevel {
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

func findFactor(model Model, factorCode string) (Factor, bool) {
	for _, factor := range model.Factors {
		if factor.Code == factorCode {
			return factor, true
		}
	}
	return Factor{}, false
}
