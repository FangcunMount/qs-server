package ruleset

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type publishedModelWriter interface {
	UpsertPublishedModel(ctx context.Context, snapshot *publishing.PublishedModelSnapshot) error
}

// ScaleRuleSetPublisher syncs published scales into published_assessment_models.
type ScaleRuleSetPublisher struct {
	writer rulesetport.PublishedWriter
}

func NewScaleRuleSetPublisher(writer rulesetport.PublishedWriter) *ScaleRuleSetPublisher {
	return &ScaleRuleSetPublisher{writer: writer}
}

func (p *ScaleRuleSetPublisher) PublishPublishedScale(ctx context.Context, scale *scaledefinition.MedicalScale) error {
	if scale == nil {
		return fmt.Errorf("scale is nil")
	}
	if p == nil || p.writer == nil {
		return nil
	}
	if scale.GetStatus() != scaledefinition.StatusPublished {
		return fmt.Errorf("scale %s is not published", scale.GetCode().String())
	}
	published, err := publishing.BuildScoringPublishedSnapshotFromScale(
		evaluationinputInfra.MedicalScaleToSnapshot(scale),
	)
	if err != nil {
		return err
	}
	if writer, ok := p.writer.(publishedModelWriter); ok {
		return writer.UpsertPublishedModel(ctx, published)
	}
	return fmt.Errorf("published model writer is not configured")
}
