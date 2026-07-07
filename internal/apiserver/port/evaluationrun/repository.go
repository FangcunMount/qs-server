package evaluationrun

import (
	"context"

	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
)

// RetryableFailedRun is a failed run scoped to an organization for operating queries.
type RetryableFailedRun struct {
	Run   evalrun.EvaluationRun
	OrgID int64
}

// ListRetryableFailedParams filters retryable failed runs for one organization.
type ListRetryableFailedParams struct {
	OrgID  int64
	Limit  int
	Cursor uint64
}

// ListRetryableFailedResult is a cursor page of retryable failed runs.
type ListRetryableFailedResult struct {
	Items      []RetryableFailedRun
	NextCursor uint64
}

// Repository persists evaluation run attempts.
type Repository interface {
	Save(ctx context.Context, run evalrun.EvaluationRun) error
	FindLatestByAssessmentID(ctx context.Context, assessmentID uint64) (*evalrun.EvaluationRun, error)
	ListByAssessmentID(ctx context.Context, assessmentID uint64, limit int) ([]evalrun.EvaluationRun, error)
	ListRetryableFailed(ctx context.Context, params ListRetryableFailedParams) (*ListRetryableFailedResult, error)
}
