// Package execute 评估引擎
// 负责调度一次测评执行，由 qs-worker 消费 AssessmentSubmittedEvent 后调用。
//
// 设计说明：
// execute 只承担通用编排：加载 Assessment、解析输入快照、按 EvaluatorKey 选择执行器、
// 将评分事实可靠提交为 EvaluationOutcome，并统一收口失败；
// 具体模型解释由 scale / personality typology 等插件实现。
package execute

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// WorkerExecutionService 服务于 Worker 的一次评估执行。
//
// Worker 只根据 evaluation.requested 调用该入口，不能直接发起后台批量执行。
type WorkerExecutionService interface {
	// Evaluate 执行评估
	Evaluate(ctx context.Context, assessmentID uint64) error
}

// OperatorExecutionService 服务于后台操作者受控的批量执行。
//
// 失败 Assessment 的常规恢复仍应通过 AssessmentOperatorRecoveryService
// 重新发布 evaluation.requested；该入口仅保留既有的受控批量运维能力。
type OperatorExecutionService interface {
	// EvaluateBatch 批量评估
	EvaluateBatch(ctx context.Context, orgID int64, assessmentIDs []uint64) (*BatchResult, error)
}

// Engine combines the role-specific execution capabilities for concrete
// assembly. Transports must depend on a narrow role port.
type Engine interface {
	WorkerExecutionService
	OperatorExecutionService
}

// Evaluator 执行某一类评估模型的评估。
type Evaluator interface {
	ExecutionIdentity() evaluation.ExecutionIdentity
	// Key 是deprecated; 使用 Execution身份()。
	Key() evaluation.ExecutionIdentity
	// Execute 执行评估模型并返回 canonical 结果。
	Execute(ctx context.Context, input ExecutionInput) (*domainoutcome.Execution, error)
}

// EvaluatorRegistry 评估模型评估器注册表。
type EvaluatorRegistry interface {
	Resolve(key evaluation.ExecutionIdentity) (Evaluator, error)
}

// DescriptorExecutor 执行 RuntimeDescriptor 已解析后的评估路径。
type DescriptorExecutor interface {
	Execute(ctx context.Context, descriptor evalpipeline.RuntimeDescriptor, input ExecutionInput) (*domainoutcome.Execution, error)
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
