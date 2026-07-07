// Package reporting 负责解释写路径: 报告构建器, event staging, 和 持久化 持久化。
package reporting

import (
	"context"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

// ReportBuilder 物化InterpretReport 从 scored 结果。
type ReportBuilder interface {
	ExecutionIdentity() evaluation.ExecutionIdentity
	// Key 是deprecated; 使用 Execution身份()。
	Key() evaluation.ExecutionIdentity
	ReportType() domainReport.ReportType
	Build(ctx context.Context, outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error)
}

// Writer 持久化reports 和 transitions Assessment 到 interpreted。
type Writer interface {
	Write(ctx context.Context, outcome evaloutcome.Outcome) error
}

// ScoreProjector 投影分数 在之后 interpretation 用于 synchronous 计分 paths。
type ScoreProjector interface {
	ExecutionIdentity() evaluation.ExecutionIdentity
	// Key 是deprecated; 使用 Execution身份()。
	Key() evaluation.ExecutionIdentity
	Project(ctx context.Context, outcome evaloutcome.Outcome) error
}

// ScoreProjectorRegistry 解析score 投影器 按 评估器键 或 机制键。
type ScoreProjectorRegistry interface {
	Resolve(key evaluation.ExecutionIdentity) ScoreProjector
	ResolveByMechanism(key MechanismReportBuilderKey) ScoreProjector
}

// CompletionNotifier notifies waiters 在之后 interpretation completes。
type CompletionNotifier interface {
	NotifyCompletion(ctx context.Context, outcome evaloutcome.Outcome)
}
