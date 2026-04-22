package actor

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/assessmententry"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

type assessmentEntryRepository struct {
	mysql.BaseRepository[*AssessmentEntryPO]
	mapper *AssessmentEntryMapper
}

// NewAssessmentEntryRepository 创建测评入口仓储。
func NewAssessmentEntryRepository(db *gorm.DB) domain.Repository {
	repo := &assessmentEntryRepository{
		BaseRepository: mysql.NewBaseRepository[*AssessmentEntryPO](db),
		mapper:         NewAssessmentEntryMapper(),
	}
	repo.SetErrorTranslator(translateError)
	return repo
}

func (r *assessmentEntryRepository) Save(ctx context.Context, item *domain.AssessmentEntry) error {
	po := r.mapper.ToPO(item)
	if err := po.BeforeCreate(nil); err != nil {
		return err
	}

	return r.CreateAndSync(ctx, po, func(saved *AssessmentEntryPO) {
		r.mapper.SyncID(saved, item)
	})
}

func (r *assessmentEntryRepository) Update(ctx context.Context, item *domain.AssessmentEntry) error {
	po := r.mapper.ToPO(item)

	return r.UpdateAndSync(ctx, po, func(saved *AssessmentEntryPO) {
		r.mapper.SyncID(saved, item)
	})
}

func (r *assessmentEntryRepository) FindByID(ctx context.Context, id domain.ID) (*domain.AssessmentEntry, error) {
	po, err := r.BaseRepository.FindByID(ctx, id.Uint64())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "assessment entry not found")
		}
		return nil, err
	}
	return r.mapper.ToDomain(po), nil
}

func (r *assessmentEntryRepository) FindByToken(ctx context.Context, token string) (*domain.AssessmentEntry, error) {
	var po AssessmentEntryPO
	err := r.WithContext(ctx).
		Where("token = ? AND deleted_at IS NULL", token).
		First(&po).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "assessment entry not found")
		}
		return nil, err
	}
	return r.mapper.ToDomain(&po), nil
}

func (r *assessmentEntryRepository) ListByClinician(
	ctx context.Context,
	orgID int64,
	clinicianID clinician.ID,
	offset, limit int,
) ([]*domain.AssessmentEntry, error) {
	var pos []*AssessmentEntryPO
	err := r.WithContext(ctx).
		Where("org_id = ? AND clinician_id = ? AND deleted_at IS NULL", orgID, clinicianID).
		Order("id DESC").
		Offset(offset).
		Limit(limit).
		Find(&pos).Error
	if err != nil {
		return nil, err
	}
	return r.mapper.ToDomains(pos), nil
}

func (r *assessmentEntryRepository) CountByClinician(ctx context.Context, orgID int64, clinicianID clinician.ID) (int64, error) {
	var count int64
	err := r.WithContext(ctx).
		Model(&AssessmentEntryPO{}).
		Where("org_id = ? AND clinician_id = ? AND deleted_at IS NULL", orgID, clinicianID).
		Count(&count).Error
	return count, err
}
