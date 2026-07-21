package scoring

import (
	"errors"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scorerange"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
)

var (
	// ErrInterpretationRuleMiss classifies a configured score-range gap.
	ErrInterpretationRuleMiss = errors.New("factor interpretation rule miss")
	// ErrOutcomePresentationMiss classifies a frozen OutcomeCode without presentation copy.
	ErrOutcomePresentationMiss = errors.New("factor outcome presentation miss")
)

// interpretScaleFactor resolves factor presentation. Primary path: frozen OutcomeCode
// → InterpretationAssets; legacy fallback: score-range rematch on InterpretRules (MC-R016).
func interpretScaleFactor(model *ReportModel, fs FactorReportScore) (string, string, error) {
	hasAssets := model != nil && model.Assets != nil && model.Assets.IsMaterialized()
	if hasAssets {
		if conclusion, suggestion, ok := presentationFromOutcomeCode(*model.Assets, outcomeCodeFromFactorScore(fs)); ok {
			return conclusion, suggestion, nil
		}
	}
	rule, hasRules := findFactorInterpretRule(model, fs.FactorCode, fs.RawScore)
	if rule != nil && rule.Conclusion != "" {
		observeFactorInterpretationCompatibility("score_rule_rematch")
		return rule.Conclusion, rule.Suggestion, nil
	}
	if hasRules {
		return "", "", fmt.Errorf("%w: factor=%q score=%g", ErrInterpretationRuleMiss, fs.FactorCode, fs.RawScore)
	}
	if hasAssets {
		return "", "", fmt.Errorf("%w: factor=%q outcome_code=%q", ErrOutcomePresentationMiss, fs.FactorCode, outcomeCodeFromFactorScore(fs))
	}
	observeFactorInterpretationCompatibility("soft_default")
	conclusion, suggestion := defaultScaleFactorInterpretation(fs.FactorName, fs.RiskLevel, fs.RawScore)
	return conclusion, suggestion, nil
}

func outcomeCodeFromFactorScore(fs FactorReportScore) string {
	if fs.Level != nil && fs.Level.Code != "" {
		return fs.Level.Code
	}
	if fs.RiskLevel != "" && fs.RiskLevel != report.RiskLevelNone {
		return string(fs.RiskLevel)
	}
	return ""
}

func presentationFromOutcomeCode(assets interpretationassets.Assets, code string) (conclusion, suggestion string, ok bool) {
	if code == "" {
		return "", "", false
	}
	pres, found := assets.FindOutcome(code)
	if !found {
		return "", "", false
	}
	conclusion = firstNonEmpty(pres.Summary, pres.Title, pres.Description)
	suggestion = pres.Description
	if pres.Summary != "" && pres.Description != "" && pres.Summary != pres.Description {
		conclusion = pres.Summary
	}
	if conclusion == "" {
		return "", "", false
	}
	return conclusion, suggestion, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func findFactorInterpretRule(model *ReportModel, factorCode string, score float64) (*FactorInterpretRule, bool) {
	if model == nil {
		return nil, false
	}
	for i := range model.Factors {
		if model.Factors[i].Code != factorCode {
			continue
		}
		rules := model.Factors[i].InterpretRules
		if len(rules) == 0 {
			return nil, false
		}
		bounds := make([]scorerange.Bound, len(rules))
		for j, rule := range rules {
			bounds[j] = scorerange.Bound{
				Min: rule.Min, Max: rule.Max, MaxInclusive: rule.MaxInclusive, UnboundedMax: rule.UnboundedMax,
			}
		}
		index, ok := scorerange.MatchBounds(score, bounds)
		if !ok {
			return nil, true
		}
		return &rules[index], true
	}
	return nil, false
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
