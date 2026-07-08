package ruleset

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
	aminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type StaticCompositeCatalog struct {
	ruleSets []*domain.PublishedModelSnapshot
	scale    ScaleBindingSource
}

var _ port.Catalog = (*StaticCompositeCatalog)(nil)

func NewStaticCompositeCatalog(ruleSets []*domain.PublishedModelSnapshot, scale ScaleBindingSource) *StaticCompositeCatalog {
	copied := make([]*domain.PublishedModelSnapshot, 0, len(ruleSets))
	for _, snapshot := range ruleSets {
		if snapshot == nil {
			continue
		}
		clone := *snapshot
		if len(snapshot.Payload) > 0 {
			clone.Payload = append([]byte(nil), snapshot.Payload...)
		}
		copied = append(copied, &clone)
	}
	return &StaticCompositeCatalog{ruleSets: copied, scale: scale}
}

func (c *StaticCompositeCatalog) ResolveByQuestionnaire(
	ctx context.Context,
	questionnaireCode, questionnaireVersion string,
) (port.Ref, bool, error) {
	if c == nil {
		return port.Ref{}, false, nil
	}
	if snapshot := c.findRuleSetByQuestionnaire(questionnaireCode, questionnaireVersion); snapshot != nil {
		return aminfra.RefFromPublished(snapshot), true, nil
	}
	if c.scale != nil {
		model, err := c.scale.FindScaleByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
		if err != nil {
			if domain.IsNotFound(err) {
				return port.Ref{}, false, nil
			}
			return port.Ref{}, false, err
		}
		if model != nil {
			return scaleRuleSetRef(model), true, nil
		}
	}
	return port.Ref{}, false, nil
}

func (c *StaticCompositeCatalog) GetPublishedModelByRef(ctx context.Context, ref port.Ref) (*domain.PublishedModelSnapshot, error) {
	if c == nil {
		return nil, fmt.Errorf("ruleset catalog is not configured")
	}
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	if snapshot := c.findRuleSetByRef(ref); snapshot != nil {
		return snapshot, nil
	}
	if ref.Kind == domain.KindScale {
		if c.scale == nil {
			return nil, domain.ErrNotFound
		}
		model, err := c.scale.GetScaleByRef(ctx, ref.Code, ref.Version)
		if err != nil {
			return nil, err
		}
		return aminfra.BuildScalePublishedSnapshot(model)
	}
	return nil, domain.ErrNotFound
}

func (c *StaticCompositeCatalog) FindPublishedModelByQuestionnaire(
	ctx context.Context,
	questionnaireCode, questionnaireVersion string,
) (*domain.PublishedModelSnapshot, error) {
	ref, ok, err := c.ResolveByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrNotFound
	}
	return c.GetPublishedModelByRef(ctx, ref)
}

func (c *StaticCompositeCatalog) findRuleSetByQuestionnaire(questionnaireCode, questionnaireVersion string) *domain.PublishedModelSnapshot {
	for _, snapshot := range c.ruleSets {
		if snapshot == nil {
			continue
		}
		if snapshot.Binding.QuestionnaireCode == questionnaireCode && snapshot.Binding.QuestionnaireVersion == questionnaireVersion {
			return snapshot
		}
	}
	return nil
}

func (c *StaticCompositeCatalog) findRuleSetByRef(ref port.Ref) *domain.PublishedModelSnapshot {
	for _, snapshot := range c.ruleSets {
		if snapshot == nil {
			continue
		}
		if aminfra.RefMatchesPublished(ref, snapshot) {
			return snapshot
		}
	}
	return nil
}

func scaleRuleSetRef(model *scalesnapshot.ScaleSnapshot) port.Ref {
	if model == nil {
		return port.Ref{}
	}
	return port.Ref{
		Kind:    domain.KindScale,
		Code:    model.Code,
		Version: model.ScaleVersion,
		Title:   model.Title,
	}
}
