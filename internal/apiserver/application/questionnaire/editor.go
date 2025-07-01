package questionnaire

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
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
func (e *Editor) EditBasicInfo(ctx context.Context, id uint64, title, imgUrl string, version uint8) (*questionnaire.Questionnaire, error) {
	// 1. 获取现有问卷
	ques, err := e.quesRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 2. 更新基本信息
	ques.Title = title
	ques.ImgUrl = imgUrl
	ques.Version = version

	// 3. 保存到数据库
	if err := e.quesRepo.Save(ctx, ques); err != nil {
		return nil, err
	}

	// 4. 同步到文档数据库
	if err := e.quesDoc.Save(ctx, ques); err != nil {
		return nil, err
	}

	return ques, nil
}
