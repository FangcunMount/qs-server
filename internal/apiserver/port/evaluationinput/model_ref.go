package evaluationinput

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

func (r ModelRef) EvaluatorKey() evaldomain.EvaluatorKey {
	if r.Algorithm != "" {
		return evaldomain.EvaluatorKey{
			Kind:      modelcatalog.Kind(r.Kind),
			SubKind:   modelcatalog.SubKind(r.SubKind),
			Algorithm: modelcatalog.Algorithm(r.Algorithm),
		}
	}
	if key, ok := evaldomain.EvaluatorKeyFromLegacyKind(modelcatalog.Kind(r.Kind)); ok {
		return key
	}
	return evaldomain.EvaluatorKey{Kind: modelcatalog.Kind(r.Kind)}
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
