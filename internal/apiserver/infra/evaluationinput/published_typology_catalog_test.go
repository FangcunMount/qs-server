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

func TestAssertTypologyAlgorithmAcceptsEquivalentAlias(t *testing.T) {
	t.Parallel()
	payload := &typology.Payload{Algorithm: domain.AlgorithmPersonalityTypology}
	got, err := assertTypologyAlgorithm(payload, domain.AlgorithmMBTI)
	if err != nil || got != payload {
		t.Fatalf("assert = %#v err=%v", got, err)
	}
	if _, err := assertTypologyAlgorithm(payload, domain.AlgorithmSBTI); err != nil {
		t.Fatalf("sbti should also be equivalent to personality_typology payload: %v", err)
	}
	payload.Algorithm = domain.AlgorithmMBTI
	if _, err := assertTypologyAlgorithm(payload, domain.AlgorithmSBTI); err == nil {
		t.Fatal("mbti payload must not match sbti ref")
	}
}

func TestTypologyLookupRefsIncludesCanonicalAlternate(t *testing.T) {
	t.Parallel()
	refs := typologyLookupRefs(evaluationinput.ModelRef{
		Kind: evaluationinput.EvaluationModelKindTypology, Code: "M", Version: "1", Algorithm: string(domain.AlgorithmMBTI),
	}, domain.AlgorithmMBTI)
	if len(refs) < 2 || refs[0].Algorithm != domain.AlgorithmMBTI || refs[1].Algorithm != domain.AlgorithmPersonalityTypology {
		t.Fatalf("refs = %#v", refs)
	}
}
