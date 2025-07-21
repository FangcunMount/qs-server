package calculation

import (
	"context"
)

// CalculationPort 计算端口接口（适配器模式的核心）
// 定义了计算的统一接口，不同的适配器实现不同的执行策略
type CalculationPort interface {
	// Calculate 执行单个计算任务
	Calculate(ctx context.Context, request *CalculationRequest) (*CalculationResult, error)

	// CalculateBatch 批量计算
	CalculateBatch(ctx context.Context, requests []*CalculationRequest) ([]*CalculationResult, error)
}
