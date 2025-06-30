package questionnaire

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
)

// Editor 问卷编辑器
type Editor struct {
	questionnaireRepo port.QuestionnaireRepository
}

// NewEditor 创建问卷编辑器
func NewEditor(quesRepo port.QuestionnaireRepository) *Editor {
	return &Editor{questionnaireRepo: quesRepo}
}

// EditBasicInfo 编辑问卷基本信息
func (e *Editor) EditBasicInfo(ctx context.Context, req port.QuestionnaireEditRequest) (*port.QuestionnaireResponse, error) {
	return nil, nil
}
