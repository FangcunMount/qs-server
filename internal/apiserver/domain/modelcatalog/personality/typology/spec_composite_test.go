package typology

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestFactorGraphSpecHasExplicitFactorGraph(t *testing.T) {
	fg := FactorGraphSpec{
		Factors: map[string]FactorSpec{"a": {ID: "a", Kind: FactorSpecKindLeaf}},
		Roots:   []string{"a"},
	}
	if !fg.HasExplicitFactorGraph() {
		t.Fatal("expected explicit factor graph")
	}
	if got := fg.DecisionFactorOrder(); len(got) != 1 || got[0] != "a" {
		t.Fatalf("DecisionFactorOrder = %#v", got)
	}
}

func TestToRuntimeSpecValidatesExplicitFactorRoots(t *testing.T) {
	payload := &Payload{
		Code:    "BAD_GRAPH",
		Version: "1.0.0",
		Runtime: &RuntimeSpec{
			FactorGraph: FactorGraphSpec{
				Factors: map[string]FactorSpec{
					"a": {ID: "a", Kind: FactorSpecKindLeaf},
				},
				Roots: []string{"missing"},
			},
			Decision: PersonalityDecisionSpec{Kind: modelcatalog.DecisionKindTraitProfile},
			OutcomeMapping: OutcomeMappingSpec{
				DetailKind: OutcomeDetailTraitProfile,
			},
			Report: ReportSpec{Kind: ReportKindTraitProfile},
		},
	}
	if _, err := payload.ToRuntimeSpec(); err == nil {
		t.Fatal("ToRuntimeSpec error = nil, want missing root validation")
	}
}
