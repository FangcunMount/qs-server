package typology

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestFromMBTIPreservesTypeProfilesAsOutcomes(t *testing.T) {
	legacy := &MBTILegacyModel{
		Code:           "MBTI_TEST",
		Version:        "1.0.0",
		DimensionOrder: []string{"EI"},
		Dimensions: map[string]MBTILegacyDimension{
			"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E"},
		},
		TypeProfiles: []MBTILegacyTypeProfile{
			{TypeCode: "INTJ", TypeName: "建筑师", Summary: "善于长远规划"},
		},
	}

	got := FromMBTI(legacy)
	if got.Algorithm != modelcatalog.AlgorithmMBTI {
		t.Fatalf("Algorithm = %s, want mbti", got.Algorithm)
	}
	if got.MatchingSpec.Kind != modelcatalog.DecisionKindPoleComposition {
		t.Fatalf("MatchingSpec.Kind = %s", got.MatchingSpec.Kind)
	}
	if len(got.Outcomes) != 1 || got.Outcomes[0].Code != "INTJ" {
		t.Fatalf("Outcomes = %#v", got.Outcomes)
	}

	roundTrip, err := ToMBTI(got)
	if err != nil {
		t.Fatalf("ToMBTI: %v", err)
	}
	if roundTrip.Code != legacy.Code || roundTrip.TypeProfiles[0].TypeCode != "INTJ" {
		t.Fatalf("round trip = %#v", roundTrip)
	}
}

func TestFromSBTIMergesNormalAndSpecialOutcomes(t *testing.T) {
	legacy := &SBTILegacyModel{
		Code:                        "SBTI_FUN",
		Version:                     "1.0.0",
		FallbackSimilarityThreshold: 0.6,
		NormalOutcomes: []SBTILegacyOutcome{
			{Code: "HIGH", Name: "高能者", Pattern: "HH"},
		},
		SpecialOutcomes: []SBTILegacyOutcome{
			{Code: "DRUNK", Name: "酒鬼", Trigger: "hidden:drink", IsSpecial: true},
		},
		DrinkTrigger: SBTILegacyDrinkTrigger{
			QuestionCodes: []string{"drink_gate_q2"},
			OptionValues:  []string{"C"},
		},
	}

	got := FromSBTI(legacy)
	if got.Algorithm != modelcatalog.AlgorithmSBTI {
		t.Fatalf("Algorithm = %s, want sbti", got.Algorithm)
	}
	if got.MatchingSpec.Kind != modelcatalog.DecisionKindNearestPattern {
		t.Fatalf("MatchingSpec.Kind = %s", got.MatchingSpec.Kind)
	}
	if len(got.Outcomes) != 2 {
		t.Fatalf("Outcomes = %#v", got.Outcomes)
	}

	roundTrip, err := ToSBTI(got)
	if err != nil {
		t.Fatalf("ToSBTI: %v", err)
	}
	if len(roundTrip.NormalOutcomes) != 1 || len(roundTrip.SpecialOutcomes) != 1 {
		t.Fatalf("round trip outcomes normal=%d special=%d", len(roundTrip.NormalOutcomes), len(roundTrip.SpecialOutcomes))
	}
	if len(roundTrip.DrinkTrigger.QuestionCodes) != 1 {
		t.Fatalf("DrinkTrigger = %#v", roundTrip.DrinkTrigger)
	}
}
