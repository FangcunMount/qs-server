package questionnaire

import (
	"context"

	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
)

// PublishedQuestionnaireCatalog 适配问卷 仓储 data 到 scale application 目录 port。
type PublishedQuestionnaireCatalog struct {
	repo domainQuestionnaire.Repository
}

// NewPublishedQuestionnaireCatalog 创建scale-facing 问卷 目录。
func NewPublishedQuestionnaireCatalog(repo domainQuestionnaire.Repository) *PublishedQuestionnaireCatalog {
	return &PublishedQuestionnaireCatalog{repo: repo}
}

// FindQuestionnaire 返回当前 问卷 head 事实。
func (c *PublishedQuestionnaireCatalog) FindQuestionnaire(ctx context.Context, code string) (*questionnairecatalog.Item, error) {
	if c == nil || c.repo == nil {
		return nil, domainQuestionnaire.ErrNotFound
	}
	q, err := c.repo.FindByCode(ctx, code)
	if err != nil || q == nil {
		return nil, err
	}
	return questionnaireCatalogItem(q), nil
}

// FindQuestionnaireVersion 返回问卷 事实 用于 concrete version。
func (c *PublishedQuestionnaireCatalog) FindQuestionnaireVersion(ctx context.Context, code, version string) (*questionnairecatalog.Item, error) {
	if c == nil || c.repo == nil {
		return nil, domainQuestionnaire.ErrNotFound
	}
	q, err := c.repo.FindByCodeVersion(ctx, code, version)
	if err != nil || q == nil {
		return nil, err
	}
	return questionnaireCatalogItem(q), nil
}

// FindPublishedQuestionnaire 返回当前 published 问卷 事实。
func (c *PublishedQuestionnaireCatalog) FindPublishedQuestionnaire(ctx context.Context, code string) (*questionnairecatalog.Item, error) {
	if c == nil || c.repo == nil {
		return nil, domainQuestionnaire.ErrNotFound
	}
	q, err := c.repo.FindPublishedByCode(ctx, code)
	if err != nil || q == nil {
		return nil, err
	}
	return questionnaireCatalogItem(q), nil
}

func questionnaireCatalogItem(q *domainQuestionnaire.Questionnaire) *questionnairecatalog.Item {
	return &questionnairecatalog.Item{
		Code:    q.GetCode().String(),
		Version: q.GetVersion().String(),
		Type:    q.GetType().String(),
		Status:  q.GetStatus().String(),
	}
}
