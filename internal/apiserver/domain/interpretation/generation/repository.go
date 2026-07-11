package generation

import "context"

// Repository persists the Generation aggregate. Implementations must enforce
// Key uniqueness and compare Version on updates.
type Repository interface {
	Create(ctx context.Context, generation *ReportGeneration) error
	FindByID(ctx context.Context, id ID) (*ReportGeneration, error)
	FindByKey(ctx context.Context, key Key) (*ReportGeneration, error)
	Save(ctx context.Context, generation *ReportGeneration, expectedVersion uint64) error
}
