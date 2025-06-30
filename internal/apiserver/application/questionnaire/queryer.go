package questionnaire

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
)

// Queryer 问卷查询器
type Queryer struct {
	quesRepo port.QuestionnaireRepository
	quesDoc  port.QuestionnaireDocument
}

// NewQueryer 创建问卷查询器
func NewQueryer(
	quesRepo port.QuestionnaireRepository,
	quesDoc port.QuestionnaireDocument,
) *Queryer {
	return &Queryer{quesRepo: quesRepo, quesDoc: quesDoc}
}

// GetQuestionnaire 根据ID获取问卷
func (q *Queryer) GetQuestionnaire(ctx context.Context, req port.QuestionnaireIDRequest) (*port.QuestionnaireResponse, error) {
	return nil, nil
}

// GetQuestionnaireByCode 根据编码获取问卷
func (q *Queryer) GetQuestionnaireByCode(ctx context.Context, code string) (*port.QuestionnaireResponse, error) {
	return nil, nil
}

// ListQuestionnaires 获取问卷列表
func (q *Queryer) ListQuestionnaires(ctx context.Context, page, pageSize int) (*port.QuestionnaireListResponse, error) {
	return nil, nil
}
