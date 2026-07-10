package ruleset

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type StaticCompositeCatalog struct {
	ruleSets []*port.PublishedModel
}

var _ port.Catalog = (*StaticCompositeCatalog)(nil)

func NewStaticCompositeCatalog(ruleSets []*port.PublishedModel) *StaticCompositeCatalog {
	copied := make([]*port.PublishedModel, 0, len(ruleSets))
	for _, model := range ruleSets {
		if model == nil {
			continue
		}
		clone := *model
		if len(model.Payload) > 0 {
			clone.Payload = append([]byte(nil), model.Payload...)
		}
		copied = append(copied, &clone)
	}
	return &StaticCompositeCatalog{ruleSets: copied}
}

func (c *StaticCompositeCatalog) ResolveByQuestionnaire(
	ctx context.Context,
	questionnaireCode, questionnaireVersion string,
) (port.Ref, bool, error) {
	if c == nil {
		return port.Ref{}, false, nil
	}
	if snapshot := c.findRuleSetByQuestionnaire(questionnaireCode, questionnaireVersion); snapshot != nil {
		return port.RefFromPublished(snapshot), true, nil
	}
	return port.Ref{}, false, nil
}

func (c *StaticCompositeCatalog) GetPublishedModelByRef(ctx context.Context, ref port.Ref) (*port.PublishedModel, error) {
	if c == nil {
		return nil, fmt.Errorf("ruleset catalog is not configured")
	}
	if snapshot := c.findRuleSetByRef(ref); snapshot != nil {
		return snapshot, nil
	}
	return nil, fmt.Errorf("published static model is not found")
}

func (c *StaticCompositeCatalog) FindPublishedModelByQuestionnaire(
	ctx context.Context,
	questionnaireCode, questionnaireVersion string,
) (*port.PublishedModel, error) {
	ref, ok, err := c.ResolveByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrNotFound
	}
	return c.GetPublishedModelByRef(ctx, ref)
}

func (c *StaticCompositeCatalog) findRuleSetByQuestionnaire(questionnaireCode, questionnaireVersion string) *port.PublishedModel {
	for _, model := range c.ruleSets {
		if model == nil {
			continue
		}
		if model.QuestionnaireCode == questionnaireCode && model.QuestionnaireVersion == questionnaireVersion {
			return model
		}
	}
	return nil
}

func (c *StaticCompositeCatalog) findRuleSetByRef(ref port.Ref) *port.PublishedModel {
	for _, model := range c.ruleSets {
		if model == nil {
			continue
		}
		if port.RefMatchesPublished(ref, model) {
			return model
		}
	}
	return nil
}
