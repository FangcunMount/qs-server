package publishing

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

// TypologyRuntimeSpecFromModel decodes draft model definition into runtime execution spec.
func TypologyRuntimeSpecFromModel(model *AssessmentModel) (*typology.RuntimeSpec, error) {
	_, runtime, err := TypologyPayloadAndRuntimeSpecFromModel(model)
	return runtime, err
}

// TypologyPayloadAndRuntimeSpecFromModel decodes draft definition and preserves payload-level metadata.
func TypologyPayloadAndRuntimeSpecFromModel(model *AssessmentModel) (*typology.Payload, *typology.RuntimeSpec, error) {
	if model == nil {
		return nil, nil, fmt.Errorf("assessment model is nil")
	}
	var payload typology.Payload
	if err := json.Unmarshal(model.Definition.Data, &payload); err == nil && (payload.HasExplicitRuntime() || payload.Algorithm != "" || len(payload.Dimensions) > 0) {
		if payload.Algorithm == "" {
			payload.Algorithm = model.Algorithm
		}
		runtime, err := payload.ToRuntimeSpec()
		if err != nil {
			return nil, nil, err
		}
		return &payload, runtime, nil
	}
	var runtime typology.RuntimeSpec
	if err := json.Unmarshal(model.Definition.Data, &runtime); err != nil {
		return nil, nil, fmt.Errorf("decode typology runtime spec: %w", err)
	}
	wrapped := &typology.Payload{
		Algorithm: model.Algorithm,
		Runtime:   &runtime,
	}
	resolved, err := wrapped.ToRuntimeSpec()
	if err != nil {
		return nil, nil, err
	}
	return wrapped, resolved, nil
}
