package specialrule_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification/specialrule"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

func TestEngineApplyBeforeScoreMatchesDrinkTrigger(t *testing.T) {
	payload := modeltypology.FromSBTI(&modeltypology.SBTILegacyModel{
		Code:           "SBTI_FUN",
		Version:        "1.0.0",
		DimensionOrder: []string{"D1"},
		Dimensions: map[string]modeltypology.SBTILegacyDimension{
			"D1": {Code: "D1", Name: "D1", Model: "M1"},
		},
		SpecialOutcomes: []modeltypology.SBTILegacyOutcome{
			{Code: "DRUNK", Name: "酒鬼", Trigger: "hidden:drink", IsSpecial: true},
		},
		DrinkTrigger: modeltypology.SBTILegacyDrinkTrigger{
			QuestionCodes: []string{"drink_gate_q2"},
			OptionValues:  []string{"C"},
		},
	})
	spec, err := payload.ToRuntimeSpec()
	if err != nil {
		t.Fatalf("ToRuntimeSpec: %v", err)
	}

	got, ok := (specialrule.Engine{}).ApplyBeforeScore(spec.SpecialRules, payload, []classification.Answer{
		{QuestionCode: "drink_gate_q2", Value: "C"},
	})
	if !ok {
		t.Fatal("expected drink rule match")
	}
	if got.OutcomeCode != "DRUNK" || !got.SkipScoring {
		t.Fatalf("match = %#v", got)
	}
	for _, rule := range spec.SpecialRules {
		if rule.Code == "DRUNK" && rule.Kind != modeltypology.SpecialRuleKindAnswerMatch {
			t.Fatalf("DRUNK rule Kind = %s, want answer_match", rule.Kind)
		}
	}
}

func TestEngineApplyAfterDecisionUsesFallbackOutcome(t *testing.T) {
	payload := modeltypology.FromSBTI(&modeltypology.SBTILegacyModel{
		Code:                        "SBTI_FUN",
		Version:                     "1.0.0",
		FallbackSimilarityThreshold: 0.9,
		DimensionOrder:              []string{"D1"},
		Dimensions: map[string]modeltypology.SBTILegacyDimension{
			"D1": {Code: "D1", Name: "D1", Model: "M1"},
		},
		SpecialOutcomes: []modeltypology.SBTILegacyOutcome{
			{Code: "HHHH", Name: "傻乐者", Trigger: "fallback:best_match<60%", IsSpecial: true},
		},
	})
	spec, err := payload.ToRuntimeSpec()
	if err != nil {
		t.Fatalf("ToRuntimeSpec: %v", err)
	}

	got, ok := (specialrule.Engine{}).ApplyAfterDecision(spec.SpecialRules, spec.Decision, payload, 0.5)
	if !ok {
		t.Fatal("expected fallback match")
	}
	if got.OutcomeCode != "HHHH" || got.Trigger != "fallback:best_match<60%" {
		t.Fatalf("match = %#v", got)
	}
}
