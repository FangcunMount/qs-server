package typology_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	newtypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
	oldtypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func TestTypologyPayloadJSONShapeMatchesLegacyPayload(t *testing.T) {
	oldPayload := &oldtypology.Payload{
		Code:                 "MBTI_CONTRACT",
		Version:              "1.0.0",
		Title:                "MBTI Contract",
		QuestionnaireCode:    "Q_MBTI",
		QuestionnaireVersion: "2.0.0",
		Status:               "published",
		Algorithm:            binding.AlgorithmMBTI,
		DimensionOrder:       []string{"EI"},
		Dimensions: map[string]oldtypology.Dimension{
			"EI": {Code: "EI", Name: "Extraversion", LeftPole: "I", RightPole: "E"},
		},
		QuestionMappings: []oldtypology.QuestionMapping{{
			QuestionCode: "Q1",
			Dimension:    "EI",
			Sign:         1,
		}},
		Outcomes: []oldtypology.Outcome{{
			Code:     "ENFP",
			Name:     "Campaigner",
			OneLiner: "warm",
			Summary:  "summary",
		}},
		MatchingSpec: oldtypology.MatchingSpec{Kind: binding.DecisionKindPoleComposition},
		Runtime: &oldtypology.RuntimeSpec{
			FactorGraph: oldtypology.FactorGraphSpec{
				DimensionOrder: []string{"EI"},
				Dimensions: map[string]oldtypology.Dimension{
					"EI": {Code: "EI", Name: "Extraversion"},
				},
				QuestionMappings: []oldtypology.QuestionMapping{{QuestionCode: "Q1", Dimension: "EI", Sign: 1}},
			},
			Decision:       oldtypology.PersonalityDecisionSpec{Kind: binding.DecisionKindPoleComposition},
			OutcomeMapping: oldtypology.OutcomeMappingSpec{DetailKind: oldtypology.OutcomeDetailPersonalityType},
			Report:         oldtypology.ReportSpec{Kind: oldtypology.ReportKindPersonalityType, AdapterKey: oldtypology.ReportAdapterMBTI},
		},
	}
	newPayload := &newtypology.Payload{
		Code:                 "MBTI_CONTRACT",
		Version:              "1.0.0",
		Title:                "MBTI Contract",
		QuestionnaireCode:    "Q_MBTI",
		QuestionnaireVersion: "2.0.0",
		Status:               "published",
		Algorithm:            binding.AlgorithmMBTI,
		DimensionOrder:       []string{"EI"},
		Dimensions: map[string]newtypology.Dimension{
			"EI": {Code: "EI", Name: "Extraversion", LeftPole: "I", RightPole: "E"},
		},
		QuestionMappings: []newtypology.QuestionMapping{{
			QuestionCode: "Q1",
			Dimension:    "EI",
			Sign:         1,
		}},
		Outcomes: []newtypology.Outcome{{
			Code:     "ENFP",
			Name:     "Campaigner",
			OneLiner: "warm",
			Summary:  "summary",
		}},
		MatchingSpec: newtypology.MatchingSpec{Kind: binding.DecisionKindPoleComposition},
		Runtime: &newtypology.RuntimeSpec{
			FactorGraph: newtypology.FactorGraphSpec{
				DimensionOrder: []string{"EI"},
				Dimensions: map[string]newtypology.Dimension{
					"EI": {Code: "EI", Name: "Extraversion"},
				},
				QuestionMappings: []newtypology.QuestionMapping{{QuestionCode: "Q1", Dimension: "EI", Sign: 1}},
			},
			Decision:       newtypology.PersonalityDecisionSpec{Kind: binding.DecisionKindPoleComposition},
			OutcomeMapping: newtypology.OutcomeMappingSpec{DetailKind: newtypology.OutcomeDetailPersonalityType},
			Report:         newtypology.ReportSpec{Kind: newtypology.ReportKindPersonalityType, AdapterKey: newtypology.ReportAdapterMBTI},
		},
	}

	oldBytes, err := json.Marshal(oldPayload)
	if err != nil {
		t.Fatalf("marshal legacy typology payload: %v", err)
	}
	newBytes, err := json.Marshal(newPayload)
	if err != nil {
		t.Fatalf("marshal new typology payload: %v", err)
	}
	if !bytes.Equal(newBytes, oldBytes) {
		t.Fatalf("new payload JSON = %s\nlegacy JSON = %s", newBytes, oldBytes)
	}

	decoded, runtime, err := newtypology.PayloadAndRuntimeSpecFromDefinition(newBytes, binding.AlgorithmMBTI)
	if err != nil {
		t.Fatalf("PayloadAndRuntimeSpecFromDefinition: %v", err)
	}
	if decoded.Code != "MBTI_CONTRACT" || runtime.Decision.Kind != binding.DecisionKindPoleComposition {
		t.Fatalf("decoded payload/runtime = %#v / %#v", decoded, runtime)
	}
}

func TestTypologyLegacyMBTIConversionMatchesLegacyPackage(t *testing.T) {
	legacy := &oldtypology.MBTILegacyModel{
		Code:                 "MBTI_LEGACY",
		Version:              "1.0.0",
		Title:                "MBTI Legacy",
		QuestionnaireCode:    "Q_MBTI",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		DimensionOrder:       []string{"EI"},
		Dimensions: map[string]oldtypology.MBTILegacyDimension{
			"EI": {Code: "EI", Name: "E/I", LeftPole: "I", RightPole: "E"},
		},
		QuestionMappings: []oldtypology.MBTILegacyQuestionMapping{{QuestionCode: "Q1", Dimension: "EI", Sign: 1}},
		TypeProfiles: []oldtypology.MBTILegacyTypeProfile{{
			TypeCode: "ENFP",
			TypeName: "Campaigner",
			Summary:  "summary",
		}},
	}
	oldUnified := oldtypology.FromMBTI(legacy)
	newUnified := newtypology.FromMBTI((*newtypology.MBTILegacyModel)(legacy))
	if !reflect.DeepEqual(newUnified, oldUnified) {
		t.Fatalf("new unified = %#v\nlegacy unified = %#v", newUnified, oldUnified)
	}

	back, err := newtypology.ToMBTI(newUnified)
	if err != nil {
		t.Fatalf("ToMBTI: %v", err)
	}
	if back.Code != legacy.Code || len(back.TypeProfiles) != 1 || back.TypeProfiles[0].TypeCode != "ENFP" {
		t.Fatalf("round trip legacy MBTI = %#v", back)
	}
}
