package evaluationinput

import (
	"context"
	taskperfsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

const EvaluationModelKindCognitive EvaluationModelKind = "cognitive"

type CognitiveModelPayload struct {
	Snapshot *taskperfsnapshot.Snapshot
}

func (CognitiveModelPayload) RuleSetKind() EvaluationModelKind {
	return EvaluationModelKindCognitive
}

func NewCognitiveModelSnapshot(snapshot *taskperfsnapshot.Snapshot) *ModelSnapshot {
	if snapshot == nil {
		return nil
	}
	ms := &ModelSnapshot{
		Kind:    EvaluationModelKindCognitive,
		Code:    snapshot.Code,
		Version: snapshot.Version,
		Title:   snapshot.Title,
		Payload: CognitiveModelPayload{Snapshot: snapshot},
	}
	return applyPublishedRuntime(ms, snapshot.PublishedRuntime)
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
	GetCognitiveByRef(ctx context.Context, ref ModelRef) (*taskperfsnapshot.Snapshot, error)
	FindCognitiveByQuestionnaire(ctx context.Context, code, version string) (*taskperfsnapshot.Snapshot, error)
}
