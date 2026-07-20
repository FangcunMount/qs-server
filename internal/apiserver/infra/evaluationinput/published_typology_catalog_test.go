package evaluationinput

import (
	"os"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	typology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func TestDecodePublishedTypologyModelUsesDefinitionV2(t *testing.T) {
	raw, err := os.ReadFile("../../port/modelcatalog/payload/typology/testdata/personality_typology_v1.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	definition, err := typology.DefinitionFromLegacyPayload(raw, binding.AlgorithmMBTI)
	if err != nil {
		t.Fatalf("DefinitionFromLegacyPayload: %v", err)
	}
	got, err := decodePublishedTypologyModel(&port.PublishedModel{
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Code:          "MBTI_CONTRACT",
		Version:       "1.0.0",
		Title:         "MBTI Contract",
		Status:        "published",
		Algorithm:     domain.AlgorithmMBTI,
		Payload:       []byte("not-json"),
		DefinitionV2:  definition,
	})
	if err != nil {
		t.Fatalf("decodePublishedTypologyModel: %v", err)
	}
	if got.Code != "MBTI_CONTRACT" || got.Algorithm != domain.AlgorithmMBTI {
		t.Fatalf("payload = %#v", got)
	}
}

func TestDecodePublishedTypologyModelRequiresDefinitionV2(t *testing.T) {
	_, err := decodePublishedTypologyModel(&port.PublishedModel{PayloadFormat: domain.PayloadFormatPersonalityTypologyV1})
	if err == nil {
		t.Fatal("decodePublishedTypologyModel() error = nil, want definition_v2 requirement")
	}
}

func TestAssertTypologyAlgorithmRequiresExactMatch(t *testing.T) {
	t.Parallel()
	payload := &typology.Payload{Algorithm: domain.AlgorithmPersonalityTypology}
	if _, err := assertTypologyAlgorithm(payload, domain.AlgorithmMBTI); err == nil {
		t.Fatal("mbti ref must not match personality_typology payload after dual-identity retirement")
	}
	got, err := assertTypologyAlgorithm(payload, domain.AlgorithmPersonalityTypology)
	if err != nil || got != payload {
		t.Fatalf("exact match = %#v err=%v", got, err)
	}
}

func TestTypologyLookupRefsExactAlgorithmOnly(t *testing.T) {
	t.Parallel()
	refs := typologyLookupRefs(evaluationinput.ModelRef{
		Kind: evaluationinput.EvaluationModelKindTypology, Code: "M", Version: "1", Algorithm: string(domain.AlgorithmMBTI),
	}, domain.AlgorithmMBTI)
	if len(refs) != 1 || refs[0].Algorithm != domain.AlgorithmMBTI {
		t.Fatalf("refs = %#v", refs)
	}
}
