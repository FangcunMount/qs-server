package codec

import (
	"encoding/json"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	rulesetv1 "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/v1envelope"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func EncodeSBTI(model *typology.SBTILegacyModel) ([]byte, string, error) {
	return EncodeTypology(typology.FromSBTI(model))
}

func EncodeMBTI(model *typology.MBTILegacyModel) ([]byte, string, error) {
	return EncodeTypology(typology.FromMBTI(model))
}

func EncodeTypology(model *typology.Payload) ([]byte, string, error) {
	if model == nil {
		return nil, "", fmt.Errorf("typology model is nil")
	}
	payload, err := json.Marshal(model)
	if err != nil {
		return nil, "", fmt.Errorf("marshal typology payload: %w", err)
	}
	return payload, domain.PayloadFormatPersonalityTypologyV1, nil
}

func EncodeScale(model *scalesnapshot.ScaleSnapshot) ([]byte, string, error) {
	if model == nil {
		return nil, "", fmt.Errorf("scale model is nil")
	}
	payload, err := json.Marshal(model)
	if err != nil {
		return nil, "", fmt.Errorf("marshal scale payload: %w", err)
	}
	return payload, domain.PayloadFormatAssessmentScaleV1, nil
}

func DecodeSBTI(snapshot *rulesetv1.V1Snapshot) (*typology.SBTILegacyModel, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("ruleset snapshot is nil")
	}
	format := snapshot.PayloadFormat
	if format == "" {
		format = domain.PayloadFormatPersonalityTypologyV1
	}
	if !domain.IsPersonalityTypologyPayloadFormat(format) {
		return nil, fmt.Errorf("unsupported sbti payload format: %s", format)
	}
	payload, err := decodeTypologyPayload(snapshot.Payload)
	if err != nil {
		return nil, err
	}
	return typology.ToSBTI(payload)
}

func DecodeMBTI(snapshot *rulesetv1.V1Snapshot) (*typology.MBTILegacyModel, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("ruleset snapshot is nil")
	}
	format := snapshot.PayloadFormat
	if format == "" {
		format = domain.PayloadFormatPersonalityTypologyV1
	}
	if !domain.IsPersonalityTypologyPayloadFormat(format) {
		return nil, fmt.Errorf("unsupported mbti payload format: %s", format)
	}
	payload, err := decodeTypologyPayload(snapshot.Payload)
	if err != nil {
		return nil, err
	}
	return typology.ToMBTI(payload)
}

func DecodeScale(snapshot *rulesetv1.V1Snapshot) (*scalesnapshot.ScaleSnapshot, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("ruleset snapshot is nil")
	}
	format, err := resolvePayloadFormat(snapshot, domain.KindScale, domain.PayloadFormatScaleV1)
	if err != nil {
		return nil, err
	}
	if !domain.IsScalePayloadFormat(format) {
		return nil, fmt.Errorf("unsupported scale payload format: %s", format)
	}
	return scalesnapshot.ParsePublishedPayload(snapshot.Payload)
}

func DecodeTypology(model *port.PublishedModel) (*typology.Payload, error) {
	if model == nil {
		return nil, fmt.Errorf("published model snapshot is nil")
	}
	format := model.PayloadFormat
	if format == "" {
		format = domain.PayloadFormatPersonalityTypologyV1
	}
	if !domain.IsPersonalityTypologyPayloadFormat(format) {
		return nil, fmt.Errorf("unsupported typology payload format: %s", format)
	}
	return decodeTypologyPayload(model.Payload)
}

func decodeTypologyPayload(payload []byte) (*typology.Payload, error) {
	var model typology.Payload
	if err := json.Unmarshal(payload, &model); err != nil {
		return nil, fmt.Errorf("decode typology payload: %w", err)
	}
	return &model, nil
}

func resolvePayloadFormat(snapshot *rulesetv1.V1Snapshot, kind domain.Kind, defaultFormat string) (string, error) {
	if snapshot.PayloadFormat != "" {
		return snapshot.PayloadFormat, nil
	}
	if snapshot.Definition.Kind != "" && snapshot.Definition.Kind != kind {
		return "", fmt.Errorf("ruleset kind %s does not match decoder %s", snapshot.Definition.Kind, kind)
	}
	return defaultFormat, nil
}
