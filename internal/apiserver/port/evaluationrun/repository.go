package evaluationrun

import (
	"context"

	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
)

// Repository persists evaluation run attempts.
type Repository interface {
	Save(ctx context.Context, run evalrun.EvaluationRun) error
	FindLatestByAssessmentID(ctx context.Context, assessmentID uint64) (*evalrun.EvaluationRun, error)
}
