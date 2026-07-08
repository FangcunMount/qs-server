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
	ruleSets []*domain.RuleSetSnapshot
	scale    ScaleBindingSource
}

var _ port.RuleSetCatalog = (*StaticCompositeCatalog)(nil)

func NewStaticCompositeCatalog(ruleSets []*domain.RuleSetSnapshot, scale ScaleBindingSource) *StaticCompositeCatalog {
	copied := make([]*domain.RuleSetSnapshot, 0, len(ruleSets))
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
) (port.RuleSetRef, bool, error) {
	if c == nil {
		return port.RuleSetRef{}, false, nil
	}
	if snapshot := c.findRuleSetByQuestionnaire(questionnaireCode, questionnaireVersion); snapshot != nil {
		return RuleSetRefFromSnapshot(snapshot), true, nil
	}
	if c.scale != nil {
		model, err := c.scale.FindScaleByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
		if err != nil {
			if domain.IsNotFound(err) {
				return port.RuleSetRef{}, false, nil
			}
			return port.RuleSetRef{}, false, err
		}
		if model != nil {
			return scaleRuleSetRef(model), true, nil
		}
	}
	return port.RuleSetRef{}, false, nil
}

func (c *StaticCompositeCatalog) GetPublishedByRef(ctx context.Context, ref port.RuleSetRef) (*domain.RuleSetSnapshot, error) {
	if c == nil {
		return nil, fmt.Errorf("ruleset catalog is not configured")
	}
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	if snapshot := c.findRuleSetByRef(ref); snapshot != nil {
		return snapshot, nil
	}
	if ref.Kind == domain.RuleSetKindScale {
		if c.scale == nil {
			return nil, domain.ErrNotFound
		}
		model, err := c.scale.GetScaleByRef(ctx, ref.Code, ref.Version)
		if err != nil {
			return nil, err
		}
		return ScaleRuleSetSnapshot(model)
	}
	return nil, domain.ErrNotFound
}

func (c *StaticCompositeCatalog) FindPublishedByQuestionnaire(
	ctx context.Context,
	questionnaireCode, questionnaireVersion string,
) (*domain.RuleSetSnapshot, error) {
	ref, ok, err := c.ResolveByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrNotFound
	}
	return c.GetPublishedByRef(ctx, ref)
}

func (c *StaticCompositeCatalog) findRuleSetByQuestionnaire(questionnaireCode, questionnaireVersion string) *domain.RuleSetSnapshot {
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

func (c *StaticCompositeCatalog) findRuleSetByRef(ref port.RuleSetRef) *domain.RuleSetSnapshot {
	for _, snapshot := range c.ruleSets {
		if snapshot == nil {
			continue
		}
		if aminfra.RefMatchesSnapshot(ref, snapshot) {
			return snapshot
		}
	}
	return nil
}

func scaleRuleSetRef(model *scalesnapshot.ScaleSnapshot) port.RuleSetRef {
	if model == nil {
		return port.RuleSetRef{}
	}
	return port.RuleSetRef{
		Kind:    domain.RuleSetKindScale,
		Code:    model.Code,
		Version: model.ScaleVersion,
		Title:   model.Title,
	}
}
