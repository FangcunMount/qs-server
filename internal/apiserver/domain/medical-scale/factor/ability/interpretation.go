package ability

import "github.com/yshujie/questionnaire-scale/internal/pkg/interpretation"

type InterpretationAbility struct {
	interpretationRule *interpretation.InterpretRule
}

// GetInterpretationRule 获取解读规则
func (i *InterpretationAbility) GetInterpretationRule() *interpretation.InterpretRule {
	return i.interpretationRule
}

func (i *InterpretationAbility) SetInterpretationRule(interpretationRule *interpretation.InterpretRule) {
	i.interpretationRule = interpretationRule
}
