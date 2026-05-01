package pipeline

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretengine"
)

// buildInterpretConfig 将 scale.Factor 的解读规则转换为 interpretation.InterpretConfig
func (g *InterpretationGenerator) buildInterpretConfig(factor *scale.Factor) *interpretengine.Config {
	scaleRules := factor.GetInterpretRules()
	if len(scaleRules) == 0 {
		return nil
	}

	// 转换为领域解读规则
	// 注意：scale.ScoreRange 和 interpretation.InterpretRule 都使用左闭右开区间 [min, max)
	// 直接使用 Min() 和 Max() 即可保持区间类型一致
	rules := make([]interpretengine.RuleSpec, 0, len(scaleRules))
	for _, scaleRule := range scaleRules {
		rules = append(rules, interpretengine.RuleSpec{
			Min:         scaleRule.GetScoreRange().Min(),
			Max:         scaleRule.GetScoreRange().Max(),
			RiskLevel:   string(scaleRule.GetRiskLevel()),
			Label:       string(scaleRule.GetRiskLevel()),
			Description: scaleRule.GetConclusion(),
			Suggestion:  scaleRule.GetSuggestion(),
		})
	}

	return &interpretengine.Config{
		FactorCode: factor.GetCode().Value(),
		Rules:      rules,
		Params:     nil,
	}
}
