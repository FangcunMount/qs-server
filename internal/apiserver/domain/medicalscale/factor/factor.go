package factor

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medicalscale/factor/ability"
)

// Factor 因子实体
type Factor struct {
	code       string
	title      string
	factorType FactorType

	calculationAbility    *ability.CalculationAbility
	interpretationAbility *ability.InterpretationAbility
}

// NewFactor 创建新的因子
func NewFactor(code, title string, factorType FactorType, opts ...FactorOption) Factor {
	f := Factor{
		code:       code,
		title:      title,
		factorType: factorType,
	}

	for _, opt := range opts {
		opt(&f)
	}

	return f
}

// FactorOption 因子选项
type FactorOption func(*Factor)

// WithCalculation 设置计算能力
func WithCalculation(calculationAbility *ability.CalculationAbility) FactorOption {
	return func(f *Factor) {
		if calculationAbility != nil {
			f.calculationAbility = calculationAbility
		}
	}
}

// WithInterpretation 设置解读能力
func WithInterpretation(interpretationAbility *ability.InterpretationAbility) FactorOption {
	return func(f *Factor) {
		if interpretationAbility != nil {
			f.interpretationAbility = interpretationAbility
		}
	}
}

// GetCode 获取因子代码
func (f Factor) GetCode() string {
	return f.code
}

// GetTitle 获取因子标题
func (f Factor) GetTitle() string {
	return f.title
}

// GetFactorType 获取因子类型
func (f Factor) GetFactorType() FactorType {
	return f.factorType
}

// GetCalculationAbility 获取计算能力
func (f Factor) GetCalculationAbility() *ability.CalculationAbility {
	return f.calculationAbility
}

// GetInterpretationAbility 获取解读能力
func (f Factor) GetInterpretationAbility() *ability.InterpretationAbility {
	return f.interpretationAbility
}
