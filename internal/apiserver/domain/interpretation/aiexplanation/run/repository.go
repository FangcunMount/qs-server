package run

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Repository persists append-only attempts and their mutable execution state.
// Implementations must enforce (generation_id, attempt) uniqueness.
type Repository interface {
	Create(context.Context, *AIExplanationRun) error
	FindByID(context.Context, meta.ID) (*AIExplanationRun, error)
	FindLatestByGenerationID(context.Context, meta.ID) (*AIExplanationRun, error)
	Save(context.Context, *AIExplanationRun) error
}

// RetryAuthorizer atomically attaches an immutable authorization to the exact
// latest failed attempt. The bool reports whether this call created it; an
// idempotent replay of the same request returns the existing authorization.
type RetryAuthorizer interface {
	AuthorizeRetry(context.Context, meta.ID, RetryAuthorization) (*AIExplanationRun, bool, error)
}

// RecoveryWakeupScheduler atomically records one durable wake-up for the
// exact expired lease. The bool is true only for the transaction that created
// the wake-up; an exact replay returns the existing record.
type RecoveryWakeupScheduler interface {
	ScheduleRecoveryWakeup(context.Context, meta.ID, RecoveryWakeup) (*AIExplanationRun, bool, error)
}

// LeaseReclaimer atomically transfers an expired running attempt to one
// worker. Post-dispatch reclaim is allowed only when the frozen Provider route
// guarantees idempotent redispatch for the stable InvocationID.
type LeaseReclaimer interface {
	ReclaimExpiredLease(context.Context, meta.ID, time.Time, string, time.Time, bool) (*AIExplanationRun, bool, error)
}

type ExpiredLease struct {
	RunID           meta.ID
	GenerationID    meta.ID
	LeaseExpiredAt  time.Time
	InvocationPhase InvocationPhase
}

type ExpiredLeaseReader interface {
	ListExpiredLeases(context.Context, time.Time, int) ([]ExpiredLease, error)
}
