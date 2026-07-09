package typology

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

// PayloadAndRuntimeSpecFromDefinition decodes draft definition bytes into the
// payload envelope plus the resolved runtime execution spec.
func PayloadAndRuntimeSpecFromDefinition(data []byte, defaultAlgorithm binding.Algorithm) (*Payload, *RuntimeSpec, error) {
	var payload Payload
	if err := json.Unmarshal(data, &payload); err == nil && (payload.HasExplicitRuntime() || payload.Algorithm != "" || len(payload.Dimensions) > 0) {
		if payload.Algorithm == "" {
			payload.Algorithm = defaultAlgorithm
		}
		runtime, err := payload.ToRuntimeSpec()
		if err != nil {
			return nil, nil, err
		}
		return &payload, runtime, nil
	}
	var runtime RuntimeSpec
	if err := json.Unmarshal(data, &runtime); err != nil {
		return nil, nil, fmt.Errorf("decode typology runtime spec: %w", err)
	}
	wrapped := &Payload{
		Algorithm: defaultAlgorithm,
		Runtime:   &runtime,
	}
	resolved, err := wrapped.ToRuntimeSpec()
	if err != nil {
		return nil, nil, err
	}
	return wrapped, resolved, nil
}
