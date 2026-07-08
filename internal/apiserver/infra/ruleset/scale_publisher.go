package ruleset

import (
	"context"
	"fmt"

	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// ScaleRuleSetPublisher 将已发布量表同步到 evaluation_rule_sets。
type ScaleRuleSetPublisher struct {
	writer rulesetport.PublishedRuleSetWriter
}

func NewScaleRuleSetPublisher(writer rulesetport.PublishedRuleSetWriter) *ScaleRuleSetPublisher {
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
	snapshot, err := ScaleRuleSetSnapshot(evaluationinputInfra.MedicalScaleToSnapshot(scale))
	if err != nil {
		return err
	}
	return p.writer.UpsertPublished(ctx, snapshot)
}
