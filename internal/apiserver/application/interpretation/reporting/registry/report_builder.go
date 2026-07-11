package registry

import (
	"context"

	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

// ReportBuilder assembles immutable report content from Interpretation-owned
// input. It never receives an Assessment aggregate or an Evaluation outcome
// application object.
type ReportBuilder interface {
	ReportType() domainReport.ReportType
	TemplateVersion() policy.TemplateVersion
	BuilderIdentity() string
	ContentSchemaVersion() string
	Build(ctx context.Context, input interpinput.InterpretationInput) (*report.Draft, error)
}
