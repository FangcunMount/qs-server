package strategies

import (
	"context"

	"github.com/fangcun-mount/qs-server/internal/evaluation-server/domain/calculation/rules"
)

// CalculationStrategy 计算策略接口
type CalculationStrategy interface {
	// Calculate 执行计算
	Calculate(ctx context.Context, operands []float64, rule *rules.CalculationRule) (*CalculationResult, error)
	// Validate 验证操作数和规则
	Validate(operands []float64, rule *rules.CalculationRule) error
	// GetStrategyName 获取策略名称
	GetStrategyName() string
	// GetDescription 获取策略描述
	GetDescription() string
}

// BaseStrategy 基础计算策略
type BaseStrategy struct {
	Name        string
	Description string
}

// GetStrategyName 获取策略名称
func (s *BaseStrategy) GetStrategyName() string {
	return s.Name
}

// GetDescription 获取策略描述
func (s *BaseStrategy) GetDescription() string {
	return s.Description
}

// Validate 基础验证实现
func (s *BaseStrategy) Validate(operands []float64, rule *rules.CalculationRule) error {
	if len(operands) == 0 {
		return NewCalculationError("", "操作数不能为空", operands, s.Name)
	}
	return nil
}

// CalculationResult 计算结果
type CalculationResult struct {
	Value       float64                `json:"value"`     // 计算结果
	Precision   int                    `json:"precision"` // 精度
	Metadata    map[string]interface{} `json:"metadata"`  // 元数据
	OperandInfo []OperandInfo          `json:"operands"`  // 操作数信息
	Strategy    string                 `json:"strategy"`  // 使用的策略
}

// OperandInfo 操作数信息
type OperandInfo struct {
	Value  float64 `json:"value"`
	Weight float64 `json:"weight,omitempty"`
	Label  string  `json:"label,omitempty"`
	Index  int     `json:"index"`
}

// NewCalculationResult 创建计算结果
func NewCalculationResult(value float64, strategy string) *CalculationResult {
	return &CalculationResult{
		Value:       value,
		Precision:   2,
		Metadata:    make(map[string]interface{}),
		OperandInfo: []OperandInfo{},
		Strategy:    strategy,
	}
}

// SetMetadata 设置元数据
func (r *CalculationResult) SetMetadata(key string, value interface{}) {
	if r.Metadata == nil {
		r.Metadata = make(map[string]interface{})
	}
	r.Metadata[key] = value
}

// AddOperandInfo 添加操作数信息
func (r *CalculationResult) AddOperandInfo(value, weight float64, label string, index int) {
	r.OperandInfo = append(r.OperandInfo, OperandInfo{
		Value:  value,
		Weight: weight,
		Label:  label,
		Index:  index,
	})
}

// CalculationError 计算错误
type CalculationError struct {
	Field    string      `json:"field"`
	Message  string      `json:"message"`
	Operands interface{} `json:"operands"`
	Strategy string      `json:"strategy"`
}

// Error 实现 error 接口
func (e *CalculationError) Error() string {
	return e.Message
}

// NewCalculationError 创建计算错误
func NewCalculationError(field, message string, operands interface{}, strategy string) *CalculationError {
	return &CalculationError{
		Field:    field,
		Message:  message,
		Operands: operands,
		Strategy: strategy,
	}
}
