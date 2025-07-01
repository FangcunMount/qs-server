package questionnaire

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
)

// Publisher 问卷发布器
type Publisher struct {
	quesRepo port.QuestionnaireRepository
	quesDoc  port.QuestionnaireDocument
}

// NewPublisher 创建问卷发布器
func NewPublisher(
	quesRepo port.QuestionnaireRepository,
	quesDoc port.QuestionnaireDocument,
) *Publisher {
	return &Publisher{quesRepo: quesRepo, quesDoc: quesDoc}
}

// PublishQuestionnaire 发布问卷
func (p *Publisher) PublishQuestionnaire(ctx context.Context, id uint64) (*questionnaire.Questionnaire, error) {
	// 1. 获取问卷
	ques, err := p.quesRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 2. 更新状态为已发布
	ques.Status = questionnaire.STATUS_PUBLISHED.Value()

	// 3. 保存到数据库
	if err := p.quesRepo.Save(ctx, ques); err != nil {
		return nil, err
	}

	// 4. 同步到文档数据库
	if err := p.quesDoc.Save(ctx, ques); err != nil {
		return nil, err
	}

	return ques, nil
}

// UnpublishQuestionnaire 下架问卷
func (p *Publisher) UnpublishQuestionnaire(ctx context.Context, id uint64) (*questionnaire.Questionnaire, error) {
	// 1. 获取问卷
	ques, err := p.quesRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 2. 更新状态为草稿
	ques.Status = questionnaire.STATUS_DRAFT.Value()

	// 3. 保存到数据库
	if err := p.quesRepo.Save(ctx, ques); err != nil {
		return nil, err
	}

	// 4. 同步到文档数据库
	if err := p.quesDoc.Save(ctx, ques); err != nil {
		return nil, err
	}

	return ques, nil
}
