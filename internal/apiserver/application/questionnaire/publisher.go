package questionnaire

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/service"
)

// Publisher 问卷发布器
type Publisher struct {
	qRepoMySQL port.QuestionnaireRepositoryMySQL
	qRepoMongo port.QuestionnaireRepositoryMongo
}

// NewPublisher 创建问卷发布器
func NewPublisher(
	qRepoMySQL port.QuestionnaireRepositoryMySQL,
	qRepoMongo port.QuestionnaireRepositoryMongo,
) *Publisher {
	return &Publisher{qRepoMySQL: qRepoMySQL, qRepoMongo: qRepoMongo}
}

// PublishQuestionnaire 发布问卷
func (p *Publisher) Publish(
	ctx context.Context,
	code questionnaire.QuestionnaireCode,
) (*questionnaire.Questionnaire, error) {
	// 1. 获取问卷
	qBo, err := p.qRepoMySQL.FindByCode(ctx, code.Value())
	if err != nil {
		return nil, err
	}

	// 2. 更新状态为已发布
	service := service.VersionService{}
	service.Publish(qBo)

	// 3. 更新到数据库
	if err := p.qRepoMySQL.Update(ctx, qBo); err != nil {
		return nil, err
	}

	// 4. 同步到文档数据库
	if err := p.qRepoMongo.Update(ctx, qBo); err != nil {
		return nil, err
	}

	return qBo, nil
}

// UnpublishQuestionnaire 下架问卷
func (p *Publisher) Unpublish(
	ctx context.Context,
	code questionnaire.QuestionnaireCode,
) (*questionnaire.Questionnaire, error) {
	// 1. 获取问卷
	qBo, err := p.qRepoMySQL.FindByCode(ctx, code.Value())
	if err != nil {
		return nil, err
	}

	// 2. 更新状态为草稿
	service := service.VersionService{}
	service.Archive(qBo)

	// 3. 更新到数据库
	if err := p.qRepoMySQL.Update(ctx, qBo); err != nil {
		return nil, err
	}

	// 4. 同步到文档数据库
	if err := p.qRepoMongo.Update(ctx, qBo); err != nil {
		return nil, err
	}

	return qBo, nil
}
