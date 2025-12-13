package calculation

import "sync"

// ==================== 批量计算任务定义 ====================

// ScoreTask 计分任务
// 封装单个计分任务的输入参数
type ScoreTask struct {
	ID           string             // 任务标识（如题目编码）
	Value        ScorableValue      // 待计分的值
	OptionScores map[string]float64 // 选项分数映射
}

// ScoreResult 计分结果
type ScoreResult struct {
	ID       string  // 任务标识（对应 ScoreTask.ID）
	Score    float64 // 计算得分
	MaxScore float64 // 满分（选项中的最高分）
}

// ==================== 批量计分器 ====================

// BatchScorer 批量计分器
// 支持串行和并发两种计算模式
type BatchScorer struct {
	scorer *OptionScorer
}

// NewBatchScorer 创建批量计分器
func NewBatchScorer() *BatchScorer {
	return &BatchScorer{
		scorer: NewOptionScorer(),
	}
}

// ScoreAll 串行批量计分
// 适用于任务量较小或需要保证顺序的场景
func (b *BatchScorer) ScoreAll(tasks []ScoreTask) []ScoreResult {
	results := make([]ScoreResult, len(tasks))

	for i, task := range tasks {
		score, maxScore := b.scorer.ScoreWithMax(task.Value, task.OptionScores)
		results[i] = ScoreResult{
			ID:       task.ID,
			Score:    score,
			MaxScore: maxScore,
		}
	}

	return results
}

// ScoreAllConcurrent 并发批量计分
// 适用于大量任务需要快速计算的场景
// workerCount: 并发工作协程数，0 表示使用默认值（任务数/10，最少1）
func (b *BatchScorer) ScoreAllConcurrent(tasks []ScoreTask, workerCount int) []ScoreResult {
	if len(tasks) == 0 {
		return []ScoreResult{}
	}

	// 少量任务直接串行处理
	if len(tasks) < 10 {
		return b.ScoreAll(tasks)
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
				score, maxScore := b.scorer.ScoreWithMax(it.task.Value, it.task.OptionScores)
				resultChan <- indexedResult{
					index: it.index,
					result: ScoreResult{
						ID:       it.task.ID,
						Score:    score,
						MaxScore: maxScore,
					},
				}
			}
		}()
	}

	// 发送任务
	go func() {
		for i, task := range tasks {
			taskChan <- indexedTask{index: i, task: task}
		}
		close(taskChan)
	}()

	// 等待所有工作完成并关闭结果通道
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集结果（保持原始顺序）
	results := make([]ScoreResult, len(tasks))
	for ir := range resultChan {
		results[ir.index] = ir.result
	}

	return results
}

// ScoreAllToMap 批量计分并返回 Map
// key: 任务ID, value: 计分结果
func (b *BatchScorer) ScoreAllToMap(tasks []ScoreTask) map[string]ScoreResult {
	results := b.ScoreAll(tasks)
	resultMap := make(map[string]ScoreResult, len(results))
	for _, r := range results {
		resultMap[r.ID] = r
	}
	return resultMap
}

// 内部类型：带索引的任务
type indexedTask struct {
	index int
	task  ScoreTask
}

// 内部类型：带索引的结果
type indexedResult struct {
	index  int
	result ScoreResult
}

// ==================== 默认批量计分器实例 ====================

var defaultBatchScorer = NewBatchScorer()

// DefaultBatchScorer 获取默认批量计分器
func DefaultBatchScorer() *BatchScorer {
	return defaultBatchScorer
}

// BatchScore 使用默认批量计分器计算（便捷函数）
func BatchScore(tasks []ScoreTask) []ScoreResult {
	return defaultBatchScorer.ScoreAll(tasks)
}

// BatchScoreConcurrent 使用默认批量计分器并发计算（便捷函数）
func BatchScoreConcurrent(tasks []ScoreTask, workerCount int) []ScoreResult {
	return defaultBatchScorer.ScoreAllConcurrent(tasks, workerCount)
}

// BatchScoreToMap 使用默认批量计分器计算并返回 Map（便捷函数）
func BatchScoreToMap(tasks []ScoreTask) map[string]ScoreResult {
	return defaultBatchScorer.ScoreAllToMap(tasks)
}
