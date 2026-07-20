package scale

import (
	"reflect"
	"testing"
)

func TestFindFactor(t *testing.T) {
	scale := &ScaleSnapshot{
		Factors: []FactorSnapshot{{Code: "F1"}},
	}
	got, ok := scale.FindFactor("F1")
	if !ok || got.Code != "F1" {
		t.Fatalf("FindFactor = %+v, %v", got, ok)
	}
}

func TestInterpretRuleSnapshotMatchesLeftClosedRightOpen(t *testing.T) {
	rule := InterpretRuleSnapshot{Min: 0, Max: 10}
	if !rule.Matches(0) || !rule.Matches(9.9) || rule.Matches(10) {
		t.Fatal("expected [0,10) semantics: min inclusive, max exclusive")
	}
}

func TestDefinitionRoundTripPreservesScaleRiskRules(t *testing.T) {
	t.Parallel()

	maxScore := 10.0
	original := &ScaleSnapshot{
		Code:                 "SCL-001",
		ScaleVersion:         "1.0.0",
		Title:                "Demo",
		QuestionnaireCode:    "QNR-001",
		QuestionnaireVersion: "2.0.0",
		Status:               "published",
		Factors: []FactorSnapshot{{
			Code:            "total",
			Title:           "总分",
			IsTotalScore:    true,
			QuestionCodes:   []string{"q1", "q2"},
			ScoringStrategy: "sum",
			MaxScore:        &maxScore,
			InterpretRules: []InterpretRuleSnapshot{{
				Min: 0, Max: 10, MaxInclusive: true, RiskLevel: "low", Conclusion: "低风险", Suggestion: "观察",
			}},
		}},
	}

	definition := DefinitionFromScaleSnapshot(original)
	got := ScaleSnapshotFromDefinition(ExecutionEnvelope{
		Code:                 original.Code,
		ScaleVersion:         original.ScaleVersion,
		Title:                original.Title,
		QuestionnaireCode:    original.QuestionnaireCode,
		QuestionnaireVersion: original.QuestionnaireVersion,
		Status:               original.Status,
	}, definition)

	if !reflect.DeepEqual(got, original) {
		t.Fatalf("scale definition round trip mismatch\n got: %#v\nwant: %#v", got, original)
	}
}
