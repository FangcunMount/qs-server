package actor

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

type relationRepository struct {
	mysql.BaseRepository[*ClinicianRelationPO]
	mapper *RelationMapper
}

// NewRelationRepository 创建关系仓储。
func NewRelationRepository(db *gorm.DB) domain.Repository {
	repo := &relationRepository{
		BaseRepository: mysql.NewBaseRepository[*ClinicianRelationPO](db),
		mapper:         NewRelationMapper(),
	}
	repo.SetErrorTranslator(translateError)
	return repo
}

func (r *relationRepository) Save(ctx context.Context, item *domain.ClinicianTesteeRelation) error {
	po := r.mapper.ToPO(item)
	if err := po.BeforeCreate(nil); err != nil {
		return err
	}

	return r.CreateAndSync(ctx, po, func(saved *ClinicianRelationPO) {
		r.mapper.SyncID(saved, item)
	})
}

func (r *relationRepository) FindByID(ctx context.Context, id domain.ID) (*domain.ClinicianTesteeRelation, error) {
	po, err := r.BaseRepository.FindByID(ctx, uint64(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "relation not found")
		}
		return nil, err
	}
	return r.mapper.ToDomain(po), nil
}

func (r *relationRepository) FindActive(
	ctx context.Context,
	orgID int64,
	clinicianID clinician.ID,
	testeeID testee.ID,
	relationType domain.RelationType,
) (*domain.ClinicianTesteeRelation, error) {
	var po ClinicianRelationPO
	err := r.WithContext(ctx).
		Where(
			"org_id = ? AND clinician_id = ? AND testee_id = ? AND relation_type = ? AND is_active = ? AND deleted_at IS NULL",
			orgID,
			clinicianID,
			testeeID,
			string(relationType),
			true,
		).
		First(&po).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "relation not found")
		}
		return nil, err
	}
	return r.mapper.ToDomain(&po), nil
}

func (r *relationRepository) ListActiveByClinician(
	ctx context.Context,
	orgID int64,
	clinicianID clinician.ID,
	offset, limit int,
) ([]*domain.ClinicianTesteeRelation, error) {
	var pos []*ClinicianRelationPO
	err := r.WithContext(ctx).
		Where("org_id = ? AND clinician_id = ? AND is_active = ? AND deleted_at IS NULL", orgID, clinicianID, true).
		Order("bound_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&pos).Error
	if err != nil {
		return nil, err
	}
	return r.mapper.ToDomains(pos), nil
}

func (r *relationRepository) CountActiveByClinician(ctx context.Context, orgID int64, clinicianID clinician.ID) (int64, error) {
	var count int64
	err := r.WithContext(ctx).
		Model(&ClinicianRelationPO{}).
		Where("org_id = ? AND clinician_id = ? AND is_active = ? AND deleted_at IS NULL", orgID, clinicianID, true).
		Count(&count).Error
	return count, err
}

func (r *relationRepository) HasActiveRelationForTestee(
	ctx context.Context,
	orgID int64,
	clinicianID clinician.ID,
	testeeID testee.ID,
) (bool, error) {
	var count int64
	err := r.WithContext(ctx).
		Model(&ClinicianRelationPO{}).
		Where(
			"org_id = ? AND clinician_id = ? AND testee_id = ? AND is_active = ? AND deleted_at IS NULL",
			orgID,
			clinicianID,
			testeeID,
			true,
		).
		Count(&count).Error
	return count > 0, err
}

func (r *relationRepository) ListActiveTesteeIDsByClinician(
	ctx context.Context,
	orgID int64,
	clinicianID clinician.ID,
) ([]testee.ID, error) {
	var rawIDs []uint64
	err := r.WithContext(ctx).
		Model(&ClinicianRelationPO{}).
		Where("org_id = ? AND clinician_id = ? AND is_active = ? AND deleted_at IS NULL", orgID, clinicianID, true).
		Order("bound_at DESC").
		Pluck("testee_id", &rawIDs).Error
	if err != nil {
		return nil, err
	}

	ids := make([]testee.ID, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		ids = append(ids, testee.ID(rawID))
	}
	return ids, nil
}
