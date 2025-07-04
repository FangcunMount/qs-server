package questionnaire

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
	errorCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// Editor 问卷编辑器
type Editor struct {
	qRepoMySQL port.QuestionnaireRepositoryMySQL
	qRepoMongo port.QuestionnaireRepositoryMongo
}

// NewEditor 创建问卷编辑器
func NewEditor(
	qRepoMySQL port.QuestionnaireRepositoryMySQL,
	qRepoMongo port.QuestionnaireRepositoryMongo,
) *Editor {
	return &Editor{qRepoMySQL: qRepoMySQL, qRepoMongo: qRepoMongo}
}

// EditBasicInfo 编辑问卷基本信息
func (e *Editor) EditBasicInfo(
	ctx context.Context,
	code questionnaire.QuestionnaireCode,
	title, description, imgUrl string,
) (*questionnaire.Questionnaire, error) {
	// 1. 获取现有问卷
	qBo, err := e.qRepoMySQL.FindByCode(ctx, code.Value())
	log.Infow("---- qBo: ", "qBo", qBo)
	log.Infow("qBO", "qBo", qBo.GetID().Value())
	log.Infow("title", "title", title)
	log.Infow("description", "description", description)
	log.Infow("imgUrl", "imgUrl", imgUrl)
	if err != nil {
		return nil, err
	}

	// 2. 判断问卷状态
	if qBo.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能编辑")
	}

	// 3. 更新基本信息
	questionnaire.BaseInfoService{}.UpdateTitle(qBo, title)
	questionnaire.BaseInfoService{}.UpdateDescription(qBo, description)
	questionnaire.BaseInfoService{}.UpdateCoverImage(qBo, imgUrl)

	// 3. 保存到数据库
	if err := e.qRepoMySQL.Update(ctx, qBo); err != nil {
		return nil, err
	}

	// 4. 同步到文档数据库
	if err := e.qRepoMongo.Update(ctx, qBo); err != nil {
		return nil, err
	}

	return qBo, nil
}

// UpdateQuestions 更新问题
func (e *Editor) UpdateQuestions(
	ctx context.Context,
	code questionnaire.QuestionnaireCode,
	questions []question.Question,
) (*questionnaire.Questionnaire, error) {
	log.Infow("---- in Editor UpdateQuestions: ")
	// 1. 获取现有问卷
	qBo, err := e.qRepoMySQL.FindByCode(ctx, code.Value())
	if err != nil {
		return nil, err
	}
	log.Infow("---- qBo: ", "qBo", qBo)

	// 2. 判断问卷状态
	if qBo.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能编辑")
	}

	// 3. 保存问题
	questionService := questionnaire.QuestionService{}
	for _, q := range questions {
		questionService.AddQuestion(qBo, q)
	}

	log.Infow("---- qBo: ", "qBo", qBo)
	log.Infow("---- qBo.GetQuestions(): ", "qBo.GetQuestions()", qBo.GetQuestions())

	// 4. 保存到数据库
	if err := e.qRepoMongo.Update(ctx, qBo); err != nil {
		return nil, err
	}

	return qBo, nil
}
