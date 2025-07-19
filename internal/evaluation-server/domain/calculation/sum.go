package calculation

import (
	errCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// SumCalculater 求和计算器
type SumCalculater struct{}

// Calculate 计算求和值
func (c SumCalculater) Calculate(operands []Operand) (CalcResult, error) {
	if len(operands) == 0 {
		return CalcResult(0), errors.WithCode(errCode.ErrOperandsEmpty, "operands is empty")
	}
	sum := 0.0
	for _, operand := range operands {
		sum += operand.Value()
	}
	return CalcResult(sum), nil
}
