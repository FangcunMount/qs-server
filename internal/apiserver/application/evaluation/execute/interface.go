// Package execute 评估引擎
// 负责调度一次测评执行，由 qs-worker 消费 AssessmentSubmittedEvent 后调用。
//
// 设计说明：
// execute 只承担通用编排：加载 Assessment、解析输入快照、按 EvaluatorKey 选择执行器、
// 经 scoringWriter + interpretationService 分两阶段写入结果，并统一收口失败；
// 具体模型解释由 scale / personality typology 等插件实现。
package execute

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// Service 评估引擎服务接口
// 行为者：评估引擎 (Evaluation Engine / qs-worker)
// 职责：执行一次通用测评编排
// 变更来源：测评执行流程、失败收口、结果写入边界
// 说明：此服务由 qs-worker 调用，消费 AssessmentSubmittedEvent
type Service interface {
	// Evaluate 执行评估
	Evaluate(ctx context.Context, assessmentID uint64) error

	// GenerateReport 基于已计分结果生成报告（异步解读阶段）
	GenerateReport(ctx context.Context, assessmentID uint64) error

	// EvaluateBatch 批量评估
	EvaluateBatch(ctx context.Context, orgID int64, assessmentIDs []uint64) (*BatchResult, error)
}

// Evaluator 执行某一类评估模型的评估。
type Evaluator interface {
	ExecutionIdentity() evaluation.ExecutionIdentity
	// Key 是deprecated; 使用 Execution身份()。
	Key() evaluation.ExecutionIdentity
	// Execute 执行评估模型并返回 canonical 结果。
	Execute(ctx context.Context, input ExecutionInput) (*assessment.AssessmentOutcome, error)
}

// EvaluatorRegistry 评估模型评估器注册表。
type EvaluatorRegistry interface {
	Resolve(key evaluation.ExecutionIdentity) (Evaluator, error)
}

// DescriptorExecutor 执行 RuntimeDescriptor 已解析后的评估路径。
type DescriptorExecutor interface {
	Execute(ctx context.Context, descriptor evalpipeline.RuntimeDescriptor, input ExecutionInput) (*assessment.AssessmentOutcome, error)
}

// ExecutionInput 执行输入
type ExecutionInput struct {
	Assessment *assessment.Assessment
	Input      *evaluationinput.InputSnapshot
}

// BatchResult 批量评估结果
type BatchResult struct {
	TotalCount   int
	SuccessCount int
	FailedCount  int
	FailedIDs    []uint64
}
