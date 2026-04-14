package actor

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

type clinicianRepository struct {
	mysql.BaseRepository[*ClinicianPO]
	mapper *ClinicianMapper
}

// NewClinicianRepository 创建从业者仓储。
func NewClinicianRepository(db *gorm.DB) domain.Repository {
	repo := &clinicianRepository{
		BaseRepository: mysql.NewBaseRepository[*ClinicianPO](db),
		mapper:         NewClinicianMapper(),
	}
	repo.SetErrorTranslator(translateError)
	return repo
}

func (r *clinicianRepository) Save(ctx context.Context, item *domain.Clinician) error {
	po := r.mapper.ToPO(item)
	if err := po.BeforeCreate(nil); err != nil {
		return err
	}

	return r.CreateAndSync(ctx, po, func(saved *ClinicianPO) {
		r.mapper.SyncID(saved, item)
	})
}

func (r *clinicianRepository) Update(ctx context.Context, item *domain.Clinician) error {
	po := r.mapper.ToPO(item)

	return r.UpdateAndSync(ctx, po, func(saved *ClinicianPO) {
		r.mapper.SyncID(saved, item)
	})
}

func (r *clinicianRepository) FindByID(ctx context.Context, id domain.ID) (*domain.Clinician, error) {
	po, err := r.BaseRepository.FindByID(ctx, uint64(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "clinician not found")
		}
		return nil, err
	}
	return r.mapper.ToDomain(po), nil
}

func (r *clinicianRepository) FindByOperator(ctx context.Context, orgID int64, operatorID uint64) (*domain.Clinician, error) {
	var po ClinicianPO
	tx := r.WithContext(ctx).
		Where("org_id = ? AND operator_id = ? AND deleted_at IS NULL", orgID, operatorID).
		Limit(1).
		Find(&po)
	if tx.Error != nil {
		return nil, tx.Error
	}
	if tx.RowsAffected == 0 {
		return nil, errors.WithCode(code.ErrUserNotFound, "clinician not found")
	}
	return r.mapper.ToDomain(&po), nil
}

func (r *clinicianRepository) ListByOrg(ctx context.Context, orgID int64, offset, limit int) ([]*domain.Clinician, error) {
	var pos []*ClinicianPO
	err := r.WithContext(ctx).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Order("id DESC").
		Offset(offset).
		Limit(limit).
		Find(&pos).Error
	if err != nil {
		return nil, err
	}
	return r.mapper.ToDomains(pos), nil
}

func (r *clinicianRepository) Count(ctx context.Context, orgID int64) (int64, error) {
	var count int64
	err := r.WithContext(ctx).
		Model(&ClinicianPO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&count).Error
	return count, err
}

func (r *clinicianRepository) Delete(ctx context.Context, id domain.ID) error {
	return r.DeleteByID(ctx, uint64(id))
}
