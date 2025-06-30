package questionnaire

import (
	"context"

	"gorm.io/gorm"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/driven/mysql"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
)

// Repository 存储库现
type Repository struct {
	mysql.BaseRepository[*QuestionnaireEntity]
	mapper *QuestionnaireMapper
}

// NewRepository 创建问卷存储库
func NewRepository(db *gorm.DB) port.QuestionnaireRepository {
	return &Repository{
		BaseRepository: mysql.NewBaseRepository[*QuestionnaireEntity](db),
		mapper:         NewQuestionnaireMapper(),
	}
}

// Save 保存问卷
func (r *Repository) Save(ctx context.Context, qDomain *questionnaire.Questionnaire) error {
	entity := r.mapper.ToEntity(qDomain)
	return r.UpdateAndSync(ctx, entity, func(qEntity *QuestionnaireEntity) {
		qDomain.SetID(questionnaire.NewQuestionnaireID(qEntity.ID))
		qDomain.SetCreatedAt(qEntity.CreatedAt)
		qDomain.SetUpdatedAt(qEntity.UpdatedAt)
		qDomain.SetCreatedBy(qEntity.CreatedBy)
		qDomain.SetUpdatedBy(qEntity.UpdatedBy)
		qDomain.SetDeletedBy(qEntity.DeletedBy)
		qDomain.SetDeletedAt(qEntity.DeletedAt)
	})
}

// Update 更新问卷
func (r *Repository) Update(ctx context.Context, qDomain *questionnaire.Questionnaire) error {
	return r.BaseRepository.UpdateAndSync(ctx, r.mapper.ToEntity(qDomain), func(qEntity *QuestionnaireEntity) {
		qDomain.SetID(questionnaire.NewQuestionnaireID(qEntity.ID))
		qDomain.SetCreatedAt(qEntity.CreatedAt)
		qDomain.SetUpdatedAt(qEntity.UpdatedAt)
		qDomain.SetCreatedBy(qEntity.CreatedBy)
		qDomain.SetUpdatedBy(qEntity.UpdatedBy)
		qDomain.SetDeletedBy(qEntity.DeletedBy)
		qDomain.SetDeletedAt(qEntity.DeletedAt)
	})
}

// Remove 删除问卷
func (r *Repository) Remove(ctx context.Context, id uint64) error {
	return r.BaseRepository.DeleteByID(ctx, id)
}

// FindByID 根据ID查询问卷
func (r *Repository) FindByID(ctx context.Context, id uint64) (*questionnaire.Questionnaire, error) {
	var entity QuestionnaireEntity
	err := r.BaseRepository.FindByField(ctx, &entity, "id", id)
	if err != nil {
		return nil, err
	}
	return r.mapper.ToDomain(&entity), nil
}

// FindByCode 根据编码查询问卷
func (r *Repository) FindByCode(ctx context.Context, code string) (*questionnaire.Questionnaire, error) {
	var entity QuestionnaireEntity
	err := r.BaseRepository.FindByField(ctx, &entity, "code", code)
	if err != nil {
		return nil, err
	}
	return r.mapper.ToDomain(&entity), nil
}
