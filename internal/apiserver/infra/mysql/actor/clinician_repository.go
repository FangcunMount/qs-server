package actor

import (
	"context"
	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type clinicianRepository struct {
	mysql.BaseRepository[*ClinicianPO]
	mapper *ClinicianMapper
}

// NewClinicianRepository 创建从业者仓储。
func NewClinicianRepository(db *gorm.DB, opts ...mysql.BaseRepositoryOptions) domain.Repository {
	repo := &clinicianRepository{
		BaseRepository: mysql.NewBaseRepository[*ClinicianPO](db, opts...),
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

	res := r.WithContext(ctx).Model(&ClinicianPO{}).Where("id=? AND org_id=? AND version=? AND deleted_at IS NULL", item.ID(), item.OrgID(), item.Version()).Updates(map[string]any{
		"name": po.Name, "department": po.Department, "title": po.Title, "clinician_type": po.ClinicianType, "employee_code": po.EmployeeCode, "is_active": po.IsActive, "version": gorm.Expr("version + 1"), "updated_at": time.Now().UTC(), "updated_by": middleware.GetUserIDFromContext(ctx),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return errors.WithCode(code.ErrConflict, "clinician changed; refresh and retry")
	}
	item.RestoreStore(item.StoreID(), item.Version()+1)
	return nil
}

func (r *clinicianRepository) FindByID(ctx context.Context, id domain.ID) (*domain.Clinician, error) {
	query := r.WithContext(ctx)
	if _, ok := mysql.TxFromContext(ctx); ok && query.Name() == "mysql" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var po ClinicianPO
	err := query.Where("id = ?", id.Uint64()).First(&po).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "clinician not found")
		}
		return nil, err
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
	return r.DeleteByID(ctx, id.Uint64())
}
