package questionnaire

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
)

// Queryer 问卷查询器
type Queryer struct {
	qRepoMySQL port.QuestionnaireRepositoryMySQL
	qRepoMongo port.QuestionnaireRepositoryMongo
}

// NewQueryer 创建问卷查询器
func NewQueryer(
	qRepoMySQL port.QuestionnaireRepositoryMySQL,
	qRepoMongo port.QuestionnaireRepositoryMongo,
) *Queryer {
	return &Queryer{qRepoMySQL: qRepoMySQL, qRepoMongo: qRepoMongo}
}

// GetQuestionnaire 根据ID获取问卷
func (q *Queryer) GetQuestionnaire(ctx context.Context, id uint64) (*questionnaire.Questionnaire, error) {
	return q.qRepoMySQL.FindByID(ctx, id)
}

// GetQuestionnaireByCode 根据编码获取问卷
func (q *Queryer) GetQuestionnaireByCode(ctx context.Context, code string) (*questionnaire.Questionnaire, error) {
	return q.qRepoMySQL.FindByCode(ctx, code)
}

// ListQuestionnaires 获取问卷列表
func (q *Queryer) ListQuestionnaires(ctx context.Context, page, pageSize int, conditions map[string]string) ([]*questionnaire.Questionnaire, int64, error) {
	questionnaires, err := q.qRepoMySQL.FindList(ctx, page, pageSize, conditions)
	if err != nil {
		return nil, 0, err
	}

	total, err := q.qRepoMySQL.CountWithConditions(ctx, conditions)
	if err != nil {
		return nil, 0, err
	}
	return questionnaires, total, nil
}
