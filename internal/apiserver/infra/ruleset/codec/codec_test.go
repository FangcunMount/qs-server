package codec

import (
	"encoding/json"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
)

func TestSBTICodecRoundTrip(t *testing.T) {
	model := &modeltypology.SBTILegacyModel{
		Code:              "SBTI_FUN",
		Version:           "1.0.0",
		QuestionnaireCode: "SBTI_FUN",
		Status:            "published",
	}
	payload, format, err := EncodeSBTI(model)
	if err != nil {
		t.Fatalf("EncodeSBTI: %v", err)
	}
	if format != domain.PayloadFormatPersonalityTypologyV1 {
		t.Fatalf("format = %s, want %s", format, domain.PayloadFormatPersonalityTypologyV1)
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
	model := &modeltypology.MBTILegacyModel{
		Code:              "MBTI_OEJTS",
		Version:           "1.0.0",
		QuestionnaireCode: "MBTI_OEJTS",
		Status:            "published",
	}
	payload, format, err := EncodeMBTI(model)
	if err != nil {
		t.Fatalf("EncodeMBTI: %v", err)
	}
	if format != domain.PayloadFormatPersonalityTypologyV1 {
		t.Fatalf("format = %s, want %s", format, domain.PayloadFormatPersonalityTypologyV1)
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
	model := &modeltypology.MBTILegacyModel{
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

func TestScaleCodecRoundTrip(t *testing.T) {
	model := &scalesnapshot.ScaleSnapshot{
		Code:         "PHQ9",
		ScaleVersion: "1.0.0",
		Status:       "published",
		Factors: []scalesnapshot.FactorSnapshot{
			{Code: "total", IsTotalScore: true},
		},
	}
	payload, format, err := EncodeScale(model)
	if err != nil {
		t.Fatalf("EncodeScale: %v", err)
	}
	if format != domain.PayloadFormatAssessmentScaleV1 {
		t.Fatalf("format = %s, want %s", format, domain.PayloadFormatAssessmentScaleV1)
	}
	snapshot := &domain.RuleSetSnapshot{
		SchemaVersion: domain.RuleSetSchemaVersionV1,
		PayloadFormat: format,
		Definition: domain.RuleSetDefinition{
			Kind: domain.RuleSetKindScale,
			Code: model.Code,
		},
		Payload: payload,
	}
	got, err := DecodeScale(snapshot)
	if err != nil {
		t.Fatalf("DecodeScale: %v", err)
	}
	if got.Code != model.Code {
		t.Fatalf("Code = %s, want %s", got.Code, model.Code)
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
