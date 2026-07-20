package evaluationinput

import (
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func (r ModelRef) ExecutionIdentity() evaldomain.ExecutionIdentity {
	kind := modelcatalog.Kind(r.Kind)
	if r.Algorithm != "" {
		return evaldomain.ExecutionIdentity{
			Kind:      kind,
			SubKind:   modelcatalog.SubKind(r.SubKind),
			Algorithm: modelcatalog.Algorithm(r.Algorithm),
		}
	}
	if id, ok := evaldomain.ExecutionIdentityFromLegacyKind(kind); ok {
		return id
	}
	return evaldomain.ExecutionIdentity{Kind: kind, SubKind: modelcatalog.SubKind(r.SubKind)}
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
