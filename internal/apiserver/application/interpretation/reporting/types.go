// Package reporting owns interpretation write paths: report builders, event staging, and durable persistence.
package reporting

import (
	"context"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

// ReportBuilder materializes an InterpretReport from a scored outcome.
type ReportBuilder interface {
	Key() evaluation.EvaluatorKey
	ReportType() domainReport.ReportType
	Build(ctx context.Context, outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error)
}

// Writer persists reports and transitions Assessment to interpreted.
type Writer interface {
	Write(ctx context.Context, outcome evaloutcome.Outcome) error
}

// ScoreProjector projects scores after interpretation for synchronous scoring paths.
type ScoreProjector interface {
	Key() evaluation.EvaluatorKey
	Project(ctx context.Context, outcome evaloutcome.Outcome) error
}

// ScoreProjectorRegistry resolves score projectors by evaluator key or mechanism key.
type ScoreProjectorRegistry interface {
	Resolve(key evaluation.EvaluatorKey) ScoreProjector
	ResolveByMechanism(key MechanismReportBuilderKey) ScoreProjector
}

// CompletionNotifier notifies waiters after interpretation completes.
type CompletionNotifier interface {
	NotifyCompletion(ctx context.Context, outcome evaloutcome.Outcome)
}
