// Package execute 评估引擎
// 负责调度一次测评执行，由 qs-worker 消费 evaluation.requested 后调用。
//
// 设计说明：
// execute 只承担通用编排：加载 Assessment、解析输入快照、解析 RuntimeDescriptor、
// 将评分事实可靠提交为 EvaluationOutcome，并统一收口失败；
// 具体评分算法由 scale / personality typology 等机制实现。
package execute

import (
	"context"
)

// WorkerExecutionService 服务于 Worker 的一次评估执行。
//
// Worker 只根据 evaluation.requested 调用该入口，不能直接发起后台批量执行。
type WorkerExecutionService interface {
	// Evaluate 执行评估
	Evaluate(ctx context.Context, assessmentID uint64) error
}

// Engine is the Worker-facing concrete execution capability.
type Engine interface{ WorkerExecutionService }
