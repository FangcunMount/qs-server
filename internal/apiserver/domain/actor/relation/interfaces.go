package relation

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

// Repository 关系仓储接口。
type Repository interface {
	Save(ctx context.Context, item *ClinicianTesteeRelation) error
	FindByID(ctx context.Context, id ID) (*ClinicianTesteeRelation, error)
	FindActive(
		ctx context.Context,
		orgID int64,
		clinicianID clinician.ID,
		testeeID testee.ID,
		relationType RelationType,
	) (*ClinicianTesteeRelation, error)
	ListActiveByClinician(
		ctx context.Context,
		orgID int64,
		clinicianID clinician.ID,
		offset, limit int,
	) ([]*ClinicianTesteeRelation, error)
	CountActiveByClinician(ctx context.Context, orgID int64, clinicianID clinician.ID) (int64, error)
	HasActiveRelationForTestee(ctx context.Context, orgID int64, clinicianID clinician.ID, testeeID testee.ID) (bool, error)
	ListActiveTesteeIDsByClinician(ctx context.Context, orgID int64, clinicianID clinician.ID) ([]testee.ID, error)
}
