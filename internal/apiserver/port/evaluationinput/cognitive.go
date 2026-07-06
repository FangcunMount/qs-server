package evaluationinput

import (
	"context"

	cognitivesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/cognitive/snapshot"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
)

const EvaluationModelKindCognitive EvaluationModelKind = "cognitive"

type CognitiveModelPayload struct {
	Snapshot *cognitivesnapshot.Snapshot
}

func (CognitiveModelPayload) RuleSetKind() EvaluationModelKind {
	return EvaluationModelKindCognitive
}

func NewCognitiveModelSnapshot(snapshot *cognitivesnapshot.Snapshot) *ModelSnapshot {
	if snapshot == nil {
		return nil
	}
	return &ModelSnapshot{
		Kind:      EvaluationModelKindCognitive,
		Algorithm: "spm",
		Code:      snapshot.Code,
		Version:   snapshot.Version,
		Title:     snapshot.Title,
		Payload:   CognitiveModelPayload{Snapshot: snapshot},
	}
}

func CognitivePayload(input *InputSnapshot) (CognitiveModelPayload, bool) {
	if input == nil {
		return CognitiveModelPayload{}, false
	}
	if payload, ok := input.ModelPayload.(CognitiveModelPayload); ok && payload.Snapshot != nil {
		return payload, true
	}
	if input.Model != nil {
		if payload, ok := input.Model.Payload.(CognitiveModelPayload); ok && payload.Snapshot != nil {
			return payload, true
		}
	}
	return CognitiveModelPayload{}, false
}

func CognitiveScaleSnapshot(input *InputSnapshot) (*scalesnapshot.ScaleSnapshot, bool) {
	payload, ok := CognitivePayload(input)
	if !ok || payload.Snapshot == nil {
		return nil, false
	}
	return payload.Snapshot.ToScaleSnapshot(), true
}

type CognitiveModelCatalog interface {
	GetCognitiveByRef(ctx context.Context, ref ModelRef) (*cognitivesnapshot.Snapshot, error)
	FindCognitiveByQuestionnaire(ctx context.Context, code, version string) (*cognitivesnapshot.Snapshot, error)
}
