package legacy

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

const personalityTypologyPayloadFormatV1 = "assessmentmodel.personality.typology.v1"

// DecodeTypologyFromSnapshot decodes a legacy ruleset snapshot into typology payload.
func DecodeTypologyFromSnapshot(snapshot *Snapshot) (*typology.Payload, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("ruleset snapshot is nil")
	}
	format := snapshot.PayloadFormat
	if format == "" && snapshot.Definition.Kind == binding.KindPersonality {
		format = personalityTypologyPayloadFormatV1
	}
	if format != personalityTypologyPayloadFormatV1 {
		return nil, fmt.Errorf("unsupported typology snapshot: kind=%s format=%s", snapshot.Definition.Kind, snapshot.PayloadFormat)
	}
	var model typology.Payload
	if err := json.Unmarshal(snapshot.Payload, &model); err != nil {
		return nil, fmt.Errorf("decode typology payload: %w", err)
	}
	return &model, nil
}
