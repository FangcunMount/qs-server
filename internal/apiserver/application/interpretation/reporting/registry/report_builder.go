package registry

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

// ReportBuilder assembles immutable report content from Interpretation-owned
// input. It never receives an Assessment aggregate or an Evaluation outcome
// application object.
type ReportBuilder interface {
	ExecutionIdentity() evaluation.ExecutionIdentity
	// Key 是deprecated; 使用 Execution身份()。
	Key() evaluation.ExecutionIdentity
	ReportType() domainReport.ReportType
	TemplateVersion() policy.TemplateVersion
	BuilderIdentity() string
	ContentSchemaVersion() string
	Build(ctx context.Context, input interpinput.InterpretationInput) (*report.Draft, error)
}
