package outcome

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// PublishedSnapshotFromInput 构建路由 快照 从 resolved 评估输入。
func PublishedSnapshotFromInput(input *evaluationinput.InputSnapshot) (modelcatalog.PublishedModelSnapshot, bool) {
	if input == nil {
		return modelcatalog.PublishedModelSnapshot{}, false
	}
	if input.Model == nil {
		if scale, ok := evaluationinput.ScalePayload(input); ok {
			input.Model = evaluationinput.NewScaleModelSnapshot(scale)
		}
	}
	if input.Model == nil {
		return modelcatalog.PublishedModelSnapshot{}, false
	}
	model := input.Model
	kind := modelcatalog.Kind(model.Kind)
	subKind := modelcatalog.SubKind(model.SubKind)
	algorithm := modelcatalog.Algorithm(model.Algorithm)

	decision := modelcatalog.DecisionSpec{}
	if payload, ok := evaluationinput.TypologyPayload(input); ok && payload.HasExplicitRuntime() && payload.Runtime.Decision.Kind != "" {
		decision.Kind = payload.Runtime.Decision.Kind
	} else if resolved, ok := modelcatalog.DecisionKindForIdentity(kind, subKind, algorithm); ok {
		decision.Kind = resolved
	}

	payloadFormat := modelcatalog.DraftPayloadFormatForModel(kind, algorithm)
	return modelcatalog.PublishedModelSnapshot{
		Model: modelcatalog.ModelDefinition{
			Kind:      kind,
			SubKind:   subKind,
			Algorithm: algorithm,
			Code:      model.Code,
			Version:   model.Version,
			Title:     model.Title,
		},
		Decision:      decision,
		PayloadFormat: payloadFormat,
	}, true
}
