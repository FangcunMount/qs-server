package evaluationinput

import (
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

func (r ModelRef) ExecutionIdentity() evaldomain.ExecutionIdentity {
	if r.Algorithm != "" {
		id := evaldomain.ExecutionIdentity{
			Kind:      modelcatalog.Kind(r.Kind),
			SubKind:   modelcatalog.SubKind(r.SubKind),
			Algorithm: modelcatalog.Algorithm(r.Algorithm),
		}
		return evaldomain.ResolveBehavioralRatingExecutorIdentity(id)
	}
	if id, ok := evaldomain.ExecutionIdentityFromLegacyKind(modelcatalog.Kind(r.Kind)); ok {
		return id
	}
	if modelcatalog.Kind(r.Kind) == modelcatalog.KindBehavioralRating && r.Algorithm == "" {
		return evaldomain.ExecutionIdentityBehavioralRatingDefault
	}
	return evaldomain.ExecutionIdentity{Kind: modelcatalog.Kind(r.Kind)}
}

func TypologyPayload(input *InputSnapshot) (*typology.Payload, bool) {
	if input == nil {
		return nil, false
	}
	if payload, ok := input.ModelPayload.(TypologyModelPayload); ok && payload.Payload != nil {
		return payload.Payload, true
	}
	if input.Model != nil {
		if payload, ok := input.Model.Payload.(TypologyModelPayload); ok && payload.Payload != nil {
			return payload.Payload, true
		}
	}
	return nil, false
}
