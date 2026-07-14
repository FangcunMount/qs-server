package grpcbridge

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	modeldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
	appcatalog "github.com/FangcunMount/qs-server/internal/collection-server/application/modelcatalog"
)

func TestTypologyCatalogProjectorUsesCanonicalDefinition(t *testing.T) {
	t.Parallel()

	payload, err := os.ReadFile(filepath.Join("..", "..", "..", "apiserver", "testdata", "personality", "frontend_payload_mbti.json"))
	if err != nil {
		t.Fatalf("read typology fixture: %v", err)
	}
	materialized, err := modeltypology.ImportLegacyDefinition(payload, modeldomain.AlgorithmMBTI)
	if err != nil {
		t.Fatalf("ImportLegacyDefinition: %v", err)
	}
	definitionData, err := json.Marshal(materialized.Definition)
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	reader := &catalogReaderStub{model: &appcatalog.CatalogModel{
		Code:                 "MBTI_CONTRACT",
		Kind:                 string(modeldomain.KindTypology),
		SubKind:              string(modeldomain.SubKindTypology),
		Algorithm:            string(modeldomain.AlgorithmMBTI),
		ProductChannel:       string(modeldomain.ProductChannelTypology),
		Version:              "1.0.0",
		Title:                "MBTI Contract",
		Status:               "published",
		QuestionnaireCode:    "Q_MBTI",
		QuestionnaireVersion: "2.0.0",
		Definition:           definitionData,
	}}

	projector := NewTypologyCatalogProjector(appcatalog.NewQueryService(reader))
	result, err := projector.GetTypologyModel(context.Background(), "MBTI_CONTRACT")
	if err != nil {
		t.Fatalf("GetTypologyModel: %v", err)
	}
	if result == nil || result.Kind != string(modeldomain.KindTypology) || result.QuestionCount != 4 {
		t.Fatalf("projected typology model = %#v", result)
	}
	if result.DecisionKind != string(modeldomain.DecisionKindPoleComposition) || len(result.Dimensions) != 4 || len(result.Outcomes) != 1 {
		t.Fatalf("DefinitionV2 projection lost typology semantics: %#v", result)
	}
}

func TestTypologyCatalogProjectorNormalizesVerifiedLegacyPersonalityKind(t *testing.T) {
	t.Parallel()

	definition := typologyDefinitionFixture(t)
	result, err := typologyResponseFromCatalog(&appcatalog.ModelResponse{
		Code:       "SBTI_FUN",
		Kind:       "personality",
		SubKind:    string(modeldomain.SubKindTypology),
		Algorithm:  string(modeldomain.AlgorithmSBTI),
		Definition: definition,
	}, nil)
	if err != nil {
		t.Fatalf("typologyResponseFromCatalog: %v", err)
	}
	if result.Kind != string(modeldomain.KindTypology) {
		t.Fatalf("normalized kind = %q, want typology", result.Kind)
	}
}

func TestTypologyCatalogProjectorRejectsUnverifiedLegacyPersonalityKind(t *testing.T) {
	t.Parallel()

	_, err := typologyResponseFromCatalog(&appcatalog.ModelResponse{
		Code:      "NOT_A_TYPOLOGY",
		Kind:      "personality",
		SubKind:   "trait",
		Algorithm: string(modeldomain.AlgorithmSBTI),
	}, nil)
	if err == nil {
		t.Fatal("typologyResponseFromCatalog error = nil, want rejected legacy personality record")
	}
}

func typologyDefinitionFixture(t *testing.T) json.RawMessage {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("..", "..", "..", "apiserver", "testdata", "personality", "frontend_payload_mbti.json"))
	if err != nil {
		t.Fatalf("read typology fixture: %v", err)
	}
	materialized, err := modeltypology.ImportLegacyDefinition(payload, modeldomain.AlgorithmMBTI)
	if err != nil {
		t.Fatalf("ImportLegacyDefinition: %v", err)
	}
	definition, err := json.Marshal(materialized.Definition)
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	return definition
}

type catalogReaderStub struct {
	model *appcatalog.CatalogModel
}

func (s *catalogReaderStub) GetPublishedModel(context.Context, string, string) (*appcatalog.CatalogModel, error) {
	return s.model, nil
}

func (s *catalogReaderStub) ListPublishedModels(context.Context, string, string, string, string, string, string, string, int32, int32) (*appcatalog.CatalogList, error) {
	return nil, nil
}

func (s *catalogReaderStub) ListHotPublishedModels(context.Context, string, int32, int32) (*appcatalog.HotCatalogList, error) {
	return nil, nil
}

func (s *catalogReaderStub) GetCatalogOptions(context.Context, string) (*appcatalog.CatalogOptions, error) {
	return nil, nil
}
