package outcome

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type Repository interface {
	Save(ctx context.Context, record *Record) error
	FindByID(ctx context.Context, id ID) (*Record, error)
	FindByAssessmentID(ctx context.Context, assessmentID meta.ID) (*Record, error)
}
