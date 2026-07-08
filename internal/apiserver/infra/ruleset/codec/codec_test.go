package codec

import (
	"encoding/json"
	v1envelope "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/v1envelope"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
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
	snapshot := &v1envelope.V1Snapshot{
		SchemaVersion: v1envelope.RuleSetSchemaVersionV1,
		PayloadFormat: format,
		Definition: v1envelope.V1Definition{
			Kind: v1envelope.RuleSetKindSBTI,
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
	snapshot := &v1envelope.V1Snapshot{
		SchemaVersion: v1envelope.RuleSetSchemaVersionV1,
		PayloadFormat: format,
		Definition: v1envelope.V1Definition{
			Kind: v1envelope.RuleSetKindMBTI,
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

func TestDecodeRejectsLegacyFlatPayloadFormat(t *testing.T) {
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
	snapshot := &v1envelope.V1Snapshot{
		PayloadFormat: domain.PayloadFormatMBTIV1Legacy,
		Definition: v1envelope.V1Definition{
			Kind: v1envelope.RuleSetKindMBTI,
			Code: model.Code,
		},
		Payload: payload,
	}
	if _, err := DecodeMBTI(snapshot); err == nil {
		t.Fatal("expected error for legacy flat mbti payload format")
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
	snapshot := &v1envelope.V1Snapshot{
		SchemaVersion: v1envelope.RuleSetSchemaVersionV1,
		PayloadFormat: format,
		Definition: v1envelope.V1Definition{
			Kind: v1envelope.RuleSetKindScale,
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
	snapshot := &v1envelope.V1Snapshot{
		PayloadFormat: domain.PayloadFormatMBTIV1,
		Definition: v1envelope.V1Definition{
			Kind: v1envelope.RuleSetKindMBTI,
		},
		Payload: []byte(`not-json`),
	}
	if _, err := DecodeMBTI(snapshot); err == nil {
		t.Fatal("expected decode error for invalid payload")
	}
}
