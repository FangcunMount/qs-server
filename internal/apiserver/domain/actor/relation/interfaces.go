package relation

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

// Repository 关系仓储接口。
type Repository interface {
	Save(ctx context.Context, item *ClinicianTesteeRelation) error
	Update(ctx context.Context, item *ClinicianTesteeRelation) error
	FindByID(ctx context.Context, id ID) (*ClinicianTesteeRelation, error)
	FindActive(
		ctx context.Context,
		orgID int64,
		clinicianID clinician.ID,
		testeeID testee.ID,
		relationType RelationType,
	) (*ClinicianTesteeRelation, error)
	FindActiveByTypes(
		ctx context.Context,
		orgID int64,
		clinicianID clinician.ID,
		testeeID testee.ID,
		relationTypes []RelationType,
	) (*ClinicianTesteeRelation, error)
	ListActiveByClinician(
		ctx context.Context,
		orgID int64,
		clinicianID clinician.ID,
		relationTypes []RelationType,
		offset, limit int,
	) ([]*ClinicianTesteeRelation, error)
	CountActiveByClinician(
		ctx context.Context,
		orgID int64,
		clinicianID clinician.ID,
		relationTypes []RelationType,
	) (int64, error)
	ListActiveByTestee(
		ctx context.Context,
		orgID int64,
		testeeID testee.ID,
		relationTypes []RelationType,
	) ([]*ClinicianTesteeRelation, error)
	ListHistoryByTestee(ctx context.Context, orgID int64, testeeID testee.ID) ([]*ClinicianTesteeRelation, error)
	HasActiveRelationForTestee(
		ctx context.Context,
		orgID int64,
		clinicianID clinician.ID,
		testeeID testee.ID,
		relationTypes []RelationType,
	) (bool, error)
	ListActiveTesteeIDsByClinician(
		ctx context.Context,
		orgID int64,
		clinicianID clinician.ID,
		relationTypes []RelationType,
	) ([]testee.ID, error)
}
