package evaluationinput

import (
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func (r ModelRef) ExecutionIdentity() evaldomain.ExecutionIdentity {
	return evaldomain.ExecutionIdentity{
		Kind:      modelcatalog.Kind(r.Kind),
		SubKind:   modelcatalog.SubKind(r.SubKind),
		Algorithm: modelcatalog.Algorithm(r.Algorithm),
	}
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
