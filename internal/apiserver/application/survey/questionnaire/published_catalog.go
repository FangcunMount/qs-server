package questionnaire

import (
	"context"

	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
)

// PublishedQuestionnaireCatalog adapts questionnaire repository data to the scale application catalog port.
type PublishedQuestionnaireCatalog struct {
	repo domainQuestionnaire.Repository
}

// NewPublishedQuestionnaireCatalog creates a scale-facing questionnaire catalog.
func NewPublishedQuestionnaireCatalog(repo domainQuestionnaire.Repository) *PublishedQuestionnaireCatalog {
	return &PublishedQuestionnaireCatalog{repo: repo}
}

// FindQuestionnaire returns the current questionnaire head facts.
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

// FindQuestionnaireVersion returns questionnaire facts for a concrete version.
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

// FindPublishedQuestionnaire returns the current published questionnaire facts.
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
	}
}
