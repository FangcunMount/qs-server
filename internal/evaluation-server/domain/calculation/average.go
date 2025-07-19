package calculation

import (
	"math"

	errCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// AverageCalculater 平均值计算器
type AverageCalculater struct{}

// Calculate 计算平均值，计算结果四舍五入
func (c AverageCalculater) Calculate(operands []Operand) (CalcResult, error) {
	if len(operands) == 0 {
		return CalcResult(0), errors.WithCode(errCode.ErrOperandsEmpty, "operands is empty")
	}
	sum := 0.0
	for _, operand := range operands {
		sum += operand.Value()
	}
	return CalcResult(math.Round(sum / float64(len(operands)))), nil
}
