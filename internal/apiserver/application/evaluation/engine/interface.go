// Package engine 评估引擎
// 负责执行测评的计分和解读流程，由 qs-worker 消费 AssessmentSubmittedEvent 后调用。
//
// 设计说明：
// 评估引擎是一个独立的模块，使用职责链模式（Pipeline）实现评估流程。
// 每个处理器负责一个独立的职责，遵循单一职责原则。
//
// 处理器链路：
//  1. ValidationHandler - 前置校验（校验输入数据完整性）
//  2. FactorScoreHandler - 因子分数计算（从答卷读取预计算分数，按因子聚合，计算总分）
//  3. RiskLevelHandler - 风险等级计算（计算因子/整体风险等级，保存得分）
//  4. InterpretationHandler - 测评分析解读、保存（生成结论建议，保存报告）
//  5. EventPublishHandler - 事件发布（发布 AssessmentInterpretedEvent）
package engine

import "context"

// Service 评估引擎服务接口
// 行为者：评估引擎 (Evaluation Engine / qs-worker)
// 职责：执行计分、解读、生成报告
// 变更来源：评估算法和流程变化
// 说明：此服务由 qs-worker 调用，消费 AssessmentSubmittedEvent
type Service interface {
	// Evaluate 执行评估
	// 场景：qs-worker 消费 AssessmentSubmittedEvent 后调用
	// 流程：
	//   1. 加载 Assessment、MedicalScale、Questionnaire、AnswerSheet
	//   2. 调用 calculation 功能域计算各因子得分
	//   3. 调用 interpretation 功能域生成解读
	//   4. 组装 EvaluationResult
	//   5. 应用评估结果到 Assessment
	//   6. 保存 AssessmentScore
	//   7. 生成并保存 InterpretReport
	Evaluate(ctx context.Context, assessmentID uint64) error

	// EvaluateBatch 批量评估
	// 场景：批量处理积压的测评任务
	EvaluateBatch(ctx context.Context, assessmentIDs []uint64) (*BatchResult, error)
}

// BatchResult 批量评估结果
type BatchResult struct {
	TotalCount   int      // 总数
	SuccessCount int      // 成功数
	FailedCount  int      // 失败数
	FailedIDs    []uint64 // 失败的测评ID列表
}
