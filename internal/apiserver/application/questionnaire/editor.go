package questionnaire

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
)

// Editor 问卷编辑器
type Editor struct {
	quesRepo port.QuestionnaireRepository
	quesDoc  port.QuestionnaireDocument
}

// NewEditor 创建问卷编辑器
func NewEditor(
	quesRepo port.QuestionnaireRepository,
	quesDoc port.QuestionnaireDocument,
) *Editor {
	return &Editor{quesRepo: quesRepo, quesDoc: quesDoc}
}

// EditBasicInfo 编辑问卷基本信息
func (e *Editor) EditBasicInfo(ctx context.Context, req port.QuestionnaireEditRequest) (*port.QuestionnaireResponse, error) {
	return nil, nil
}
