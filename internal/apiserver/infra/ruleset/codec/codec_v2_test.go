package codec

import (
	"encoding/json"
	v1envelope "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/v1envelope"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func TestEncodeWritesAssessmentModelPayloadFormats(t *testing.T) {
	scalePayload, scaleFormat, err := EncodeScale(&scalesnapshot.ScaleSnapshot{Code: "PHQ9"})
	if err != nil {
		t.Fatalf("EncodeScale: %v", err)
	}
	if scaleFormat != domain.PayloadFormatAssessmentScaleV1 {
		t.Fatalf("scale format = %s", scaleFormat)
	}

	mbtiPayload, mbtiFormat, err := EncodeMBTI(&modeltypology.MBTILegacyModel{Code: "MBTI_OEJTS"})
	if err != nil {
		t.Fatalf("EncodeMBTI: %v", err)
	}
	if mbtiFormat != domain.PayloadFormatPersonalityTypologyV1 {
		t.Fatalf("mbti format = %s", mbtiFormat)
	}

	sbtiPayload, sbtiFormat, err := EncodeSBTI(&modeltypology.SBTILegacyModel{Code: "SBTI_FUN"})
	if err != nil {
		t.Fatalf("EncodeSBTI: %v", err)
	}
	if sbtiFormat != domain.PayloadFormatPersonalityTypologyV1 {
		t.Fatalf("sbti format = %s", sbtiFormat)
	}

	var decodedTypology modeltypology.Payload
	if err := json.Unmarshal(mbtiPayload, &decodedTypology); err != nil {
		t.Fatalf("unmarshal typology: %v", err)
	}
	if decodedTypology.Algorithm != domain.AlgorithmMBTI {
		t.Fatalf("typology algorithm = %s", decodedTypology.Algorithm)
	}
	_ = scalePayload
	_ = sbtiPayload
}

func TestDecodeRejectsLegacyRulesetPayloadFormats(t *testing.T) {
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
	_, err = DecodeMBTI(&v1envelope.V1Snapshot{
		PayloadFormat: domain.PayloadFormatMBTIV1Legacy,
		Definition:    v1envelope.V1Definition{Kind: v1envelope.RuleSetKindMBTI, Code: model.Code},
		Payload:       payload,
	})
	if err == nil {
		t.Fatal("expected error for legacy flat mbti payload format")
	}
}

func TestTypologyEncodeDecodeRoundTripThroughLegacyDecoder(t *testing.T) {
	legacy := &modeltypology.SBTILegacyModel{
		Code:              "SBTI_FUN",
		Version:           "1.0.0",
		QuestionnaireCode: "SBTI_FUN",
		Status:            "published",
		NormalOutcomes: []modeltypology.SBTILegacyOutcome{
			{Code: "HIGH", Name: "高能者", Pattern: "HH"},
		},
	}
	payload, format, err := EncodeSBTI(legacy)
	if err != nil {
		t.Fatalf("EncodeSBTI: %v", err)
	}
	got, err := DecodeSBTI(&v1envelope.V1Snapshot{
		SchemaVersion: v1envelope.RuleSetSchemaVersionV1,
		PayloadFormat: format,
		Definition:    v1envelope.V1Definition{Kind: v1envelope.RuleSetKindSBTI, Code: legacy.Code},
		Payload:       payload,
	})
	if err != nil {
		t.Fatalf("DecodeSBTI: %v", err)
	}
	if got.Code != legacy.Code || len(got.NormalOutcomes) != 1 {
		t.Fatalf("round trip = %#v", got)
	}
}
