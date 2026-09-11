package assessmententry

import "context"

// Repository 测评入口仓储接口。
type Repository interface {
	Save(ctx context.Context, item *AssessmentEntry) error
	Update(ctx context.Context, item *AssessmentEntry) error
	FindByID(ctx context.Context, id ID) (*AssessmentEntry, error)
	FindByToken(ctx context.Context, token string) (*AssessmentEntry, error)
	// LockByID performs a current read after the owning clinician is locked.
	LockByID(ctx context.Context, id ID) (*AssessmentEntry, error)
}
