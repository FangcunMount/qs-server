package pipeline

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type RiskClassifier interface {
	Classify(
		medicalScale *evaluationinput.ScaleSnapshot,
		factorScores []assessment.FactorScoreResult,
	) ([]assessment.FactorScoreResult, assessment.RiskLevel)
}

type scaleRuleRiskClassifier struct{}

func NewRiskClassifier() RiskClassifier {
	return scaleRuleRiskClassifier{}
}

func (c scaleRuleRiskClassifier) Classify(
	medicalScale *evaluationinput.ScaleSnapshot,
	factorScores []assessment.FactorScoreResult,
) ([]assessment.FactorScoreResult, assessment.RiskLevel) {
	updatedScores := make([]assessment.FactorScoreResult, 0, len(factorScores))
	for _, fs := range factorScores {
		riskLevel := c.calculateFactorRiskLevel(medicalScale, fs.FactorCode, fs.RawScore)
		updatedScores = append(updatedScores, assessment.NewFactorScoreResult(
			fs.FactorCode,
			fs.FactorName,
			fs.RawScore,
			riskLevel,
			fs.Conclusion,
			fs.Suggestion,
			fs.IsTotalScore,
		))
	}
	return updatedScores, c.calculateOverallRiskLevel(medicalScale, updatedScores)
}

func (c scaleRuleRiskClassifier) calculateFactorRiskLevel(
	medicalScale *evaluationinput.ScaleSnapshot,
	factorCode assessment.FactorCode,
	score float64,
) assessment.RiskLevel {
	if medicalScale != nil {
		if factor, found := medicalScale.FindFactor(string(factorCode)); found {
			if rule := factor.FindInterpretRule(score); rule != nil {
				return convertRiskLevel(rule.RiskLevel)
			}
		}
	}
	return defaultRiskLevelByScore(score)
}

func (c scaleRuleRiskClassifier) calculateOverallRiskLevel(
	medicalScale *evaluationinput.ScaleSnapshot,
	factorScores []assessment.FactorScoreResult,
) assessment.RiskLevel {
	if medicalScale != nil {
		for _, fs := range factorScores {
			if fs.IsTotalScore {
				if factor, found := medicalScale.FindFactor(string(fs.FactorCode)); found {
					if rule := factor.FindInterpretRule(fs.RawScore); rule != nil {
						return convertRiskLevel(rule.RiskLevel)
					}
				}
			}
		}
	}

	maxRisk := assessment.RiskLevelNone
	for _, fs := range factorScores {
		if riskLevelOrder(fs.RiskLevel) > riskLevelOrder(maxRisk) {
			maxRisk = fs.RiskLevel
		}
	}
	return maxRisk
}

func defaultRiskLevelByScore(score float64) assessment.RiskLevel {
	switch {
	case score >= 80:
		return assessment.RiskLevelSevere
	case score >= 60:
		return assessment.RiskLevelHigh
	case score >= 40:
		return assessment.RiskLevelMedium
	case score >= 20:
		return assessment.RiskLevelLow
	default:
		return assessment.RiskLevelNone
	}
}

func convertRiskLevel(level string) assessment.RiskLevel {
	switch level {
	case string(assessment.RiskLevelNone):
		return assessment.RiskLevelNone
	case string(assessment.RiskLevelLow):
		return assessment.RiskLevelLow
	case string(assessment.RiskLevelMedium):
		return assessment.RiskLevelMedium
	case string(assessment.RiskLevelHigh):
		return assessment.RiskLevelHigh
	case string(assessment.RiskLevelSevere):
		return assessment.RiskLevelSevere
	default:
		return assessment.RiskLevelNone
	}
}

func riskLevelOrder(level assessment.RiskLevel) int {
	switch level {
	case assessment.RiskLevelNone:
		return 0
	case assessment.RiskLevelLow:
		return 1
	case assessment.RiskLevelMedium:
		return 2
	case assessment.RiskLevelHigh:
		return 3
	case assessment.RiskLevelSevere:
		return 4
	default:
		return 0
	}
}
