package run

import "context"

// Repository persists the append-only attempt history for a Generation.
// Implementations must enforce (generation_id, attempt) uniqueness.
type Repository interface {
	Create(ctx context.Context, run *InterpretationRun) error
	FindByID(ctx context.Context, id ID) (*InterpretationRun, error)
	FindLatestByGenerationID(ctx context.Context, generationID ID) (*InterpretationRun, error)
	Save(ctx context.Context, run *InterpretationRun) error
}
