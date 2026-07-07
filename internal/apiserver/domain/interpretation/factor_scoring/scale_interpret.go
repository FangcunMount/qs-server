package factor_scoring

import (
	"fmt"

	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

// interpretScaleFactor 生成单个因子的结论/建议文案：优先命中解读规则，否则回退默认文案。
// 该逻辑由 evaluation 领域下沉至此，evaluation 只负责产出分数/等级等事实。
func interpretScaleFactor(model *ReportModel, fs FactorReportScore) (string, string) {
	if rule := findFactorInterpretRule(model, fs.FactorCode, fs.RawScore); rule != nil && rule.Conclusion != "" {
		return rule.Conclusion, rule.Suggestion
	}
	return defaultScaleFactorInterpretation(fs.FactorName, fs.RiskLevel, fs.RawScore)
}

func findFactorInterpretRule(model *ReportModel, factorCode string, score float64) *FactorInterpretRule {
	if model == nil {
		return nil
	}
	for i := range model.Factors {
		if model.Factors[i].Code != factorCode {
			continue
		}
		return findInterpretRuleWithRangeFallback(model.Factors[i].InterpretRules, score)
	}
	return nil
}

func findInterpretRuleWithRangeFallback(rules []FactorInterpretRule, score float64) *FactorInterpretRule {
	for i := range rules {
		if rules[i].Matches(score) {
			return &rules[i]
		}
	}
	if len(rules) == 0 {
		return nil
	}
	last := rules[len(rules)-1]
	return &last
}

func defaultScaleFactorInterpretation(factorName string, riskLevel domainreport.RiskLevel, score float64) (string, string) {
	switch riskLevel {
	case domainreport.RiskLevelSevere:
		return fmt.Sprintf("%s得分%.1f分，处于严重异常水平", factorName, score), "建议立即寻求专业帮助，进行进一步评估"
	case domainreport.RiskLevelHigh:
		return fmt.Sprintf("%s得分%.1f分，处于较高风险水平", factorName, score), "建议尽快咨询专业人员，了解更多信息"
	case domainreport.RiskLevelMedium:
		return fmt.Sprintf("%s得分%.1f分，处于中等水平", factorName, score), "建议关注相关方面，适当调整生活方式"
	case domainreport.RiskLevelLow:
		return fmt.Sprintf("%s得分%.1f分，处于正常偏低水平", factorName, score), "整体情况良好，保持当前状态"
	default:
		return fmt.Sprintf("%s得分%.1f分，处于正常水平", factorName, score), "状态良好，继续保持"
	}
}
