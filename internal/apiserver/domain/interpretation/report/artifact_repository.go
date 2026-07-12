package report

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ReportRepository stores successful immutable reports only.
// Implementations must enforce one report per Generation.
type ReportRepository interface {
	Insert(ctx context.Context, report *InterpretReport) error
	FindByID(ctx context.Context, id meta.ID) (*InterpretReport, error)
	FindByGenerationID(ctx context.Context, generationID meta.ID) (*InterpretReport, error)
	ListByAssessmentID(ctx context.Context, assessmentID meta.ID) ([]*InterpretReport, error)
}
