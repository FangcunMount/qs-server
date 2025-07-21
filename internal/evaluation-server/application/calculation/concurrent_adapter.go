package calculation

import (
	"context"
	"fmt"
	"time"

	"github.com/yshujie/questionnaire-scale/internal/evaluation-server/domain/calculation"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// ConcurrentCalculationAdapter 并发计算适配器
// 实现计算端口接口，使用并发执行策略，直接操作计算引擎
type ConcurrentCalculationAdapter struct {
	calculationEngine *calculation.CalculationEngine
	maxConcurrency    int
}

// NewConcurrentCalculationAdapter 创建并发计算适配器
func NewConcurrentCalculationAdapter(maxConcurrency int) *ConcurrentCalculationAdapter {
	if maxConcurrency <= 0 {
		maxConcurrency = 10 // 默认并发数
	}

	return &ConcurrentCalculationAdapter{
		calculationEngine: calculation.GetGlobalCalculationEngine(),
		maxConcurrency:    maxConcurrency,
	}
}

// SetMaxConcurrency 设置最大并发数
func (a *ConcurrentCalculationAdapter) SetMaxConcurrency(maxConcurrency int) {
	if maxConcurrency > 0 {
		a.maxConcurrency = maxConcurrency
	}
}

// GetMaxConcurrency 获取最大并发数
func (a *ConcurrentCalculationAdapter) GetMaxConcurrency() int {
	return a.maxConcurrency
}

// Calculate 执行单个计算任务
func (a *ConcurrentCalculationAdapter) Calculate(ctx context.Context, request *CalculationRequest) (*CalculationResult, error) {
	if request == nil {
		return nil, fmt.Errorf("计算请求不能为空")
	}

	log.Debugf("并发适配器: 执行单个计算任务 %s", request.ID)
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

// CalculateBatch 批量计算（并发执行）
func (a *ConcurrentCalculationAdapter) CalculateBatch(ctx context.Context, requests []*CalculationRequest) ([]*CalculationResult, error) {
	if len(requests) == 0 {
		return []*CalculationResult{}, nil
	}

	log.Infof("并发适配器: 开始批量计算，任务数量: %d, 最大并发数: %d", len(requests), a.maxConcurrency)

	// 创建工作池
	taskChan := make(chan struct {
		index   int
		request *CalculationRequest
	}, len(requests))

	resultChan := make(chan struct {
		index  int
		result *CalculationResult
		err    error
	}, len(requests))

	// 启动工作协程
	for i := 0; i < a.maxConcurrency; i++ {
		go func(workerID int) {
			for task := range taskChan {
				result, err := a.Calculate(ctx, task.request)
				resultChan <- struct {
					index  int
					result *CalculationResult
					err    error
				}{task.index, result, err}

				if err == nil && result.Error == "" {
					log.Debugf("Worker %d: 计算完成，任务 %s，结果: %f", workerID, task.request.Name, result.Value)
				}
			}
		}(i)
	}

	// 发送任务
	for i, request := range requests {
		taskChan <- struct {
			index   int
			request *CalculationRequest
		}{i, request}
	}
	close(taskChan)

	// 收集结果
	results := make([]*CalculationResult, len(requests))
	successCount := 0
	errorCount := 0

	for i := 0; i < len(requests); i++ {
		taskResult := <-resultChan
		if taskResult.err != nil {
			return nil, fmt.Errorf("并发计算失败，任务 %d: %w", taskResult.index, taskResult.err)
		}

		results[taskResult.index] = taskResult.result
		if taskResult.result.Error == "" {
			successCount++
		} else {
			errorCount++
		}
	}

	log.Infof("并发批量计算完成，共 %d 个任务，成功 %d 个，失败 %d 个，使用 %d 个 worker",
		len(requests), successCount, errorCount, a.maxConcurrency)
	return results, nil
}
