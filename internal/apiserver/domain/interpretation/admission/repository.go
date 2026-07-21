package admission

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Repository persists admission failures without creating Generation/Run.
type Repository interface {
	// UpsertByFingerprint stores evidence once per fingerprint.
	UpsertByFingerprint(ctx context.Context, failure *Failure) (created bool, err error)
	FindByFingerprint(ctx context.Context, fingerprint string) (*Failure, error)
	FindByOutcomeID(ctx context.Context, outcomeID meta.ID, limit int) ([]*Failure, error)
}
