package calculation

import (
	errCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// TheOptionCalculater 选项计算器
type TheOptionCalculater struct{}

// Calculate 计算选项值
func (c TheOptionCalculater) Calculate(operands []Operand) (CalcResult, error) {
	if len(operands) == 0 {
		return CalcResult(0), errors.WithCode(errCode.ErrOperandsEmpty, "operands is empty")
	}
	if len(operands) > 1 {
		return CalcResult(0), errors.WithCode(errCode.ErrOperandsOverside, "operands is overside")
	}

	return CalcResult(operands[0].Value()), nil
}
