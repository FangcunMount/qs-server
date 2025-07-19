package calculation

import (
	errCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// MinCalculater 最小值计算器
type MinCalculater struct{}

// Calculate 计算最小值
func (c MinCalculater) Calculate(operands []Operand) (CalcResult, error) {
	if len(operands) == 0 {
		return CalcResult(0), errors.WithCode(errCode.ErrOperandsEmpty, "operands is empty")
	}

	min := operands[0]
	for _, operand := range operands[1:] {
		if operand.Value() < min.Value() {
			min = operand
		}
	}

	return CalcResult(min.Value()), nil
}
