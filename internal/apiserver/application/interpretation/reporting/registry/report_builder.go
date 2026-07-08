package registry

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
