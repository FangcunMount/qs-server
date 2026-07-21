package typology

import (
	"encoding/json"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

func TestToRuntimeSpecUsesDefinitionV2Materialization(t *testing.T) {
	payload := &Payload{
		Code:      "CUSTOM_V1",
		Version:   "1.0.0",
		Algorithm: binding.AlgorithmPersonalityTypology,
		Runtime: &RuntimeSpec{
			FactorGraph: FactorGraphSpec{
				Factors: map[string]FactorSpec{
					"EI": {ID: "EI", Code: "EI", Name: "外向-内向", Kind: FactorSpecKindLeaf, Contributions: []FactorContributionSpec{{QuestionCode: "q1", ScoringMode: QuestionScoringModeQuestionScore, Sign: 1, Weight: 1}}},
				},
				Roots:      []string{"EI"},
				Dimensions: map[string]Dimension{"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E"}},
			},
			Decision: PersonalityDecisionSpec{
				Kind: binding.DecisionKindTraitProfile,
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
	if spec.Decision.Kind != binding.DecisionKindTraitProfile {
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
	if spec.Decision.Kind != binding.DecisionKindPoleComposition {
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

func TestRuntimeSpecTemplateReportRequiresAdapterKey(t *testing.T) {
	payload := explicitPoleCompositionPayload()
	payload.Runtime.Report = ReportSpec{Kind: ReportKindTemplate, TemplateID: "custom-template"}

	_, err := payload.ToRuntimeSpec()
	if err == nil {
		t.Fatal("ToRuntimeSpec error = nil, want template adapter key error")
	}
}

func TestRuntimeSpecRejectsUnsupportedSpecialRulePhase(t *testing.T) {
	payload := explicitPoleCompositionPayload()
	payload.Runtime.SpecialRules = []SpecialRuleSpec{{
		Code:  "before-decision",
		Kind:  SpecialRuleKindAnswerMatch,
		Phase: SpecialRuleBeforeDecision,
	}}

	_, err := payload.ToRuntimeSpec()
	if err == nil {
		t.Fatal("ToRuntimeSpec error = nil, want unsupported special rule phase")
	}
}

func explicitPoleCompositionPayload() *Payload {
	return &Payload{
		Code:                 "CUSTOM_POLE_V1",
		Version:              "1.0.0",
		QuestionnaireCode:    "CUSTOM_POLE_V1",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		Outcomes: []Outcome{
			{Code: "INTJ", Name: "建筑师", OneLiner: "独立战略家"},
		},
		Runtime: &RuntimeSpec{
			FactorGraph: FactorGraphSpec{
				Factors: map[string]FactorSpec{
					"EI": explicitLeaf("EI", "外向-内向", "Q_EI", -1),
					"SN": explicitLeaf("SN", "感觉-直觉", "Q_SN", 1),
					"TF": explicitLeaf("TF", "思考-情感", "Q_TF", -1),
					"JP": explicitLeaf("JP", "判断-知觉", "Q_JP", -1),
				},
				Roots: []string{"EI", "SN", "TF", "JP"},
				Dimensions: map[string]Dimension{
					"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E", Constant: 24, Threshold: 24},
					"SN": {Code: "SN", Name: "感觉-直觉", LeftPole: "S", RightPole: "N", Constant: 24, Threshold: 24},
					"TF": {Code: "TF", Name: "思考-情感", LeftPole: "T", RightPole: "F", Constant: 24, Threshold: 24},
					"JP": {Code: "JP", Name: "判断-知觉", LeftPole: "J", RightPole: "P", Constant: 24, Threshold: 24},
				},
			},
			Decision: PersonalityDecisionSpec{
				Kind: binding.DecisionKindPoleComposition,
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

func explicitLeaf(code, name, question string, sign float64) FactorSpec {
	return FactorSpec{
		ID: code, Code: code, Name: name, Kind: FactorSpecKindLeaf, Constant: 24,
		Contributions: []FactorContributionSpec{{QuestionCode: question, ScoringMode: QuestionScoringModeQuestionScore, Sign: sign, Weight: 1}},
	}
}
