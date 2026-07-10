package evaluationinput

import (
	"os"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	typology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func TestDecodePublishedTypologyModelUsesDefinitionV2(t *testing.T) {
	raw, err := os.ReadFile("../../port/modelcatalog/payload/typology/testdata/personality_typology_v1.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	definition, err := typology.DefinitionFromPayload(raw, binding.AlgorithmMBTI)
	if err != nil {
		t.Fatalf("DefinitionFromPayload: %v", err)
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
