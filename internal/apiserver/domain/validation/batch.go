package validation

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"
)

// ==================== 批量校验任务定义 ====================

// ValidationTask 校验任务
// 封装单个校验任务的输入参数
type ValidationTask struct {
	ID    string           // 任务标识（如题目编码）
	Value ValidatableValue // 待校验的值
	Rules []ValidationRule // 校验规则列表
}

// TaskResult 校验任务结果
type TaskResult struct {
	ID     string            // 任务标识（对应 ValidationTask.ID）
	Result *ValidationResult // 校验结果
}

// ==================== 批量校验器 ====================

// BatchValidator 批量校验器
// 支持串行和并发两种校验模式
type BatchValidator struct {
	validator *DefaultValidator
}

// NewBatchValidator 创建批量校验器
func NewBatchValidator() *BatchValidator {
	return &BatchValidator{
		validator: NewDefaultValidator(),
	}
}

// ValidateAll 串行批量校验
// 适用于任务量较小或需要保证顺序的场景
func (b *BatchValidator) ValidateAll(tasks []ValidationTask) []TaskResult {
	results := make([]TaskResult, len(tasks))

	for i, task := range tasks {
		result := b.validator.ValidateValue(task.Value, task.Rules)
		results[i] = TaskResult{
			ID:     task.ID,
			Result: result,
		}
	}

	return results
}

// ValidateAllConcurrent 并发批量校验（使用 errgroup）
// 适用于大量任务需要快速校验的场景
// workerCount: 并发工作协程数，0 表示使用默认值（任务数/10，最少1，最多100）
func (b *BatchValidator) ValidateAllConcurrent(ctx context.Context, tasks []ValidationTask, workerCount int) ([]TaskResult, error) {
	if len(tasks) == 0 {
		return []TaskResult{}, nil
	}

	// 少量任务直接串行处理
	if len(tasks) < 10 {
		return b.ValidateAll(tasks), nil
	}

	// 计算工作协程数
	if workerCount <= 0 {
		workerCount = len(tasks) / 10
		if workerCount < 1 {
			workerCount = 1
		}
		if workerCount > 100 {
			workerCount = 100
		}
	}

	// 使用 errgroup 进行并发控制
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(workerCount)

	// 结果收集（需要线程安全）
	results := make([]TaskResult, len(tasks))
	var mu sync.Mutex

	// 发送任务到 errgroup
	for i, task := range tasks {
		i, task := i, task // 捕获循环变量
		g.Go(func() error {
			// 检查上下文是否已取消
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// 执行校验
			result := b.validator.ValidateValue(task.Value, task.Rules)

			// 安全写入结果
			mu.Lock()
			results[i] = TaskResult{
				ID:     task.ID,
				Result: result,
			}
			mu.Unlock()

			return nil
		})
	}

	// 等待所有任务完成
	if err := g.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

// ValidateAllToMap 批量校验并返回 Map
// key: 任务ID, value: 校验结果
func (b *BatchValidator) ValidateAllToMap(tasks []ValidationTask) map[string]TaskResult {
	results := b.ValidateAll(tasks)
	resultMap := make(map[string]TaskResult, len(results))
	for _, r := range results {
		resultMap[r.ID] = r
	}
	return resultMap
}

// ValidateAllConcurrentToMap 并发批量校验并返回 Map
func (b *BatchValidator) ValidateAllConcurrentToMap(ctx context.Context, tasks []ValidationTask, workerCount int) (map[string]TaskResult, error) {
	results, err := b.ValidateAllConcurrent(ctx, tasks, workerCount)
	if err != nil {
		return nil, err
	}

	resultMap := make(map[string]TaskResult, len(results))
	for _, r := range results {
		resultMap[r.ID] = r
	}
	return resultMap, nil
}

// ==================== 聚合结果辅助方法 ====================

// AggregatedResult 聚合校验结果
type AggregatedResult struct {
	Valid       bool                // 所有任务是否都通过
	TotalTasks  int                 // 总任务数
	PassedTasks int                 // 通过的任务数
	FailedTasks int                 // 失败的任务数
	Failures    map[string][]string // 失败任务的错误信息 (taskID -> error messages)
}

// Aggregate 聚合多个校验结果
func Aggregate(results []TaskResult) *AggregatedResult {
	agg := &AggregatedResult{
		Valid:      true,
		TotalTasks: len(results),
		Failures:   make(map[string][]string),
	}

	for _, r := range results {
		if r.Result.IsValid() {
			agg.PassedTasks++
		} else {
			agg.Valid = false
			agg.FailedTasks++

			// 收集错误信息
			var msgs []string
			for _, err := range r.Result.GetErrors() {
				msgs = append(msgs, err.GetMessage())
			}
			agg.Failures[r.ID] = msgs
		}
	}

	return agg
}

// ==================== 默认批量校验器实例 ====================

var defaultBatchValidator = NewBatchValidator()

// DefaultBatchValidator 获取默认批量校验器
func DefaultBatchValidator() *BatchValidator {
	return defaultBatchValidator
}

// BatchValidate 使用默认批量校验器校验（便捷函数）
func BatchValidate(tasks []ValidationTask) []TaskResult {
	return defaultBatchValidator.ValidateAll(tasks)
}

// BatchValidateConcurrent 使用默认批量校验器并发校验（便捷函数）
func BatchValidateConcurrent(ctx context.Context, tasks []ValidationTask, workerCount int) ([]TaskResult, error) {
	return defaultBatchValidator.ValidateAllConcurrent(ctx, tasks, workerCount)
}

// BatchValidateToMap 使用默认批量校验器校验并返回 Map（便捷函数）
func BatchValidateToMap(tasks []ValidationTask) map[string]TaskResult {
	return defaultBatchValidator.ValidateAllToMap(tasks)
}

// BatchValidateAndAggregate 批量校验并聚合结果（便捷函数）
func BatchValidateAndAggregate(tasks []ValidationTask) *AggregatedResult {
	results := defaultBatchValidator.ValidateAll(tasks)
	return Aggregate(results)
}
