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
	qBOFromMySQL, err := q.qRepoMySQL.FindByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	qBOFromMongo, err := q.qRepoMongo.FindByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	// 构建选项列表
	opts := []questionnaire.QuestionnaireOption{
		questionnaire.WithID(qBOFromMySQL.GetID()),
		questionnaire.WithDescription(qBOFromMySQL.GetDescription()),
		questionnaire.WithImgUrl(qBOFromMySQL.GetImgUrl()),
		questionnaire.WithVersion(qBOFromMySQL.GetVersion()),
		questionnaire.WithStatus(qBOFromMySQL.GetStatus()),
	}

	// 如果 MongoDB 中有问卷数据且有问题列表，则添加问题
	if qBOFromMongo != nil && qBOFromMongo.GetQuestions() != nil {
		opts = append(opts, questionnaire.WithQuestions(qBOFromMongo.GetQuestions()))
	}

	// 创建问卷对象
	qBo := questionnaire.NewQuestionnaire(
		questionnaire.NewQuestionnaireCode(code),
		qBOFromMySQL.GetTitle(),
		opts...,
	)

	return qBo, nil
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
