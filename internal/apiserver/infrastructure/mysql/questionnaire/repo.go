package questionnaire

import (
	"context"

	"gorm.io/gorm"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/infrastructure/mysql"
)

// Repository 存储库实现
type Repository struct {
	mysql.BaseRepository[*QuestionnairePO]
	mapper *QuestionnaireMapper
}

// NewRepository 创建问卷存储库
func NewRepository(db *gorm.DB) port.QuestionnaireRepository {
	return &Repository{
		BaseRepository: mysql.NewBaseRepository[*QuestionnairePO](db),
		mapper:         NewQuestionnaireMapper(),
	}
}

// Save 保存问卷
func (r *Repository) Save(ctx context.Context, qDomain *questionnaire.Questionnaire) error {
	po := r.mapper.ToPO(qDomain)
	return r.UpdateAndSync(ctx, po, func(qPO *QuestionnairePO) {
		qDomain.SetID(questionnaire.NewQuestionnaireID(qPO.ID))
		qDomain.SetCreatedAt(qPO.CreatedAt)
		qDomain.SetUpdatedAt(qPO.UpdatedAt)
		qDomain.SetCreatedBy(qPO.CreatedBy)
		qDomain.SetUpdatedBy(qPO.UpdatedBy)
		qDomain.SetDeletedBy(qPO.DeletedBy)
		qDomain.SetDeletedAt(qPO.DeletedAt)
	})
}

// Update 更新问卷
func (r *Repository) Update(ctx context.Context, qDomain *questionnaire.Questionnaire) error {
	return r.BaseRepository.UpdateAndSync(ctx, r.mapper.ToPO(qDomain), func(qPO *QuestionnairePO) {
		qDomain.SetID(questionnaire.NewQuestionnaireID(qPO.ID))
		qDomain.SetCreatedAt(qPO.CreatedAt)
		qDomain.SetUpdatedAt(qPO.UpdatedAt)
		qDomain.SetCreatedBy(qPO.CreatedBy)
		qDomain.SetUpdatedBy(qPO.UpdatedBy)
		qDomain.SetDeletedBy(qPO.DeletedBy)
		qDomain.SetDeletedAt(qPO.DeletedAt)
	})
}

// Remove 删除问卷
func (r *Repository) Remove(ctx context.Context, id uint64) error {
	return r.BaseRepository.DeleteByID(ctx, id)
}

// FindByID 根据ID查询问卷
func (r *Repository) FindByID(ctx context.Context, id uint64) (*questionnaire.Questionnaire, error) {
	var po QuestionnairePO
	err := r.BaseRepository.FindByField(ctx, &po, "id", id)
	if err != nil {
		return nil, err
	}
	return r.mapper.ToBO(&po), nil
}

// FindByCode 根据编码查询问卷
func (r *Repository) FindByCode(ctx context.Context, code string) (*questionnaire.Questionnaire, error) {
	var po QuestionnairePO
	err := r.BaseRepository.FindByField(ctx, &po, "code", code)
	if err != nil {
		return nil, err
	}
	return r.mapper.ToBO(&po), nil
}
