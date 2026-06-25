package interpretationmodel

import (
	"context"
	"fmt"

	domscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	interpretationmodelport "github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationmodel"
)

// ScaleInterpretationPublisher 将已发布量表同步到 interpretation_models。
type ScaleInterpretationPublisher struct {
	writer interpretationmodelport.PublishedModelWriter
}

func NewScaleInterpretationPublisher(writer interpretationmodelport.PublishedModelWriter) *ScaleInterpretationPublisher {
	return &ScaleInterpretationPublisher{writer: writer}
}

func (p *ScaleInterpretationPublisher) PublishPublishedScale(ctx context.Context, scale *domscale.MedicalScale) error {
	if scale == nil {
		return fmt.Errorf("scale is nil")
	}
	if p == nil || p.writer == nil {
		return nil
	}
	if scale.GetStatus() != domscale.StatusPublished {
		return fmt.Errorf("scale %s is not published", scale.GetCode().String())
	}
	snapshot, err := ScaleRuleSetSnapshot(evaluationinputInfra.MedicalScaleToSnapshot(scale))
	if err != nil {
		return err
	}
	return p.writer.UpsertPublished(ctx, snapshot)
}
