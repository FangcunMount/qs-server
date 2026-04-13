package assessmententry

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
)

// Repository 测评入口仓储接口。
type Repository interface {
	Save(ctx context.Context, item *AssessmentEntry) error
	Update(ctx context.Context, item *AssessmentEntry) error
	FindByID(ctx context.Context, id ID) (*AssessmentEntry, error)
	FindByToken(ctx context.Context, token string) (*AssessmentEntry, error)
	ListByClinician(
		ctx context.Context,
		orgID int64,
		clinicianID clinician.ID,
		offset, limit int,
	) ([]*AssessmentEntry, error)
	CountByClinician(ctx context.Context, orgID int64, clinicianID clinician.ID) (int64, error)
}
