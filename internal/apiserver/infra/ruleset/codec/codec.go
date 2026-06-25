package codec

import (
	"encoding/json"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset"
	rulesetmbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/mbti"
	rulesetsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/sbti"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale/snapshot"
)

func EncodeSBTI(model *rulesetsbti.ModelSnapshot) ([]byte, string, error) {
	if model == nil {
		return nil, "", fmt.Errorf("sbti model is nil")
	}
	payload, err := json.Marshal(model)
	if err != nil {
		return nil, "", fmt.Errorf("marshal sbti payload: %w", err)
	}
	return payload, domain.PayloadFormatSBTIV1, nil
}

func EncodeMBTI(model *rulesetmbti.ModelSnapshot) ([]byte, string, error) {
	if model == nil {
		return nil, "", fmt.Errorf("mbti model is nil")
	}
	payload, err := json.Marshal(model)
	if err != nil {
		return nil, "", fmt.Errorf("marshal mbti payload: %w", err)
	}
	return payload, domain.PayloadFormatMBTIV1, nil
}

func EncodeScale(model *scalesnapshot.ScaleSnapshot) ([]byte, string, error) {
	if model == nil {
		return nil, "", fmt.Errorf("scale model is nil")
	}
	payload, err := json.Marshal(model)
	if err != nil {
		return nil, "", fmt.Errorf("marshal scale payload: %w", err)
	}
	return payload, domain.PayloadFormatScaleV1, nil
}

func DecodeSBTI(snapshot *domain.RuleSetSnapshot) (*rulesetsbti.ModelSnapshot, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("ruleset snapshot is nil")
	}
	format, err := resolvePayloadFormat(snapshot, domain.RuleSetKindSBTI, domain.PayloadFormatSBTIV1)
	if err != nil {
		return nil, err
	}
	if !domain.IsSBTIPayloadFormat(format) {
		return nil, fmt.Errorf("unsupported sbti payload format: %s", format)
	}
	var model rulesetsbti.ModelSnapshot
	if err := json.Unmarshal(snapshot.Payload, &model); err != nil {
		return nil, fmt.Errorf("decode sbti payload: %w", err)
	}
	return &model, nil
}

func DecodeMBTI(snapshot *domain.RuleSetSnapshot) (*rulesetmbti.ModelSnapshot, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("ruleset snapshot is nil")
	}
	format, err := resolvePayloadFormat(snapshot, domain.RuleSetKindMBTI, domain.PayloadFormatMBTIV1)
	if err != nil {
		return nil, err
	}
	if !domain.IsMBTIPayloadFormat(format) {
		return nil, fmt.Errorf("unsupported mbti payload format: %s", format)
	}
	var model rulesetmbti.ModelSnapshot
	if err := json.Unmarshal(snapshot.Payload, &model); err != nil {
		return nil, fmt.Errorf("decode mbti payload: %w", err)
	}
	return &model, nil
}

func DecodeScale(snapshot *domain.RuleSetSnapshot) (*scalesnapshot.ScaleSnapshot, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("ruleset snapshot is nil")
	}
	format, err := resolvePayloadFormat(snapshot, domain.RuleSetKindScale, domain.PayloadFormatScaleV1)
	if err != nil {
		return nil, err
	}
	if !domain.IsScalePayloadFormat(format) {
		return nil, fmt.Errorf("unsupported scale payload format: %s", format)
	}
	var model scalesnapshot.ScaleSnapshot
	if err := json.Unmarshal(snapshot.Payload, &model); err != nil {
		return nil, fmt.Errorf("decode scale payload: %w", err)
	}
	return &model, nil
}

func resolvePayloadFormat(snapshot *domain.RuleSetSnapshot, kind domain.RuleSetKind, defaultFormat string) (string, error) {
	if snapshot.PayloadFormat != "" {
		return snapshot.PayloadFormat, nil
	}
	if snapshot.Definition.Kind != "" && snapshot.Definition.Kind != kind {
		return "", fmt.Errorf("ruleset kind %s does not match decoder %s", snapshot.Definition.Kind, kind)
	}
	return defaultFormat, nil
}
