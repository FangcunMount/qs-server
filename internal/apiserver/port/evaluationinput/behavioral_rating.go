package evaluationinput

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

const EvaluationModelKindBehavioralRating EvaluationModelKind = "behavioral_rating"

type BehavioralRatingModelPayload struct {
	Snapshot *behavioralsnapshot.Snapshot
}

func (BehavioralRatingModelPayload) RuleSetKind() EvaluationModelKind {
	return EvaluationModelKindBehavioralRating
}

func NewBehavioralRatingModelSnapshot(snapshot *behavioralsnapshot.Snapshot, algorithm modelcatalog.Algorithm) *ModelSnapshot {
	if snapshot == nil {
		return nil
	}
	version := snapshot.Version
	// MC-R018 batch 5: empty Algorithm fills canonical brief2 (not retained-read
	// behavioral_rating_default). Dual-identity lookup still resolves historical
	// Assessment rows that store behavioral_rating_default.
	if algorithm == "" {
		filled := modelcatalog.AlgorithmBrief2
		modelcatalog.ObserveAlgorithmFallback(
			modelcatalog.KindBehavioralRating, "", filled, "evaluationinput.behavioral_snapshot",
		)
		algorithm = filled
	}
	ms := &ModelSnapshot{
		Kind:           EvaluationModelKindBehavioralRating,
		Algorithm:      string(algorithm),
		ProductChannel: string(modelcatalog.ProductChannelBehaviorAbility),
		Code:           snapshot.Code,
		Version:        version,
		Title:          snapshot.Title,
		Payload:        BehavioralRatingModelPayload{Snapshot: snapshot},
	}
	return applyPublishedRuntime(ms, snapshot.PublishedRuntime)
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
