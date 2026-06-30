package personality

import (
	"encoding/json"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
)

func TestDefinitionFromModelNormalizesFullPayloadToRuntimeSpec(t *testing.T) {
	payload := &modeltypology.Payload{
		Algorithm: domain.AlgorithmMBTI,
		DimensionOrder: []string{"EI"},
		Dimensions: map[string]modeltypology.Dimension{
			"EI": {Code: "EI", Name: "外向/内向"},
		},
		Runtime: &modeltypology.RuntimeSpec{
			Decision: modeltypology.PersonalityDecisionSpec{Kind: domain.DecisionKindPoleComposition},
			Report: modeltypology.ReportSpec{
				Kind:       modeltypology.ReportKindPersonalityType,
				AdapterKey: modeltypology.ReportAdapterMBTI,
			},
			FactorGraph: modeltypology.FactorGraphSpec{
				Factors: map[string]modeltypology.FactorSpec{
					"EI": {ID: "EI", Kind: modeltypology.FactorSpecKindLeaf},
				},
				Roots: []string{"EI"},
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	model := &domain.AssessmentModel{
		Kind:      domain.KindPersonality,
		SubKind:   domain.SubKindTypology,
		Algorithm: domain.AlgorithmMBTI,
		Definition: domain.DefinitionPayload{
			Format: domain.PayloadFormatPersonalityTypologyV1,
			Data:   raw,
		},
	}

	result := definitionFromModel(model)
	if result == nil {
		t.Fatal("definitionFromModel returned nil")
	}

	var runtime modeltypology.RuntimeSpec
	if err := json.Unmarshal(result.Payload, &runtime); err != nil {
		t.Fatalf("unmarshal definition payload: %v", err)
	}
	if !runtime.FactorGraph.HasExplicitFactorGraph() {
		t.Fatalf("factor graph = %#v, want explicit graph", runtime.FactorGraph)
	}
	if runtime.Decision.Kind != domain.DecisionKindPoleComposition {
		t.Fatalf("decision kind = %s", runtime.Decision.Kind)
	}
}
