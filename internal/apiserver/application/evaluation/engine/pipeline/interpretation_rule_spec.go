package pipeline

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretengine"
)

// buildInterpretConfig 将输入快照中的解读规则转换为 interpretengine.Config
func (g *InterpretationGenerator) buildInterpretConfig(factor *evaluationinput.FactorSnapshot) *interpretengine.Config {
	scaleRules := factor.InterpretRules
	if len(scaleRules) == 0 {
		return nil
	}

	// 转换为领域解读规则
	// 注意：scale.ScoreRange 和 interpretation.InterpretRule 都使用左闭右开区间 [min, max)
	// 直接使用 Min() 和 Max() 即可保持区间类型一致
	rules := make([]interpretengine.RuleSpec, 0, len(scaleRules))
	for _, scaleRule := range scaleRules {
		rules = append(rules, interpretengine.RuleSpec{
			Min:         scaleRule.Min,
			Max:         scaleRule.Max,
			RiskLevel:   scaleRule.RiskLevel,
			Label:       scaleRule.RiskLevel,
			Description: scaleRule.Conclusion,
			Suggestion:  scaleRule.Suggestion,
		})
	}

	return &interpretengine.Config{
		FactorCode: factor.Code,
		Rules:      rules,
		Params:     nil,
	}
}
