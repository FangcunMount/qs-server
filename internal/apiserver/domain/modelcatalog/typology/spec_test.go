package typology

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

func TestToRuntimeSpecFromMBTIPayload(t *testing.T) {
	payload := FromMBTI(&MBTILegacyModel{
		Code:           "MBTI_TEST",
		Version:        "1.0.0",
		DimensionOrder: []string{"EI"},
		Dimensions: map[string]MBTILegacyDimension{
			"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E"},
		},
		TypeProfiles: []MBTILegacyTypeProfile{
			{TypeCode: "INTJ", TypeName: "建筑师"},
		},
	})

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
	if spec.Report.Kind != ReportKindPersonalityType {
		t.Fatalf("Report.Kind = %s", spec.Report.Kind)
	}
	if len(spec.SpecialRules) != 0 {
		t.Fatalf("SpecialRules = %#v, want empty", spec.SpecialRules)
	}
}

func TestToRuntimeSpecFromSBTIPayload(t *testing.T) {
	payload := FromSBTI(&SBTILegacyModel{
		Code:                        "SBTI_FUN",
		Version:                     "1.0.0",
		FallbackSimilarityThreshold: 0.6,
		DimensionOrder:              []string{"D1", "D2"},
		Dimensions: map[string]SBTILegacyDimension{
			"D1": {Code: "D1", Name: "D1"},
			"D2": {Code: "D2", Name: "D2"},
		},
		NormalOutcomes: []SBTILegacyOutcome{
			{Code: "HIGH", Name: "高能者", Pattern: "HH"},
		},
		SpecialOutcomes: []SBTILegacyOutcome{
			{Code: "HHHH", Name: "傻乐者", Trigger: "fallback:best_match<60%", IsSpecial: true},
			{Code: "DRUNK", Name: "酒鬼", Trigger: "hidden:drink", IsSpecial: true},
		},
		DrinkTrigger: SBTILegacyDrinkTrigger{
			QuestionCodes: []string{"drink_gate_q2"},
			OptionValues:  []string{"C"},
		},
	})

	spec, err := payload.ToRuntimeSpec()
	if err != nil {
		t.Fatalf("ToRuntimeSpec: %v", err)
	}
	if spec.Decision.Kind != binding.DecisionKindNearestPattern {
		t.Fatalf("Decision.Kind = %s", spec.Decision.Kind)
	}
	if spec.Decision.FallbackCode != "HHHH" {
		t.Fatalf("Decision.FallbackCode = %s", spec.Decision.FallbackCode)
	}
	if len(spec.SpecialRules) < 2 {
		t.Fatalf("SpecialRules = %#v, want drink + fallback", spec.SpecialRules)
	}
	var drinkRule *SpecialRuleSpec
	for i := range spec.SpecialRules {
		if spec.SpecialRules[i].Code == "DRUNK" {
			drinkRule = &spec.SpecialRules[i]
			break
		}
	}
	if drinkRule == nil || drinkRule.Phase != SpecialRuleBeforeScore {
		t.Fatalf("drink rule = %#v, want before_score", drinkRule)
	}
	if drinkRule.Kind != SpecialRuleKindAnswerMatch {
		t.Fatalf("drink rule Kind = %s, want answer_match", drinkRule.Kind)
	}
	if len(drinkRule.ResolvedQuestionCodes()) == 0 {
		t.Fatalf("drink rule condition = %#v, want question codes", drinkRule.Condition)
	}
	var fallbackRule *SpecialRuleSpec
	for i := range spec.SpecialRules {
		if spec.SpecialRules[i].Kind == SpecialRuleKindFallbackThreshold {
			fallbackRule = &spec.SpecialRules[i]
			break
		}
	}
	if fallbackRule == nil || fallbackRule.OutcomeCode != "HHHH" {
		t.Fatalf("fallback rule = %#v, want HHHH fallback_threshold", fallbackRule)
	}
}

func TestToRuntimeSpecFromBigFivePayload(t *testing.T) {
	payload := &Payload{
		Code:           "BIGFIVE_V1",
		Version:        "1.0.0",
		Algorithm:      binding.AlgorithmBigFive,
		DimensionOrder: []string{"O"},
		Dimensions: map[string]Dimension{
			"O": {Code: "O", Name: "Openness"},
		},
		MatchingSpec: MatchingSpec{Kind: binding.DecisionKindTraitProfile},
	}

	spec, err := payload.ToRuntimeSpec()
	if err != nil {
		t.Fatalf("ToRuntimeSpec: %v", err)
	}
	if spec.Decision.Kind != binding.DecisionKindTraitProfile {
		t.Fatalf("Decision.Kind = %s", spec.Decision.Kind)
	}
	if spec.OutcomeMapping.DetailKind != OutcomeDetailTraitProfile {
		t.Fatalf("OutcomeMapping.DetailKind = %s", spec.OutcomeMapping.DetailKind)
	}
	if spec.Report.Kind != ReportKindTraitProfile {
		t.Fatalf("Report.Kind = %s", spec.Report.Kind)
	}
}
