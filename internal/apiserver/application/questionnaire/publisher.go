package questionnaire

import (
	"context"

	"github.com/fangcun-mount/qs-server/internal/apiserver/application/dto"
	"github.com/fangcun-mount/qs-server/internal/apiserver/application/mapper"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/questionnaire"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/questionnaire/port"
	errorCode "github.com/fangcun-mount/qs-server/internal/pkg/code"
	"github.com/fangcun-mount/qs-server/pkg/errors"
)

// Publisher 问卷发布器
type Publisher struct {
	qRepoMySQL port.QuestionnaireRepositoryMySQL
	qRepoMongo port.QuestionnaireRepositoryMongo
	mapper     mapper.QuestionnaireMapper
}

// NewPublisher 创建问卷发布器
func NewPublisher(
	qRepoMySQL port.QuestionnaireRepositoryMySQL,
	qRepoMongo port.QuestionnaireRepositoryMongo,
) *Publisher {
	return &Publisher{
		qRepoMySQL: qRepoMySQL,
		qRepoMongo: qRepoMongo,
		mapper:     mapper.NewQuestionnaireMapper(),
	}
}

// validateCode 验证问卷编码
func (p *Publisher) validateCode(code string) error {
	if code == "" {
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}
	return nil
}

// Publish 发布问卷
func (p *Publisher) Publish(
	ctx context.Context,
	code string,
) (*dto.QuestionnaireDTO, error) {
	// 1. 验证输入参数
	if err := p.validateCode(code); err != nil {
		return nil, err
	}

	// 2. 获取问卷
	qBo, err := p.qRepoMySQL.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 检查问卷状态
	if qBo.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能发布")
	}
	if qBo.IsPublished() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidStatus, "问卷已发布，不能重复发布")
	}

	// 4. 检查问题列表
	if len(qBo.GetQuestions()) == 0 {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidQuestion, "问卷没有问题，不能发布")
	}

	// 5. 更新状态为已发布
	versionService := questionnaire.VersionService{}
	versionService.Publish(qBo)

	// 6. 更新到数据库
	if err := p.qRepoMySQL.Update(ctx, qBo); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷状态失败")
	}

	// 7. 同步到文档数据库
	if err := p.qRepoMongo.Update(ctx, qBo); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "同步问卷状态失败")
	}

	// 8. 转换为 DTO 并返回
	return p.mapper.ToDTO(qBo), nil
}

// Unpublish 下架问卷
func (p *Publisher) Unpublish(
	ctx context.Context,
	code string,
) (*dto.QuestionnaireDTO, error) {
	// 1. 验证输入参数
	if err := p.validateCode(code); err != nil {
		return nil, err
	}

	// 2. 获取问卷
	qBo, err := p.qRepoMySQL.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 检查问卷状态
	if qBo.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能下架")
	}
	if !qBo.IsPublished() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidStatus, "问卷未发布，不能下架")
	}

	// 4. 更新状态为未发布
	versionService := questionnaire.VersionService{}
	versionService.Unpublish(qBo)

	// 5. 更新到数据库
	if err := p.qRepoMySQL.Update(ctx, qBo); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷状态失败")
	}

	// 6. 同步到文档数据库
	if err := p.qRepoMongo.Update(ctx, qBo); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "同步问卷状态失败")
	}

	// 7. 转换为 DTO 并返回
	return p.mapper.ToDTO(qBo), nil
}
