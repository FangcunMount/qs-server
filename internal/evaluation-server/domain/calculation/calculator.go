package calculation

import (
	"context"
	"sync"

	"github.com/fangcun-mount/qs-server/internal/evaluation-server/domain/calculation/rules"
	"github.com/fangcun-mount/qs-server/internal/evaluation-server/domain/calculation/strategies"
)

// CalculationEngine 主计算引擎
type CalculationEngine struct {
	strategyFactory *strategies.StrategyFactory
}

// NewCalculationEngine 创建计算引擎
func NewCalculationEngine() *CalculationEngine {
	return &CalculationEngine{
		strategyFactory: strategies.GetGlobalStrategyFactory(),
	}
}

// Calculate 执行计算
func (c *CalculationEngine) Calculate(ctx context.Context, operands []float64, rule *rules.CalculationRule) (*strategies.CalculationResult, error) {
	if rule == nil {
		return nil, NewCalculationError("", "计算规则不能为空", operands, "")
	}

	strategy, err := c.strategyFactory.GetStrategy(rule.Name)
	if err != nil {
		return nil, NewCalculationError("", err.Error(), operands, rule.Name)
	}

	return strategy.Calculate(ctx, operands, rule)
}

// CalculateWithStrategy 使用指定策略计算
func (c *CalculationEngine) CalculateWithStrategy(ctx context.Context, strategyName string, operands []float64, rule *rules.CalculationRule) (*strategies.CalculationResult, error) {
	if rule == nil {
		rule = rules.NewCalculationRule(strategyName)
	}

	strategy, err := c.strategyFactory.GetStrategy(strategyName)
	if err != nil {
		return nil, NewCalculationError("", err.Error(), operands, strategyName)
	}

	return strategy.Calculate(ctx, operands, rule)
}

// ValidateOperands 验证操作数
func (c *CalculationEngine) ValidateOperands(operands []float64, rule *rules.CalculationRule) error {
	if rule == nil {
		return NewCalculationError("", "计算规则不能为空", operands, "")
	}

	strategy, err := c.strategyFactory.GetStrategy(rule.Name)
	if err != nil {
		return NewCalculationError("", err.Error(), operands, rule.Name)
	}

	return strategy.Validate(operands, rule)
}

// RegisterCustomStrategy 注册自定义策略
func (c *CalculationEngine) RegisterCustomStrategy(strategy strategies.CalculationStrategy) error {
	return c.strategyFactory.RegisterStrategy(strategy)
}

// ListStrategies 列出所有策略
func (c *CalculationEngine) ListStrategies() []string {
	return c.strategyFactory.ListStrategies()
}

// HasStrategy 检查策略是否存在
func (c *CalculationEngine) HasStrategy(name string) bool {
	return c.strategyFactory.HasStrategy(name)
}

// GetStrategy 获取策略
func (c *CalculationEngine) GetStrategy(name string) (strategies.CalculationStrategy, error) {
	return c.strategyFactory.GetStrategy(name)
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

// BatchCalculator 批量计算器
type BatchCalculator struct {
	engine *CalculationEngine
}

// NewBatchCalculator 创建批量计算器
func NewBatchCalculator() *BatchCalculator {
	return &BatchCalculator{
		engine: NewCalculationEngine(),
	}
}

// BatchCalculationRequest 批量计算请求
type BatchCalculationRequest struct {
	Name     string                 `json:"name"`
	Operands []float64              `json:"operands"`
	Rule     *rules.CalculationRule `json:"rule"`
}

// BatchCalculationResult 批量计算结果
type BatchCalculationResult struct {
	Name   string                        `json:"name"`
	Result *strategies.CalculationResult `json:"result,omitempty"`
	Error  string                        `json:"error,omitempty"`
}

// CalculateBatch 批量计算
func (bc *BatchCalculator) CalculateBatch(ctx context.Context, requests []BatchCalculationRequest) []BatchCalculationResult {
	results := make([]BatchCalculationResult, len(requests))

	for i, req := range requests {
		result, err := bc.engine.Calculate(ctx, req.Operands, req.Rule)
		if err != nil {
			results[i] = BatchCalculationResult{
				Name:  req.Name,
				Error: err.Error(),
			}
		} else {
			results[i] = BatchCalculationResult{
				Name:   req.Name,
				Result: result,
			}
		}
	}

	return results
}

// 全局计算器实例
var (
	globalCalculationEngine *CalculationEngine
	once                    sync.Once
)

// GetGlobalCalculationEngine 获取全局计算引擎实例
func GetGlobalCalculationEngine() *CalculationEngine {
	once.Do(func() {
		globalCalculationEngine = NewCalculationEngine()
	})
	return globalCalculationEngine
}

// 便捷的全局函数

// Calculate 全局计算函数
func Calculate(ctx context.Context, operands []float64, rule *rules.CalculationRule) (*strategies.CalculationResult, error) {
	engine := GetGlobalCalculationEngine()
	return engine.Calculate(ctx, operands, rule)
}

// CalculateSum 计算求和
func CalculateSum(ctx context.Context, operands []float64) (*strategies.CalculationResult, error) {
	rule := Sum().Build()
	return Calculate(ctx, operands, rule)
}

// CalculateAverage 计算平均值
func CalculateAverage(ctx context.Context, operands []float64) (*strategies.CalculationResult, error) {
	rule := Average().Build()
	return Calculate(ctx, operands, rule)
}

// CalculateMax 计算最大值
func CalculateMax(ctx context.Context, operands []float64) (*strategies.CalculationResult, error) {
	rule := Max().Build()
	return Calculate(ctx, operands, rule)
}

// CalculateMin 计算最小值
func CalculateMin(ctx context.Context, operands []float64) (*strategies.CalculationResult, error) {
	rule := Min().Build()
	return Calculate(ctx, operands, rule)
}

// CalculateWeightedAverage 计算加权平均
func CalculateWeightedAverage(ctx context.Context, operands []float64, weights []float64) (*strategies.CalculationResult, error) {
	rule := WeightedAverage(weights).Build()
	return Calculate(ctx, operands, rule)
}
