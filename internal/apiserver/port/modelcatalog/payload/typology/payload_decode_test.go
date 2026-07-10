package typology

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

func TestPayloadAndRuntimeSpecFromDefinitionDecodesPayloadEnvelope(t *testing.T) {
	t.Parallel()

	payload := explicitPoleCompositionPayload()
	payload.Algorithm = ""
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	decoded, runtime, err := PayloadAndRuntimeSpecFromDefinition(data, binding.AlgorithmMBTI)
	if err != nil {
		t.Fatalf("PayloadAndRuntimeSpecFromDefinition: %v", err)
	}
	if decoded.Algorithm != binding.AlgorithmMBTI {
		t.Fatalf("payload algorithm = %q, want mbti fallback", decoded.Algorithm)
	}
	if runtime.Decision.Kind != binding.DecisionKindPoleComposition {
		t.Fatalf("decision kind = %q, want pole_composition", runtime.Decision.Kind)
	}
}

func TestPayloadAndRuntimeSpecFromDefinitionWrapsRuntimeOnlyJSON(t *testing.T) {
	t.Parallel()

	payload := explicitPoleCompositionPayload()
	data, err := json.Marshal(payload.Runtime)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	decoded, runtime, err := PayloadAndRuntimeSpecFromDefinition(data, binding.AlgorithmSBTI)
	if err != nil {
		t.Fatalf("PayloadAndRuntimeSpecFromDefinition: %v", err)
	}
	if decoded.Algorithm != binding.AlgorithmSBTI {
		t.Fatalf("payload algorithm = %q, want sbti", decoded.Algorithm)
	}
	if decoded.Runtime == nil {
		t.Fatal("runtime-only JSON should be wrapped into payload.Runtime")
	}
	if runtime.Report.CategoryLabel != "Custom Pole Model" {
		t.Fatalf("runtime report category = %q", runtime.Report.CategoryLabel)
	}
}

func TestPayloadAndRuntimeSpecFromDefinitionPreservesInvalidRuntimeErrorPrefix(t *testing.T) {
	t.Parallel()

	_, _, err := PayloadAndRuntimeSpecFromDefinition([]byte(`{"not":`), binding.AlgorithmMBTI)
	if err == nil {
		t.Fatal("PayloadAndRuntimeSpecFromDefinition error = nil")
	}
	if !strings.Contains(err.Error(), "decode typology runtime spec") {
		t.Fatalf("error = %q, want decode typology runtime spec prefix", err.Error())
	}
}

func TestPayloadAndRuntimeSpecFromDefinitionRequiresAlgorithmForLegacyPayload(t *testing.T) {
	t.Parallel()

	payload := &Payload{
		DimensionOrder: []string{"EI"},
		Dimensions: map[string]Dimension{
			"EI": {Code: "EI", Name: "外向-内向"},
		},
		MatchingSpec: MatchingSpec{Kind: binding.DecisionKindPoleComposition},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	_, _, err = PayloadAndRuntimeSpecFromDefinition(data, "")
	if err == nil {
		t.Fatal("PayloadAndRuntimeSpecFromDefinition error = nil")
	}
	if !strings.Contains(err.Error(), "typology payload algorithm is required") {
		t.Fatalf("error = %q, want algorithm required", err.Error())
	}
}
