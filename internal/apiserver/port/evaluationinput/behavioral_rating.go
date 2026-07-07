package evaluationinput

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/snapshot"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
)

const EvaluationModelKindBehavioralRating EvaluationModelKind = "behavioral_rating"

type BehavioralRatingModelPayload struct {
	Snapshot *behavioralsnapshot.Snapshot
}

func (BehavioralRatingModelPayload) RuleSetKind() EvaluationModelKind {
	return EvaluationModelKindBehavioralRating
}

func NewBehavioralRatingModelSnapshot(snapshot *behavioralsnapshot.Snapshot) *ModelSnapshot {
	if snapshot == nil {
		return nil
	}
	version := snapshot.Version
	algorithm := string(modelcatalog.AlgorithmBehavioralRatingDefault)
	if snapshot.Brief2 != nil {
		algorithm = string(modelcatalog.AlgorithmBrief2)
	}
	return &ModelSnapshot{
		Kind:      EvaluationModelKindBehavioralRating,
		Algorithm: algorithm,
		Code:      snapshot.Code,
		Version:   version,
		Title:     snapshot.Title,
		Payload:   BehavioralRatingModelPayload{Snapshot: snapshot},
	}
}

func BehavioralRatingPayload(input *InputSnapshot) (BehavioralRatingModelPayload, bool) {
	if input == nil {
		return BehavioralRatingModelPayload{}, false
	}
	if payload, ok := input.ModelPayload.(BehavioralRatingModelPayload); ok && payload.Snapshot != nil {
		return payload, true
	}
	if input.Model != nil {
		if payload, ok := input.Model.Payload.(BehavioralRatingModelPayload); ok && payload.Snapshot != nil {
			return payload, true
		}
	}
	return BehavioralRatingModelPayload{}, false
}

// BehavioralRatingScaleSnapshot projects behavioral_rating payload to scale execution shape.
func BehavioralRatingScaleSnapshot(input *InputSnapshot) (*scalesnapshot.ScaleSnapshot, bool) {
	payload, ok := BehavioralRatingPayload(input)
	if !ok || payload.Snapshot == nil {
		return nil, false
	}
	return payload.Snapshot.ToScaleSnapshot(), true
}

type BehavioralRatingModelCatalog interface {
	GetBehavioralRatingByRef(ctx context.Context, ref ModelRef) (*behavioralsnapshot.Snapshot, error)
	FindBehavioralRatingByQuestionnaire(ctx context.Context, code, version string) (*behavioralsnapshot.Snapshot, error)
}
