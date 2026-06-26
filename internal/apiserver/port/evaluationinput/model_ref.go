package evaluationinput

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

func (r ModelRef) EvaluatorKey() evaldomain.EvaluatorKey {
	if r.Algorithm != "" {
		return evaldomain.EvaluatorKey{
			Kind:      assessmentmodel.Kind(r.Kind),
			SubKind:   assessmentmodel.SubKind(r.SubKind),
			Algorithm: assessmentmodel.Algorithm(r.Algorithm),
		}
	}
	if key, ok := evaldomain.EvaluatorKeyFromLegacyKind(assessmentmodel.Kind(r.Kind)); ok {
		return key
	}
	return evaldomain.EvaluatorKey{Kind: assessmentmodel.Kind(r.Kind)}
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
