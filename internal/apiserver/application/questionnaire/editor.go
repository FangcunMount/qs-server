package questionnaire

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
	errorCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
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

	// 2. 判断问卷状态
	if qBo.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能编辑")
	}

	// 已发布的问卷，Copy 一份新的，旧版本归档
	if qBo.IsPublished() {
		// 归档旧版本
		questionnaire.VersionService{}.Archive(qBo)

		// 创建新版本
		qBo = questionnaire.VersionService{}.Clone(qBo)
	}

	// 3. 更新基本信息
	service := questionnaire.BaseInfoService{}
	service.UpdateTitle(qBo, title)
	service.UpdateDescription(qBo, description)
	// service.UpdateCoverImage(qBo, imgUrl)

	// 3. 保存到数据库
	if err := e.qRepoMySQL.Update(ctx, qBo); err != nil {
		return nil, err
	}

	// 4. 同步到文档数据库
	if err := e.qRepoMongo.Save(ctx, qBo); err != nil {
		return nil, err
	}

	return qBo, nil
}
