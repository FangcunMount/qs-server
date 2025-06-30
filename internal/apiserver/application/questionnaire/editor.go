package questionnaire

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
)

type Editor struct {
	questionnaireRepo port.QuestionnaireRepository
}

func NewEditor(questionnaireRepo port.QuestionnaireRepository) *Editor {
	return &Editor{questionnaireRepo: questionnaireRepo}
}

func (e *Editor) EditQuestionnaire(ctx context.Context, req port.QuestionnaireEditRequest) (*port.QuestionnaireResponse, error) {
	return nil, nil
}
