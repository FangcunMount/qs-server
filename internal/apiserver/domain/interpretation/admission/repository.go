package admission

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type QueryFilter struct {
	OrgID        int64
	Kind         *Kind
	Decision     string
	AssessmentID *meta.ID
	OutcomeID    *meta.ID
	OccurredFrom *time.Time
	OccurredTo   *time.Time
}

type QueryPage struct {
	Items      []*Failure
	NextCursor string
}

// Repository persists admission failures without creating Generation/Run.
type Repository interface {
	// UpsertByFingerprint stores evidence once per fingerprint.
	UpsertByFingerprint(ctx context.Context, failure *Failure) (created bool, err error)
	FindByFingerprint(ctx context.Context, fingerprint string) (*Failure, error)
	FindByOutcomeID(ctx context.Context, outcomeID meta.ID, limit int) ([]*Failure, error)
}

// QueryRepository is the opt-in operations projection. Keeping it separate
// prevents write-path repositories and tests from acquiring an unnecessary
// diagnostics dependency.
type QueryRepository interface {
	Repository
	ListFailures(ctx context.Context, filter QueryFilter, cursor string, limit int) (QueryPage, error)
}
