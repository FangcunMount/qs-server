package questionnaire

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
)

type Queryer struct {
	questionnaireRepo port.QuestionnaireRepository
}

func NewQueryer(questionnaireRepo port.QuestionnaireRepository) *Queryer {
	return &Queryer{questionnaireRepo: questionnaireRepo}
}

func (q *Queryer) GetQuestionnaire(ctx context.Context, req port.QuestionnaireIDRequest) (*port.QuestionnaireResponse, error) {
	return nil, nil
}

func (q *Queryer) GetQuestionnaireByCode(ctx context.Context, code string) (*port.QuestionnaireResponse, error) {
	return nil, nil
}

func (q *Queryer) ListQuestionnaires(ctx context.Context, page, pageSize int) (*port.QuestionnaireListResponse, error) {
	return nil, nil
}
