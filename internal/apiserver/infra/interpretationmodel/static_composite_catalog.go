package interpretationmodel

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretationmodel"
	evaluationinputPort "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationmodel"
)

type StaticCompositeCatalog struct {
	ruleSets []*domain.RuleSetSnapshot
	scale    ScaleBindingSource
}

var _ port.ModelCatalog = (*StaticCompositeCatalog)(nil)

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
) (port.ModelRef, bool, error) {
	if c == nil {
		return port.ModelRef{}, false, nil
	}
	if snapshot := c.findRuleSetByQuestionnaire(questionnaireCode, questionnaireVersion); snapshot != nil {
		return ModelRefFromSnapshot(snapshot), true, nil
	}
	if c.scale != nil {
		if model, err := c.scale.FindScaleByQuestionnaire(ctx, questionnaireCode, questionnaireVersion); err == nil && model != nil {
			return scaleModelRef(model), true, nil
		}
	}
	return port.ModelRef{}, false, nil
}

func (c *StaticCompositeCatalog) GetPublishedByRef(ctx context.Context, ref port.ModelRef) (*domain.RuleSetSnapshot, error) {
	if c == nil {
		return nil, fmt.Errorf("interpretation model catalog is not configured")
	}
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	if snapshot := c.findRuleSetByRef(ref); snapshot != nil {
		return snapshot, nil
	}
	if ref.Kind == domain.ModelKindScale {
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

func (c *StaticCompositeCatalog) findRuleSetByRef(ref port.ModelRef) *domain.RuleSetSnapshot {
	for _, snapshot := range c.ruleSets {
		if snapshot == nil {
			continue
		}
		if snapshot.Definition.Kind == ref.Kind &&
			snapshot.Definition.Code == ref.Code &&
			snapshot.Definition.Version == ref.Version {
			return snapshot
		}
	}
	return nil
}

func scaleModelRef(model *evaluationinputPort.ScaleSnapshot) port.ModelRef {
	if model == nil {
		return port.ModelRef{}
	}
	return port.ModelRef{
		Kind:    domain.ModelKindScale,
		Code:    model.Code,
		Version: model.ScaleVersion,
		Title:   model.Title,
	}
}
