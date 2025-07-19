package calculation

import (
	errCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// MaxCalculater 最大值计算器
type MaxCalculater struct{}

// Calculate 计算最大值
func (c MaxCalculater) Calculate(operands []Operand) (CalcResult, error) {
	if len(operands) == 0 {
		return CalcResult(0), errors.WithCode(errCode.ErrOperandsEmpty, "operands is empty")
	}

	max := operands[0]
	for _, operand := range operands[1:] {
		if operand.Value() > max.Value() {
			max = operand
		}
	}
	return CalcResult(max.Value()), nil
}
