package testee

import "context"

// AssessmentAccessCache stores the immutable assessment owner projection.
// Implementations must distinguish a cache miss from owner zero.
type AssessmentAccessCache interface {
	ReadOwner(ctx context.Context, assessmentID uint64, load func(context.Context) (uint64, error)) (ownerTesteeID uint64, err error)
}

// AssessmentDetailCache stores the participant-facing outcome projection.
// Only evaluated values are admitted by the application service.
type AssessmentDetailCache interface {
	ReadDetail(ctx context.Context, assessmentID uint64, load func(context.Context) (*Assessment, error)) (*Assessment, error)
}
