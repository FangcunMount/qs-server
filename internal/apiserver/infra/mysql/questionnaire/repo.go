package questionnaire

import (
	"context"

	"gorm.io/gorm"

	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/questionnaire"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/questionnaire/port"
	"github.com/fangcun-mount/qs-server/internal/apiserver/infra/mysql"
)

// Repository 存储库实现
type Repository struct {
	mysql.BaseRepository[*QuestionnairePO]
	mapper *QuestionnaireMapper
}

// NewRepository 创建问卷存储库
func NewRepository(db *gorm.DB) port.QuestionnaireRepositoryMySQL {
	return &Repository{
		BaseRepository: mysql.NewBaseRepository[*QuestionnairePO](db),
		mapper:         NewQuestionnaireMapper(),
	}
}

// Create 创建问卷
func (r *Repository) Create(ctx context.Context, qDomain *questionnaire.Questionnaire) error {
	po := r.mapper.ToPO(qDomain)
	return r.BaseRepository.CreateAndSync(ctx, po, func(qPO *QuestionnairePO) {
		qDomain.SetID(questionnaire.NewQuestionnaireID(qPO.ID))
	})
}

// Update 更新问卷
func (r *Repository) Update(ctx context.Context, qDomain *questionnaire.Questionnaire) error {
	return r.BaseRepository.UpdateAndSync(ctx, r.mapper.ToPO(qDomain), func(qPO *QuestionnairePO) {
		qDomain.SetID(questionnaire.NewQuestionnaireID(qPO.ID))
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

// FindList 查询问卷列表
func (r *Repository) FindList(ctx context.Context, page, pageSize int, conditions map[string]string) ([]*questionnaire.Questionnaire, error) {
	pos, err := r.BaseRepository.FindList(ctx, &QuestionnairePO{}, conditions, page, pageSize)
	if err != nil {
		return nil, err
	}
	return r.mapper.ToBOList(pos), nil
}

// CountWithConditions 根据条件统计记录数
func (r *Repository) CountWithConditions(ctx context.Context, conditions map[string]string) (int64, error) {
	return r.BaseRepository.CountWithConditions(ctx, &QuestionnairePO{}, conditions)
}
