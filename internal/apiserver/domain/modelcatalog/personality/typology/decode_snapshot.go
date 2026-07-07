package typology

import (
	"encoding/json"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// DecodeFromSnapshot decodes a published snapshot into a typology payload.
func DecodeFromSnapshot(snapshot *domain.Snapshot) (*Payload, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("ruleset snapshot is nil")
	}
	format := snapshot.PayloadFormat
	if format == "" && snapshot.Definition.Kind == domain.KindPersonality {
		format = domain.PayloadFormatPersonalityTypologyV1
	}
	if !domain.IsPersonalityTypologyPayloadFormat(format) {
		return nil, fmt.Errorf("unsupported typology snapshot: kind=%s format=%s", snapshot.Definition.Kind, snapshot.PayloadFormat)
	}
	return decodePayload(snapshot.Payload)
}

func decodePayload(payload []byte) (*Payload, error) {
	var model Payload
	if err := json.Unmarshal(payload, &model); err != nil {
		return nil, fmt.Errorf("decode typology payload: %w", err)
	}
	return &model, nil
}
