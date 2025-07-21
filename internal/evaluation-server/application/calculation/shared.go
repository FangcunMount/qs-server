package calculation

import (
	"github.com/yshujie/questionnaire-scale/internal/evaluation-server/domain/calculation/rules"
	"github.com/yshujie/questionnaire-scale/internal/evaluation-server/domain/calculation/strategies"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// CalculationRequest 计算请求
type CalculationRequest struct {
	ID           string                 `json:"id"`            // 计算任务ID
	Name         string                 `json:"name"`          // 计算任务名称
	FormulaType  string                 `json:"formula_type"`  // 计算公式类型
	Operands     []float64              `json:"operands"`      // 操作数
	Parameters   map[string]interface{} `json:"parameters"`    // 额外参数
	Precision    int                    `json:"precision"`     // 精度要求
	RoundingMode string                 `json:"rounding_mode"` // 舍入模式
}

// CalculationResult 计算结果
type CalculationResult struct {
	ID       string                        `json:"id"`       // 对应请求ID
	Name     string                        `json:"name"`     // 计算任务名称
	Value    float64                       `json:"value"`    // 计算结果
	Details  *strategies.CalculationResult `json:"details"`  // 详细计算信息
	Error    string                        `json:"error"`    // 错误信息
	Duration int64                         `json:"duration"` // 计算耗时（纳秒）
}

// createCalculationRule 创建计算规则（共享函数）
func createCalculationRule(request *CalculationRequest) (*rules.CalculationRule, error) {
	// 映射公式类型到策略名称
	strategyName := mapFormulaTypeToStrategy(request.FormulaType)

	// 创建基础规则
	rule := rules.NewCalculationRule(strategyName)

	// 应用精度设置
	if request.Precision > 0 {
		rule.SetPrecision(request.Precision)
	}

	// 应用舍入模式
	if request.RoundingMode != "" {
		rule.SetRoundingMode(request.RoundingMode)
	}

	// 应用额外参数
	for key, value := range request.Parameters {
		rule.AddParam(key, value)
	}

	return rule, nil
}

// mapFormulaTypeToStrategy 映射公式类型到策略名称（共享函数）
func mapFormulaTypeToStrategy(formulaType string) string {
	switch formulaType {
	case "the_option", "score", "option":
		return "option"
	case "sum":
		return "sum"
	case "average", "avg":
		return "average"
	case "max", "maximum":
		return "max"
	case "min", "minimum":
		return "min"
	case "weighted", "weighted_average":
		return "weighted"
	default:
		log.Warnf("未识别的公式类型: %s, 使用默认策略: option", formulaType)
		return "option"
	}
}
