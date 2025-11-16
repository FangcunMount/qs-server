package ability

import "github.com/FangcunMount/qs-server/internal/pkg/interpretation"

type InterpretationAbility struct {
	interpretationRules []interpretation.InterpretRule
}

// GetInterpretationRules 获取解读规则列表
func (i *InterpretationAbility) GetInterpretationRules() []interpretation.InterpretRule {
	return i.interpretationRules
}

// SetInterpretationRules 设置解读规则列表
func (i *InterpretationAbility) SetInterpretationRules(rules []interpretation.InterpretRule) {
	i.interpretationRules = rules
}
