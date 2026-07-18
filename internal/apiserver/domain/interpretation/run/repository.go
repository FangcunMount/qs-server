package run

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

// Repository persists the append-only attempt history for a Generation.
// Implementations must enforce (generation_id, attempt) uniqueness.
type Repository interface {
	Create(ctx context.Context, run *InterpretationRun) error
	FindByID(ctx context.Context, id ID) (*InterpretationRun, error)
	FindLatestByGenerationID(ctx context.Context, generationID ID) (*InterpretationRun, error)
	Save(ctx context.Context, run *InterpretationRun) error
}

type RetryAuthorizationRequest struct {
	GenerationID    ID
	ExpectedAttempt int
	Origin          retrygovernance.AttemptOrigin
	RequestID       string
	EventID         string
	AuthorizedAt    time.Time
}

type RetryAuthorizer interface {
	AuthorizeRetry(context.Context, RetryAuthorizationRequest) (*InterpretationRun, error)
}

type LeaseReclaimer interface {
	ReclaimExpiredLease(context.Context, ID, time.Time, string, time.Time) (*InterpretationRun, bool, error)
}

type ExpiredLease struct {
	RunID        ID
	GenerationID ID
}

type ExpiredLeaseReader interface {
	ListExpiredLeases(context.Context, time.Time, int) ([]ExpiredLease, error)
}

type HistoryReader interface {
	ListByGenerationID(context.Context, ID, int) ([]*InterpretationRun, error)
}
