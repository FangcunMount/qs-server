package evaluation

import (
	"context"
	"time"
)

import "github.com/FangcunMount/qs-server/internal/pkg/meta"

// Repository stores the immutable evidence accumulated by a Prompt evaluation
// run. Save must use aggregate Version as an optimistic concurrency token.
type Repository interface {
	Create(context.Context, *PromptEvaluationRun) error
	Save(context.Context, *PromptEvaluationRun, int64) error
	FindByID(context.Context, meta.ID) (*PromptEvaluationRun, error)
}

// RecheckRepository stores diagnostic single-attempt reruns separately from
// immutable release-gate evidence.
type RecheckRepository interface {
	CreateRecheck(context.Context, *PromptEvaluationRecheck) error
	SaveRecheck(context.Context, *PromptEvaluationRecheck, int64) error
	FindRecheckByID(context.Context, meta.ID) (*PromptEvaluationRecheck, error)
	ListRechecksBySource(context.Context, meta.ID, string, int, int) ([]*PromptEvaluationRecheck, error)
}

// ExpiredPreparation is the minimum safe identity needed to reawaken a
// prepared execution. InvocationID plus LeaseExpiresAt prevent a stale scanner
// result from recovering a newer claim that intentionally reuses the stable
// Provider invocation identity for the same run/case/attempt.
type ExpiredPreparation struct {
	RunID          meta.ID
	InvocationID   string
	LeaseExpiresAt time.Time
}

// ExpiredPreparationReader must return only collecting runs whose current
// checkpoint is prepared and expired at the supplied time. The aggregate and
// repository CAS still recheck that boundary before a wake-up is committed.
type ExpiredPreparationReader interface {
	ListExpiredPreparations(context.Context, time.Time, int) ([]ExpiredPreparation, error)
}
