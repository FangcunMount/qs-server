package codec

import (
	"encoding/json"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset"
	evaluationinputPort "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func TestSBTICodecRoundTrip(t *testing.T) {
	model := &evaluationinputPort.SBTIModelSnapshot{
		Code:             "SBTI_FUN",
		Version:          "1.0.0",
		QuestionnaireCode: "SBTI_FUN",
		Status:           "published",
	}
	payload, format, err := EncodeSBTI(model)
	if err != nil {
		t.Fatalf("EncodeSBTI: %v", err)
	}
	snapshot := &domain.RuleSetSnapshot{
		SchemaVersion: domain.RuleSetSchemaVersionV1,
		PayloadFormat: format,
		Definition: domain.RuleSetDefinition{
			Kind: domain.RuleSetKindSBTI,
			Code: model.Code,
		},
		Payload: payload,
	}
	got, err := DecodeSBTI(snapshot)
	if err != nil {
		t.Fatalf("DecodeSBTI: %v", err)
	}
	if got.Code != model.Code {
		t.Fatalf("Code = %s, want %s", got.Code, model.Code)
	}
}

func TestMBTICodecRoundTrip(t *testing.T) {
	model := &evaluationinputPort.MBTIModelSnapshot{
		Code:             "MBTI_OEJTS",
		Version:          "1.0.0",
		QuestionnaireCode: "MBTI_OEJTS",
		Status:           "published",
	}
	payload, format, err := EncodeMBTI(model)
	if err != nil {
		t.Fatalf("EncodeMBTI: %v", err)
	}
	snapshot := &domain.RuleSetSnapshot{
		SchemaVersion: domain.RuleSetSchemaVersionV1,
		PayloadFormat: format,
		Definition: domain.RuleSetDefinition{
			Kind: domain.RuleSetKindMBTI,
			Code: model.Code,
		},
		Payload: payload,
	}
	got, err := DecodeMBTI(snapshot)
	if err != nil {
		t.Fatalf("DecodeMBTI: %v", err)
	}
	if got.Code != model.Code {
		t.Fatalf("Code = %s, want %s", got.Code, model.Code)
	}
}

func TestDecodeAcceptsLegacyPayloadFormat(t *testing.T) {
	model := &evaluationinputPort.MBTIModelSnapshot{
		Code:              "MBTI_OEJTS",
		Version:           "1.0.0",
		QuestionnaireCode: "MBTI_OEJTS",
		Status:            "published",
	}
	payload, err := json.Marshal(model)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	snapshot := &domain.RuleSetSnapshot{
		PayloadFormat: domain.PayloadFormatMBTIV1Legacy,
		Definition: domain.RuleSetDefinition{
			Kind: domain.RuleSetKindMBTI,
			Code: model.Code,
		},
		Payload: payload,
	}
	got, err := DecodeMBTI(snapshot)
	if err != nil {
		t.Fatalf("DecodeMBTI legacy: %v", err)
	}
	if got.Code != model.Code {
		t.Fatalf("Code = %s", got.Code)
	}
}

func TestDecodeRejectsInvalidPayload(t *testing.T) {
	snapshot := &domain.RuleSetSnapshot{
		PayloadFormat: domain.PayloadFormatMBTIV1,
		Definition: domain.RuleSetDefinition{
			Kind: domain.RuleSetKindMBTI,
		},
		Payload: []byte(`not-json`),
	}
	if _, err := DecodeMBTI(snapshot); err == nil {
		t.Fatal("expected decode error for invalid payload")
	}
}
