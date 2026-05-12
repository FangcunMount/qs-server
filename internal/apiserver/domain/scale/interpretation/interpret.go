package interpretation

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
)

func (*Evaluator) interpret(model ScaleInterpretationModel, factorScores []ScaleFactorScore, totalScore float64, riskLevel scale.RiskLevel) ([]ScaleFactorScore, string, string) {
	updatedScores := make([]ScaleFactorScore, 0, len(factorScores))
	for _, fs := range factorScores {
		fs.Conclusion, fs.Suggestion = interpretFactor(model, fs)
		updatedScores = append(updatedScores, fs)
	}
	conclusion, suggestion := interpretOverall(model, updatedScores, totalScore, riskLevel)
	return updatedScores, conclusion, suggestion
}

func interpretFactor(model ScaleInterpretationModel, fs ScaleFactorScore) (string, string) {
	if factor, found := findFactor(model, fs.FactorCode); found {
		if rule := findInterpretRuleWithRangeFallback(factor, fs.RawScore); rule != nil && rule.GetConclusion() != "" {
			return rule.GetConclusion(), rule.GetSuggestion()
		}
	}
	return defaultFactorInterpretation(fs.FactorName, fs.RiskLevel, fs.RawScore)
}

func interpretOverall(model ScaleInterpretationModel, factorScores []ScaleFactorScore, totalScore float64, riskLevel scale.RiskLevel) (string, string) {
	for _, fs := range factorScores {
		if !fs.IsTotalScore {
			continue
		}
		if factor, found := findFactor(model, fs.FactorCode); found {
			if rule := findInterpretRule(factor, fs.RawScore); rule != nil && rule.GetConclusion() != "" {
				return rule.GetConclusion(), rule.GetSuggestion()
			}
		}
	}
	return defaultOverallInterpretation(totalScore, riskLevel)
}

func defaultFactorInterpretation(factorName string, riskLevel scale.RiskLevel, score float64) (string, string) {
	switch riskLevel {
	case scale.RiskLevelSevere:
		return fmt.Sprintf("%s得分%.1f分，处于严重异常水平", factorName, score), "建议立即寻求专业帮助，进行进一步评估"
	case scale.RiskLevelHigh:
		return fmt.Sprintf("%s得分%.1f分，处于较高风险水平", factorName, score), "建议尽快咨询专业人员，了解更多信息"
	case scale.RiskLevelMedium:
		return fmt.Sprintf("%s得分%.1f分，处于中等水平", factorName, score), "建议关注相关方面，适当调整生活方式"
	case scale.RiskLevelLow:
		return fmt.Sprintf("%s得分%.1f分，处于正常偏低水平", factorName, score), "整体情况良好，保持当前状态"
	default:
		return fmt.Sprintf("%s得分%.1f分，处于正常水平", factorName, score), "状态良好，继续保持"
	}
}

func defaultOverallInterpretation(_ float64, riskLevel scale.RiskLevel) (string, string) {
	switch riskLevel {
	case scale.RiskLevelSevere:
		return "测评结果显示存在严重问题，需要立即关注", "强烈建议尽快寻求专业帮助，进行全面评估和干预"
	case scale.RiskLevelHigh:
		return "测评结果显示存在较高风险，需要重点关注", "建议尽快咨询专业人员，获取更详细的评估和指导"
	case scale.RiskLevelMedium:
		return "测评结果显示存在一定风险，需要适度关注", "建议关注相关方面的变化，必要时寻求专业帮助"
	case scale.RiskLevelLow:
		return "测评结果显示整体情况良好，少数方面需要注意", "保持健康的生活方式，定期进行自我检查"
	default:
		return "测评已完成，整体情况良好", "保持健康的生活方式"
	}
}
