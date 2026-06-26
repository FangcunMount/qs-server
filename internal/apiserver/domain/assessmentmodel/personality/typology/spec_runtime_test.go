package typology

import (
	"encoding/json"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
)

func TestToRuntimeSpecPrefersExplicitRuntimeOverAlgorithmDerivation(t *testing.T) {
	payload := &Payload{
		Code:           "CUSTOM_V1",
		Version:        "1.0.0",
		Algorithm:      assessmentmodel.AlgorithmMBTI,
		DimensionOrder: []string{"EI"},
		Dimensions: map[string]Dimension{
			"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E"},
		},
		Runtime: &RuntimeSpec{
			Decision: PersonalityDecisionSpec{
				Kind: assessmentmodel.DecisionKindTraitProfile,
			},
			OutcomeMapping: OutcomeMappingSpec{
				DetailKind: OutcomeDetailTraitProfile,
			},
			Report: ReportSpec{
				Kind:          ReportKindTraitProfile,
				CategoryLabel: "Custom Trait",
			},
		},
	}

	spec, err := payload.ToRuntimeSpec()
	if err != nil {
		t.Fatalf("ToRuntimeSpec: %v", err)
	}
	if spec.Decision.Kind != assessmentmodel.DecisionKindTraitProfile {
		t.Fatalf("Decision.Kind = %s, want trait_profile", spec.Decision.Kind)
	}
	if spec.OutcomeMapping.DetailKind != OutcomeDetailTraitProfile {
		t.Fatalf("OutcomeMapping.DetailKind = %s", spec.OutcomeMapping.DetailKind)
	}
	if spec.Report.CategoryLabel != "Custom Trait" {
		t.Fatalf("Report.CategoryLabel = %q", spec.Report.CategoryLabel)
	}
}

func TestToRuntimeSpecWithoutAlgorithmWhenRuntimeExplicit(t *testing.T) {
	payload := explicitPoleCompositionPayload()
	payload.Algorithm = ""

	spec, err := payload.ToRuntimeSpec()
	if err != nil {
		t.Fatalf("ToRuntimeSpec: %v", err)
	}
	if spec.Decision.Kind != assessmentmodel.DecisionKindPoleComposition {
		t.Fatalf("Decision.Kind = %s", spec.Decision.Kind)
	}
	if spec.OutcomeMapping.DetailKind != OutcomeDetailPersonalityType {
		t.Fatalf("OutcomeMapping.DetailKind = %s", spec.OutcomeMapping.DetailKind)
	}
	if spec.OutcomeMapping.Algorithm != "" {
		t.Fatalf("OutcomeMapping.Algorithm = %q, want empty for explicit runtime", spec.OutcomeMapping.Algorithm)
	}
}

func TestRuntimeSpecJSONRoundTripPreservesExplicitConfig(t *testing.T) {
	payload := explicitPoleCompositionPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded Payload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !decoded.HasExplicitRuntime() {
		t.Fatal("decoded payload missing explicit runtime")
	}
	spec, err := decoded.ToRuntimeSpec()
	if err != nil {
		t.Fatalf("ToRuntimeSpec: %v", err)
	}
	if spec.Report.CategoryLabel != "Custom Pole Model" {
		t.Fatalf("Report.CategoryLabel = %q", spec.Report.CategoryLabel)
	}
}

func explicitPoleCompositionPayload() *Payload {
	return &Payload{
		Code:                 "CUSTOM_POLE_V1",
		Version:              "1.0.0",
		QuestionnaireCode:    "CUSTOM_POLE_V1",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		DimensionOrder:       []string{"EI", "SN", "TF", "JP"},
		Dimensions: map[string]Dimension{
			"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E", Constant: 24, Threshold: 24},
			"SN": {Code: "SN", Name: "感觉-直觉", LeftPole: "S", RightPole: "N", Constant: 24, Threshold: 24},
			"TF": {Code: "TF", Name: "思考-情感", LeftPole: "T", RightPole: "F", Constant: 24, Threshold: 24},
			"JP": {Code: "JP", Name: "判断-知觉", LeftPole: "J", RightPole: "P", Constant: 24, Threshold: 24},
		},
		QuestionMappings: []QuestionMapping{
			{QuestionCode: "Q_EI", Dimension: "EI", Sign: -1},
			{QuestionCode: "Q_SN", Dimension: "SN", Sign: 1},
			{QuestionCode: "Q_TF", Dimension: "TF", Sign: -1},
			{QuestionCode: "Q_JP", Dimension: "JP", Sign: -1},
		},
		Outcomes: []Outcome{
			{Code: "INTJ", Name: "建筑师", OneLiner: "独立战略家"},
		},
		Runtime: &RuntimeSpec{
			FactorGraph: FactorGraphSpec{
				DimensionOrder: []string{"EI", "SN", "TF", "JP"},
				Dimensions: map[string]Dimension{
					"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E", Constant: 24, Threshold: 24},
					"SN": {Code: "SN", Name: "感觉-直觉", LeftPole: "S", RightPole: "N", Constant: 24, Threshold: 24},
					"TF": {Code: "TF", Name: "思考-情感", LeftPole: "T", RightPole: "F", Constant: 24, Threshold: 24},
					"JP": {Code: "JP", Name: "判断-知觉", LeftPole: "J", RightPole: "P", Constant: 24, Threshold: 24},
				},
				QuestionMappings: []QuestionMapping{
					{QuestionCode: "Q_EI", Dimension: "EI", Sign: -1},
					{QuestionCode: "Q_SN", Dimension: "SN", Sign: 1},
					{QuestionCode: "Q_TF", Dimension: "TF", Sign: -1},
					{QuestionCode: "Q_JP", Dimension: "JP", Sign: -1},
				},
			},
			Decision: PersonalityDecisionSpec{
				Kind: assessmentmodel.DecisionKindPoleComposition,
			},
			OutcomeMapping: OutcomeMappingSpec{
				DetailKind: OutcomeDetailPersonalityType,
			},
			Report: ReportSpec{
				Kind:          ReportKindPersonalityType,
				CategoryLabel: "Custom Pole Model",
			},
		},
	}
}
