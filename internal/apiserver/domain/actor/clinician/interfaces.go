package clinician

import "context"

// Repository 从业者仓储接口。
type Repository interface {
	Save(ctx context.Context, item *Clinician) error
	Update(ctx context.Context, item *Clinician) error
	FindByID(ctx context.Context, id ID) (*Clinician, error)
	FindByOperator(ctx context.Context, orgID int64, operatorID uint64) (*Clinician, error)
	Delete(ctx context.Context, id ID) error
}
