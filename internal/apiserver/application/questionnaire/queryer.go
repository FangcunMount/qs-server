package questionnaire

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
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
func (q *Queryer) GetQuestionnaire(ctx context.Context, id uint64) (*questionnaire.Questionnaire, error) {
	return q.quesRepo.FindByID(ctx, id)
}

// GetQuestionnaireByCode 根据编码获取问卷
func (q *Queryer) GetQuestionnaireByCode(ctx context.Context, code string) (*questionnaire.Questionnaire, error) {
	return q.quesRepo.FindByCode(ctx, code)
}

// ListQuestionnaires 获取问卷列表
func (q *Queryer) ListQuestionnaires(ctx context.Context, page, pageSize int) ([]*questionnaire.Questionnaire, int64, error) {
	// TODO: 实现分页查询逻辑
	// 这里需要根据实际的仓储接口方法来实现
	return nil, 0, nil
}
