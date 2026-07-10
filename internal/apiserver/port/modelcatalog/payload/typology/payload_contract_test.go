package typology_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func TestTypologyPayloadJSONRoundTripsFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/personality_typology_v1.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	raw = bytes.TrimSpace(raw)

	var payload typology.Payload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	encoded, err := json.Marshal(&payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if !bytes.Equal(encoded, raw) {
		t.Fatalf("payload JSON = %s\nfixture JSON = %s", encoded, raw)
	}

	decoded, runtime, err := typology.PayloadAndRuntimeSpecFromDefinition(raw, binding.AlgorithmMBTI)
	if err != nil {
		t.Fatalf("PayloadAndRuntimeSpecFromDefinition: %v", err)
	}
	if decoded.Code != "MBTI_CONTRACT" || runtime.Decision.Kind != binding.DecisionKindPoleComposition {
		t.Fatalf("decoded payload/runtime = %#v / %#v", decoded, runtime)
	}
}

func TestMBTILegacyConversionRoundTrip(t *testing.T) {
	legacy := &typology.MBTILegacyModel{
		Code:                 "MBTI_LEGACY",
		Version:              "1.0.0",
		Title:                "MBTI Legacy",
		QuestionnaireCode:    "Q_MBTI",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		DimensionOrder:       []string{"EI"},
		Dimensions: map[string]typology.MBTILegacyDimension{
			"EI": {Code: "EI", Name: "E/I", LeftPole: "I", RightPole: "E"},
		},
		QuestionMappings: []typology.MBTILegacyQuestionMapping{{QuestionCode: "Q1", Dimension: "EI", Sign: 1}},
		TypeProfiles: []typology.MBTILegacyTypeProfile{{
			TypeCode: "ENFP", TypeName: "Campaigner", Summary: "summary",
		}},
	}

	back, err := typology.ToMBTI(typology.FromMBTI(legacy))
	if err != nil {
		t.Fatalf("ToMBTI: %v", err)
	}
	if back.Code != legacy.Code || len(back.TypeProfiles) != 1 || back.TypeProfiles[0].TypeCode != "ENFP" {
		t.Fatalf("round trip legacy MBTI = %#v", back)
	}
}
