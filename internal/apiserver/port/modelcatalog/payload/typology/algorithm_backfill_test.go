package typology

import (
	"encoding/json"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestEvaluateAlgorithmBackfillRequiresRetainedAlias(t *testing.T) {
	t.Parallel()
	got := EvaluateAlgorithmBackfill(binding.AlgorithmPersonalityTypology, nil, nil)
	if got.Eligible || got.Reason != "not_retained_read_alias" {
		t.Fatalf("got = %#v", got)
	}
}

func TestEvaluateAlgorithmBackfillAcceptsDefinitionRuntime(t *testing.T) {
	t.Parallel()
	payload := FromMBTI(&MBTILegacyModel{
		Code: "MBTI_TEST", Version: "1.0.0",
		DimensionOrder: []string{"EI"},
		Dimensions: map[string]MBTILegacyDimension{
			"EI": {Code: "EI", Name: "E/I", LeftPole: "I", RightPole: "E"},
		},
		TypeProfiles: []MBTILegacyTypeProfile{{TypeCode: "INTJ", TypeName: "建筑师"}},
	})
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	mat, err := ImportLegacyDefinition(raw, binding.AlgorithmMBTI)
	if err != nil {
		t.Fatalf("ImportLegacyDefinition: %v", err)
	}
	got := EvaluateAlgorithmBackfill(binding.AlgorithmMBTI, mat.Definition, nil)
	if !got.Eligible || got.To != binding.AlgorithmPersonalityTypology {
		t.Fatalf("got = %#v", got)
	}
}

func TestEvaluateAlgorithmBackfillAcceptsExplicitRuntime(t *testing.T) {
	t.Parallel()
	payload := explicitPoleCompositionPayload()
	payload.Algorithm = binding.AlgorithmMBTI
	got := EvaluateAlgorithmBackfill(binding.AlgorithmMBTI, nil, payload)
	if !got.Eligible {
		t.Fatalf("got = %#v", got)
	}
}

func TestEvaluateAlgorithmBackfillRejectsLegacyFlatWithoutRuntime(t *testing.T) {
	t.Parallel()
	payload := FromMBTI(&MBTILegacyModel{
		Code: "MBTI_FLAT", Version: "1.0.0",
		DimensionOrder: []string{"EI"},
		Dimensions: map[string]MBTILegacyDimension{
			"EI": {Code: "EI", Name: "E/I", LeftPole: "I", RightPole: "E"},
		},
		TypeProfiles: []MBTILegacyTypeProfile{{TypeCode: "INTJ", TypeName: "建筑师"}},
	})
	payload.Runtime = nil
	got := EvaluateAlgorithmBackfill(binding.AlgorithmMBTI, nil, payload)
	if got.Eligible {
		t.Fatalf("flat legacy without definition should be ineligible: %#v", got)
	}
}

func TestEvaluateAlgorithmBackfillRejectsIncompleteDefinition(t *testing.T) {
	t.Parallel()
	def := &definition.Definition{
		Measure: definition.MeasureSpec{Factors: []factor.Factor{{Code: "EI"}}},
		Conclusions: []conclusion.Conclusion{conclusion.TypeConclusion{
			Decision: conclusion.TypeDecision{Kind: binding.DecisionKindPoleComposition},
		}},
	}
	got := EvaluateAlgorithmBackfill(binding.AlgorithmSBTI, def, nil)
	if got.Eligible {
		t.Fatalf("incomplete definition should be ineligible: %#v", got)
	}
}
