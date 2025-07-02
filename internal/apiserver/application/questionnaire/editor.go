package questionnaire

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
)

// Editor 问卷编辑器
type Editor struct {
	qRepoMySQL port.QuestionnaireRepositoryMySQL
	qRepoMongo port.QuestionnaireRepositoryMongo
}

// NewEditor 创建问卷编辑器
func NewEditor(
	qRepoMySQL port.QuestionnaireRepositoryMySQL,
	qRepoMongo port.QuestionnaireRepositoryMongo,
) *Editor {
	return &Editor{qRepoMySQL: qRepoMySQL, qRepoMongo: qRepoMongo}
}

// EditBasicInfo 编辑问卷基本信息
func (e *Editor) EditBasicInfo(
	ctx context.Context,
	code questionnaire.QuestionnaireCode,
	title, description, imgUrl string,
) (*questionnaire.Questionnaire, error) {
	// 1. 获取现有问卷
	qBo, err := e.qRepoMySQL.FindByCode(ctx, code.Value())
	if err != nil {
		return nil, err
	}

	// 2. 更新基本信息
	qBo.ChangeBasicInfo(title, description, imgUrl)

	// 3. 保存到数据库
	if err := e.qRepoMySQL.Save(ctx, qBo); err != nil {
		return nil, err
	}

	// 4. 同步到文档数据库
	if err := e.qRepoMongo.Save(ctx, qBo); err != nil {
		return nil, err
	}

	return qBo, nil
}
