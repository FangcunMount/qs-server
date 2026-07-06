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
	if domain.IsPersonalityTypologyPayloadFormat(snapshot.PayloadFormat) {
		return decodePayload(snapshot.Payload)
	}
	switch snapshot.Definition.Kind {
	case domain.KindMBTIMigration:
		legacy, err := decodeMBTILegacy(snapshot)
		if err != nil {
			return nil, err
		}
		return FromMBTI(legacy), nil
	case domain.KindSBTIMigration:
		legacy, err := decodeSBTILegacy(snapshot)
		if err != nil {
			return nil, err
		}
		return FromSBTI(legacy), nil
	default:
		return nil, fmt.Errorf("unsupported typology snapshot kind: %s", snapshot.Definition.Kind)
	}
}

func decodePayload(payload []byte) (*Payload, error) {
	var model Payload
	if err := json.Unmarshal(payload, &model); err != nil {
		return nil, fmt.Errorf("decode typology payload: %w", err)
	}
	return &model, nil
}

func decodeMBTILegacy(snapshot *domain.Snapshot) (*MBTILegacyModel, error) {
	format, err := resolveLegacyPayloadFormat(snapshot, domain.KindMBTIMigration, domain.PayloadFormatMBTIV1)
	if err != nil {
		return nil, err
	}
	if domain.IsPersonalityTypologyPayloadFormat(format) {
		payload, err := decodePayload(snapshot.Payload)
		if err != nil {
			return nil, err
		}
		return ToMBTI(payload)
	}
	if !domain.IsMBTIPayloadFormat(format) {
		return nil, fmt.Errorf("unsupported mbti payload format: %s", format)
	}
	var model MBTILegacyModel
	if err := json.Unmarshal(snapshot.Payload, &model); err != nil {
		return nil, fmt.Errorf("decode mbti payload: %w", err)
	}
	return &model, nil
}

func decodeSBTILegacy(snapshot *domain.Snapshot) (*SBTILegacyModel, error) {
	format, err := resolveLegacyPayloadFormat(snapshot, domain.KindSBTIMigration, domain.PayloadFormatSBTIV1)
	if err != nil {
		return nil, err
	}
	if domain.IsPersonalityTypologyPayloadFormat(format) {
		payload, err := decodePayload(snapshot.Payload)
		if err != nil {
			return nil, err
		}
		return ToSBTI(payload)
	}
	if !domain.IsSBTIPayloadFormat(format) {
		return nil, fmt.Errorf("unsupported sbti payload format: %s", format)
	}
	var model SBTILegacyModel
	if err := json.Unmarshal(snapshot.Payload, &model); err != nil {
		return nil, fmt.Errorf("decode sbti payload: %w", err)
	}
	return &model, nil
}

func resolveLegacyPayloadFormat(snapshot *domain.Snapshot, kind domain.Kind, defaultFormat string) (string, error) {
	if snapshot.PayloadFormat != "" {
		return snapshot.PayloadFormat, nil
	}
	if snapshot.Definition.Kind != "" && snapshot.Definition.Kind != kind {
		return "", fmt.Errorf("ruleset kind %s does not match decoder %s", snapshot.Definition.Kind, kind)
	}
	return defaultFormat, nil
}
