package questionnaire

import (
	"context"

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
func (p *Publisher) PublishQuestionnaire(ctx context.Context, req port.QuestionnairePublishRequest) (*port.QuestionnaireResponse, error) {
	return nil, nil
}

// UnpublishQuestionnaire 下架问卷
func (p *Publisher) UnpublishQuestionnaire(ctx context.Context, req port.QuestionnaireUnpublishRequest) (*port.QuestionnaireResponse, error) {
	return nil, nil
}
