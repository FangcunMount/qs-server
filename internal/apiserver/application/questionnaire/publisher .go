package questionnaire

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
)

type Publisher struct {
	questionnaireRepo port.QuestionnaireRepository
}

func NewPublisher(questionnaireRepo port.QuestionnaireRepository) *Publisher {
	return &Publisher{questionnaireRepo: questionnaireRepo}
}

func (p *Publisher) PublishQuestionnaire(ctx context.Context, req port.QuestionnairePublishRequest) (*port.QuestionnaireResponse, error) {
	return nil, nil
}

func (p *Publisher) UnpublishQuestionnaire(ctx context.Context, req port.QuestionnaireUnpublishRequest) (*port.QuestionnaireResponse, error) {
	return nil, nil
}
