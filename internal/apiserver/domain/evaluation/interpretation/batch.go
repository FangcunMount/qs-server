package interpretation

import "sync"

// ==================== 批量解读任务定义 ====================

// InterpretTask 解读任务
// 封装单个解读任务的输入参数
type InterpretTask struct {
	ID           string           // 任务标识（如因子编码）
	Score        float64          // 因子得分
	Config       *InterpretConfig // 解读配置
	StrategyType StrategyType     // 解读策略类型
}

// InterpretTaskResult 解读任务结果
type InterpretTaskResult struct {
	ID     string           // 任务标识（对应 InterpretTask.ID）
	Result *InterpretResult // 解读结果
	Error  error            // 错误信息
}

// ==================== 批量解读器 ====================

// BatchInterpreter 批量解读器
// 支持串行和并发两种解读模式
type BatchInterpreter struct {
	interpreter *DefaultInterpreter
}

// NewBatchInterpreter 创建批量解读器
func NewBatchInterpreter() *BatchInterpreter {
	return &BatchInterpreter{
		interpreter: NewDefaultInterpreter(),
	}
}

// InterpretAll 串行批量解读
// 适用于任务量较小或需要保证顺序的场景
func (b *BatchInterpreter) InterpretAll(tasks []InterpretTask) []InterpretTaskResult {
	results := make([]InterpretTaskResult, len(tasks))

	for i, task := range tasks {
		result, err := b.interpreter.InterpretFactor(task.Score, task.Config, task.StrategyType)
		results[i] = InterpretTaskResult{
			ID:     task.ID,
			Result: result,
			Error:  err,
		}
	}

	return results
}

// InterpretAllConcurrent 并发批量解读
// 适用于大量任务需要快速解读的场景
// workerCount: 并发工作协程数，0 表示使用默认值（任务数/10，最少1）
func (b *BatchInterpreter) InterpretAllConcurrent(tasks []InterpretTask, workerCount int) []InterpretTaskResult {
	if len(tasks) == 0 {
		return []InterpretTaskResult{}
	}

	// 少量任务直接串行处理
	if len(tasks) < 10 {
		return b.InterpretAll(tasks)
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

	// 创建任务通道和结果通道
	taskChan := make(chan indexedTask, len(tasks))
	resultChan := make(chan indexedResult, len(tasks))

	// 启动工作协程
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range taskChan {
				result, err := b.interpreter.InterpretFactor(it.task.Score, it.task.Config, it.task.StrategyType)
				resultChan <- indexedResult{
					index: it.index,
					result: InterpretTaskResult{
						ID:     it.task.ID,
						Result: result,
						Error:  err,
					},
				}
			}
		}()
	}

	// 发送任务
	go func() {
		for i, task := range tasks {
			taskChan <- indexedTask{
				index: i,
				task:  task,
			}
		}
		close(taskChan)
	}()

	// 等待所有工作协程完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集结果（保持原始顺序）
	results := make([]InterpretTaskResult, len(tasks))
	for ir := range resultChan {
		results[ir.index] = ir.result
	}

	return results
}

// ==================== 内部辅助类型 ====================

// indexedTask 带索引的任务
type indexedTask struct {
	index int
	task  InterpretTask
}

// indexedResult 带索引的结果
type indexedResult struct {
	index  int
	result InterpretTaskResult
}

// ==================== 默认批量解读器实例 ====================

// 默认批量解读器（单例）
var defaultBatchInterpreter = NewBatchInterpreter()

// GetDefaultBatchInterpreter 获取默认批量解读器
func GetDefaultBatchInterpreter() *BatchInterpreter {
	return defaultBatchInterpreter
}

// ==================== 便捷函数 ====================

// InterpretAll 使用默认批量解读器串行解读（便捷函数）
func InterpretAll(tasks []InterpretTask) []InterpretTaskResult {
	return defaultBatchInterpreter.InterpretAll(tasks)
}

// InterpretAllConcurrent 使用默认批量解读器并发解读（便捷函数）
func InterpretAllConcurrent(tasks []InterpretTask, workerCount int) []InterpretTaskResult {
	return defaultBatchInterpreter.InterpretAllConcurrent(tasks, workerCount)
}
