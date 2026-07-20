package scoring

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scorerange"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

// interpretScaleFactor 生成单个因子的结论/建议文案：优先命中解读规则，否则按已判定风险等级回退默认文案。
// 未命中规则时不再取末段规则（与 calculation 判定契约对齐，MC-R004）。
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
		rules := model.Factors[i].InterpretRules
		if len(rules) == 0 {
			return nil
		}
		bounds := make([]scorerange.Bound, len(rules))
		for j, rule := range rules {
			bounds[j] = scorerange.Bound{
				Min: rule.Min, Max: rule.Max, MaxInclusive: rule.MaxInclusive, UnboundedMax: rule.UnboundedMax,
			}
		}
		index, ok := scorerange.MatchBounds(score, bounds)
		if !ok {
			return nil
		}
		return &rules[index]
	}
	return nil
}

func defaultScaleFactorInterpretation(factorName string, riskLevel report.RiskLevel, score float64) (string, string) {
	switch riskLevel {
	case report.RiskLevelSevere:
		return fmt.Sprintf("%s得分%.1f分，处于严重异常水平", factorName, score), "建议立即寻求专业帮助，进行进一步评估"
	case report.RiskLevelHigh:
		return fmt.Sprintf("%s得分%.1f分，处于较高风险水平", factorName, score), "建议尽快咨询专业人员，了解更多信息"
	case report.RiskLevelMedium:
		return fmt.Sprintf("%s得分%.1f分，处于中等水平", factorName, score), "建议关注相关方面，适当调整生活方式"
	case report.RiskLevelLow:
		return fmt.Sprintf("%s得分%.1f分，处于正常偏低水平", factorName, score), "整体情况良好，保持当前状态"
	default:
		return fmt.Sprintf("%s得分%.1f分，处于正常水平", factorName, score), "状态良好，继续保持"
	}
}
