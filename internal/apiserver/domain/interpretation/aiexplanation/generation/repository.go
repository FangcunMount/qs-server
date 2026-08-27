package generation

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Repository persists AIExplanationGeneration. Implementations must enforce
// Key uniqueness and compare Version on updates.
type Repository interface {
	Create(context.Context, *AIExplanationGeneration) error
	FindByID(context.Context, meta.ID) (*AIExplanationGeneration, error)
	FindByKey(context.Context, Key) (*AIExplanationGeneration, error)
	Save(context.Context, *AIExplanationGeneration, uint64) error
}
