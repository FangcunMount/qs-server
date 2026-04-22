package actor

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
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

func relationTypesToStrings(types []domain.RelationType) []string {
	if len(types) == 0 {
		return nil
	}
	values := make([]string, 0, len(types))
	for _, item := range types {
		values = append(values, string(item))
	}
	return values
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

func (r *relationRepository) Update(ctx context.Context, item *domain.ClinicianTesteeRelation) error {
	po := r.mapper.ToPO(item)

	return r.UpdateAndSync(ctx, po, func(saved *ClinicianRelationPO) {
		r.mapper.SyncID(saved, item)
	})
}

func (r *relationRepository) FindByID(ctx context.Context, id domain.ID) (*domain.ClinicianTesteeRelation, error) {
	po, err := r.BaseRepository.FindByID(ctx, id.Uint64())
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
	return r.FindActiveByTypes(ctx, orgID, clinicianID, testeeID, []domain.RelationType{relationType})
}

func (r *relationRepository) FindActivePrimaryByTestee(
	ctx context.Context,
	orgID int64,
	testeeID testee.ID,
) (*domain.ClinicianTesteeRelation, error) {
	var po ClinicianRelationPO
	err := r.WithContext(ctx).
		Where(
			"org_id = ? AND testee_id = ? AND relation_type = ? AND is_active = ? AND deleted_at IS NULL",
			orgID,
			testeeID,
			string(domain.RelationTypePrimary),
			true,
		).
		First(&po).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "primary relation not found")
		}
		return nil, err
	}
	return r.mapper.ToDomain(&po), nil
}

func (r *relationRepository) FindActiveByTypes(
	ctx context.Context,
	orgID int64,
	clinicianID clinician.ID,
	testeeID testee.ID,
	relationTypes []domain.RelationType,
) (*domain.ClinicianTesteeRelation, error) {
	var po ClinicianRelationPO
	query := r.WithContext(ctx).
		Where(
			"org_id = ? AND clinician_id = ? AND testee_id = ? AND is_active = ? AND deleted_at IS NULL",
			orgID,
			clinicianID,
			testeeID,
			true,
		)
	if len(relationTypes) > 0 {
		query = query.Where("relation_type IN ?", relationTypesToStrings(relationTypes))
	}
	err := query.First(&po).Error
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
	relationTypes []domain.RelationType,
	offset, limit int,
) ([]*domain.ClinicianTesteeRelation, error) {
	var pos []*ClinicianRelationPO
	query := r.WithContext(ctx).
		Where("org_id = ? AND clinician_id = ? AND is_active = ? AND deleted_at IS NULL", orgID, clinicianID, true)
	if len(relationTypes) > 0 {
		query = query.Where("relation_type IN ?", relationTypesToStrings(relationTypes))
	}
	err := query.
		Order("bound_at DESC, id DESC").
		Offset(offset).
		Limit(limit).
		Find(&pos).Error
	if err != nil {
		return nil, err
	}
	return r.mapper.ToDomains(pos), nil
}

func (r *relationRepository) ListHistoryByClinician(
	ctx context.Context,
	orgID int64,
	clinicianID clinician.ID,
) ([]*domain.ClinicianTesteeRelation, error) {
	var pos []*ClinicianRelationPO
	err := r.WithContext(ctx).
		Where("org_id = ? AND clinician_id = ? AND deleted_at IS NULL", orgID, clinicianID).
		Order("bound_at DESC, id DESC").
		Find(&pos).Error
	if err != nil {
		return nil, err
	}
	return r.mapper.ToDomains(pos), nil
}

func (r *relationRepository) CountActiveByClinician(
	ctx context.Context,
	orgID int64,
	clinicianID clinician.ID,
	relationTypes []domain.RelationType,
) (int64, error) {
	var count int64
	query := r.WithContext(ctx).
		Model(&ClinicianRelationPO{}).
		Where("org_id = ? AND clinician_id = ? AND is_active = ? AND deleted_at IS NULL", orgID, clinicianID, true)
	if len(relationTypes) > 0 {
		query = query.Where("relation_type IN ?", relationTypesToStrings(relationTypes))
	}
	err := query.Count(&count).Error
	return count, err
}

func (r *relationRepository) ListActiveByTestee(
	ctx context.Context,
	orgID int64,
	testeeID testee.ID,
	relationTypes []domain.RelationType,
) ([]*domain.ClinicianTesteeRelation, error) {
	var pos []*ClinicianRelationPO
	query := r.WithContext(ctx).
		Where("org_id = ? AND testee_id = ? AND is_active = ? AND deleted_at IS NULL", orgID, testeeID, true)
	if len(relationTypes) > 0 {
		query = query.Where("relation_type IN ?", relationTypesToStrings(relationTypes))
	}
	err := query.Order("bound_at DESC, id DESC").Find(&pos).Error
	if err != nil {
		return nil, err
	}
	return r.mapper.ToDomains(pos), nil
}

func (r *relationRepository) ListHistoryByTestee(
	ctx context.Context,
	orgID int64,
	testeeID testee.ID,
) ([]*domain.ClinicianTesteeRelation, error) {
	var pos []*ClinicianRelationPO
	err := r.WithContext(ctx).
		Where("org_id = ? AND testee_id = ? AND deleted_at IS NULL", orgID, testeeID).
		Order("bound_at DESC, id DESC").
		Find(&pos).Error
	if err != nil {
		return nil, err
	}
	return r.mapper.ToDomains(pos), nil
}

func (r *relationRepository) HasActiveRelationForTestee(
	ctx context.Context,
	orgID int64,
	clinicianID clinician.ID,
	testeeID testee.ID,
	relationTypes []domain.RelationType,
) (bool, error) {
	var count int64
	query := r.WithContext(ctx).
		Model(&ClinicianRelationPO{}).
		Where(
			"org_id = ? AND clinician_id = ? AND testee_id = ? AND is_active = ? AND deleted_at IS NULL",
			orgID,
			clinicianID,
			testeeID,
			true,
		)
	if len(relationTypes) > 0 {
		query = query.Where("relation_type IN ?", relationTypesToStrings(relationTypes))
	}
	err := query.Count(&count).Error
	return count > 0, err
}

func (r *relationRepository) ListActiveTesteeIDsByClinician(
	ctx context.Context,
	orgID int64,
	clinicianID clinician.ID,
	relationTypes []domain.RelationType,
) ([]testee.ID, error) {
	var rawIDs []uint64
	query := r.WithContext(ctx).
		Model(&ClinicianRelationPO{}).
		Where("org_id = ? AND clinician_id = ? AND is_active = ? AND deleted_at IS NULL", orgID, clinicianID, true)
	if len(relationTypes) > 0 {
		query = query.Where("relation_type IN ?", relationTypesToStrings(relationTypes))
	}
	err := query.Order("bound_at DESC, id DESC").Pluck("testee_id", &rawIDs).Error
	if err != nil {
		return nil, err
	}

	ids := make([]testee.ID, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		convertedID, err := safeconv.Uint64ToMetaID(rawID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, testee.ID(convertedID))
	}
	return ids, nil
}
