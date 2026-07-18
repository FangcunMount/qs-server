package evaluationrun

import (
	"context"
	"errors"
	"time"

	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

var ErrClaimLost = errors.New("evaluation run claim lost")

type ClaimRequest struct {
	AssessmentID    uint64
	Token           string
	ClaimedAt       time.Time
	LeaseUntil      time.Time
	TraceID         string
	RetryEventID    string
	ExpectedAttempt int
	Origin          retrygovernance.AttemptOrigin
	ActionRequestID string
}

type ClaimResult struct {
	Run     evalrun.EvaluationRun
	Claimed bool
}

type RetryAuthorizationRequest struct {
	AssessmentID    uint64
	ExpectedAttempt int
	Origin          retrygovernance.AttemptOrigin
	RequestID       string
	EventID         string
	AuthorizedAt    time.Time
}

type RetryAuthorizer interface {
	AuthorizeRetry(context.Context, RetryAuthorizationRequest) (*evalrun.EvaluationRun, error)
}

type ExpiredLease struct {
	AssessmentID uint64
	RunID        evalrun.ID
}

type ExpiredLeaseReader interface {
	ListExpiredLeases(context.Context, time.Time, int) ([]ExpiredLease, error)
}

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
	Claim(ctx context.Context, request ClaimRequest) (ClaimResult, error)
	SaveClaimed(ctx context.Context, run evalrun.EvaluationRun) error
	FindLatestByAssessmentID(ctx context.Context, assessmentID uint64) (*evalrun.EvaluationRun, error)
	ListByAssessmentID(ctx context.Context, assessmentID uint64, limit int) ([]evalrun.EvaluationRun, error)
	ListRetryableFailed(ctx context.Context, params ListRetryableFailedParams) (*ListRetryableFailedResult, error)
}
