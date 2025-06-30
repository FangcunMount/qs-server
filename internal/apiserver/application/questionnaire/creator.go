package questionnaire

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
)

// Creator 问卷创建器
type Creator struct {
	quesRepo port.QuestionnaireRepository
	quesDoc  port.QuestionnaireDocument
}

// NewCreator 创建问卷创建器
func NewCreator(
	quesRepo port.QuestionnaireRepository,
	quesDoc port.QuestionnaireDocument,
) *Creator {
	return &Creator{quesRepo: quesRepo, quesDoc: quesDoc}
}

// CreateQuestionnaire 创建问卷
func (c *Creator) CreateQuestionnaire(ctx context.Context, req port.QuestionnaireCreateRequest) (*port.QuestionnaireResponse, error) {
	return nil, nil
}
