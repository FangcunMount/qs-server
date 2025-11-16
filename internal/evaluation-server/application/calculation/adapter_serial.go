package calculation

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/evaluation-server/domain/calculation"
	"github.com/FangcunMount/qs-server/pkg/log"
)

// SerialCalculationAdapter 串行计算适配器
// 实现计算端口接口，使用串行执行策略，直接操作计算引擎
type SerialCalculationAdapter struct {
	calculationEngine *calculation.CalculationEngine
}

// NewSerialCalculationAdapter 创建串行计算适配器
func NewSerialCalculationAdapter() *SerialCalculationAdapter {
	return &SerialCalculationAdapter{
		calculationEngine: calculation.GetGlobalCalculationEngine(),
	}
}

// Calculate 执行单个计算任务
func (a *SerialCalculationAdapter) Calculate(ctx context.Context, request *CalculationRequest) (*CalculationResult, error) {
	if request == nil {
		return nil, fmt.Errorf("计算请求不能为空")
	}

	log.Debugf("串行适配器: 执行单个计算任务 %s", request.ID)
	startTime := time.Now().UnixNano()

	// 创建计算规则
	rule, err := createCalculationRule(request)
	if err != nil {
		return &CalculationResult{
			ID:    request.ID,
			Name:  request.Name,
			Error: err.Error(),
		}, nil
	}

	// 执行计算
	result, err := a.calculationEngine.Calculate(ctx, request.Operands, rule)
	duration := time.Now().UnixNano() - startTime

	if err != nil {
		return &CalculationResult{
			ID:       request.ID,
			Name:     request.Name,
			Error:    err.Error(),
			Duration: duration,
		}, nil
	}

	return &CalculationResult{
		ID:       request.ID,
		Name:     request.Name,
		Value:    result.Value,
		Details:  result,
		Duration: duration,
	}, nil
}

// CalculateBatch 批量计算（串行执行）
func (a *SerialCalculationAdapter) CalculateBatch(ctx context.Context, requests []*CalculationRequest) ([]*CalculationResult, error) {
	if len(requests) == 0 {
		return []*CalculationResult{}, nil
	}

	log.Infof("串行适配器: 开始批量计算，任务数量: %d", len(requests))
	results := make([]*CalculationResult, len(requests))

	for i, request := range requests {
		result, err := a.Calculate(ctx, request)
		if err != nil {
			return nil, fmt.Errorf("串行批量计算失败，任务 %d: %w", i, err)
		}
		results[i] = result
	}

	log.Infof("串行批量计算完成，共 %d 个任务", len(requests))
	return results, nil
}
