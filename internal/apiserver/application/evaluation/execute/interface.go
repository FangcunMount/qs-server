// Package engine 评估引擎
// 负责调度一次测评执行，由 qs-worker 消费 AssessmentSubmittedEvent 后调用。
//
// 设计说明：
// engine 只承担通用编排：加载 Assessment、解析输入快照、选择模型执行器、
// 写入结果并统一收口失败；具体模型解释由 Scale/MBTI 等 executor 实现。
package execute

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// Service 评估引擎服务接口
// 行为者：评估引擎 (Evaluation Engine / qs-worker)
// 职责：执行一次通用测评编排
// 变更来源：测评执行流程、失败收口、结果写入边界
// 说明：此服务由 qs-worker 调用，消费 AssessmentSubmittedEvent
type Service interface {
	// Evaluate 执行评估
	// 场景：qs-worker 消费 AssessmentSubmittedEvent 后调用
	// 流程：
	//   1. 加载 Assessment 并解析 evaluationinput snapshot
	//   2. 按 EvaluationModelKind 选择 executor
	//   3. executor 返回通用 EvaluationResult
	//   4. result writer 保存兼容投影、报告和事件
	Evaluate(ctx context.Context, assessmentID uint64) error

	// EvaluateBatch 批量评估
	// 场景：批量处理积压的测评任务
	EvaluateBatch(ctx context.Context, orgID int64, assessmentIDs []uint64) (*BatchResult, error)
}

// Evaluator 执行某一类评估模型的评估。
type Evaluator interface {
	// Key 返回 v2 执行路由键。
	Key() evaluation.EvaluatorKey
	// Kind 返回兼容用 flat kind（报告/投影注册仍使用）。
	Kind() assessment.EvaluationModelKind
	// Execute 执行评估模型并返回 canonical outcome。
	Execute(ctx context.Context, input ExecutionInput) (*assessment.AssessmentOutcome, error)
}

// EvaluatorRegistry 评估模型评估器注册表。
type EvaluatorRegistry interface {
	Resolve(key evaluation.EvaluatorKey) (Evaluator, error)
	ResolveLegacyKind(kind assessment.EvaluationModelKind) (Evaluator, error)
}

// ExecutionInput 执行输入
type ExecutionInput struct {
	Assessment *assessment.Assessment
	Input      *evaluationinput.InputSnapshot
}

// BatchResult 批量评估结果
type BatchResult struct {
	TotalCount   int      // 总数
	SuccessCount int      // 成功数
	FailedCount  int      // 失败数
	FailedIDs    []uint64 // 失败的测评ID列表
}
