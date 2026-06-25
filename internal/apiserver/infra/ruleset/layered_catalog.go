package ruleset

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
)

type publishedStore interface {
	GetPublishedByRef(ctx context.Context, ref port.RuleSetRef) (*domain.RuleSetSnapshot, error)
	FindPublishedByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.RuleSetSnapshot, error)
}

type LayeredCatalog struct {
	store    publishedStore
	fallback port.RuleSetCatalog
}

var _ port.RuleSetCatalog = (*LayeredCatalog)(nil)

func NewLayeredCatalog(store publishedStore, fallback port.RuleSetCatalog) *LayeredCatalog {
	return &LayeredCatalog{store: store, fallback: fallback}
}

func (c *LayeredCatalog) ResolveByQuestionnaire(
	ctx context.Context,
	questionnaireCode, questionnaireVersion string,
) (port.RuleSetRef, bool, error) {
	if c == nil {
		return port.RuleSetRef{}, false, nil
	}
	if c.store != nil {
		snapshot, err := c.store.FindPublishedByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
		if err == nil && snapshot != nil {
			return RuleSetRefFromSnapshot(snapshot), true, nil
		}
		if err != nil && !domain.IsNotFound(err) {
			return port.RuleSetRef{}, false, err
		}
	}
	if c.fallback == nil {
		return port.RuleSetRef{}, false, nil
	}
	return c.fallback.ResolveByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
}

func (c *LayeredCatalog) GetPublishedByRef(ctx context.Context, ref port.RuleSetRef) (*domain.RuleSetSnapshot, error) {
	if c == nil {
		return nil, domain.ErrNotFound
	}
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	if c.store != nil {
		snapshot, err := c.store.GetPublishedByRef(ctx, ref)
		if err == nil {
			return snapshot, nil
		}
		if !domain.IsNotFound(err) {
			return nil, err
		}
	}
	if c.fallback == nil {
		return nil, domain.ErrNotFound
	}
	return c.fallback.GetPublishedByRef(ctx, ref)
}

func (c *LayeredCatalog) FindPublishedByQuestionnaire(
	ctx context.Context,
	questionnaireCode, questionnaireVersion string,
) (*domain.RuleSetSnapshot, error) {
	if c == nil {
		return nil, domain.ErrNotFound
	}
	if c.store != nil {
		snapshot, err := c.store.FindPublishedByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
		if err == nil {
			return snapshot, nil
		}
		if !domain.IsNotFound(err) {
			return nil, err
		}
	}
	if c.fallback == nil {
		return nil, domain.ErrNotFound
	}
	return c.fallback.FindPublishedByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
}
