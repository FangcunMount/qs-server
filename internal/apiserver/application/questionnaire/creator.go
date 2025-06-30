package questionnaire

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
)

type Creator struct {
	questionnaireRepo port.QuestionnaireRepository
}

func NewCreator(questionnaireRepo port.QuestionnaireRepository) *Creator {
	return &Creator{questionnaireRepo: questionnaireRepo}
}

func (c *Creator) CreateQuestionnaire(ctx context.Context, req port.QuestionnaireCreateRequest) (*port.QuestionnaireResponse, error) {
	return nil, nil
}
