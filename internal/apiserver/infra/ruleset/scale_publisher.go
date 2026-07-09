package ruleset

import (
	"context"
	"fmt"

	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	aminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type ScalePublisher struct {
	writer rulesetport.PublishedWriter
}

func NewScalePublisher(writer rulesetport.PublishedWriter) *ScalePublisher {
	return &ScalePublisher{writer: writer}
}

func (p *ScalePublisher) PublishPublishedScale(ctx context.Context, scale *scaledefinition.MedicalScale) error {
	if scale == nil {
		return fmt.Errorf("scale is nil")
	}
	if p == nil || p.writer == nil {
		return nil
	}
	if scale.GetStatus() != scaledefinition.StatusPublished {
		return fmt.Errorf("scale %s is not published", scale.GetCode().String())
	}
	published, err := aminfra.BuildScalePublishedSnapshot(
		evaluationinputInfra.MedicalScaleToSnapshot(scale),
	)
	if err != nil {
		return err
	}
	return p.writer.UpsertPublishedModel(ctx, published)
}
