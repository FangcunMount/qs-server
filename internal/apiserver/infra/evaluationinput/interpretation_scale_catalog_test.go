package evaluationinput

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretationmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/interpretationmodel/codec"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	interpretationmodelport "github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationmodel"
)

type stubScaleRuleReader struct {
	snapshot *domain.RuleSetSnapshot
}

func (s stubScaleRuleReader) GetPublishedByRef(context.Context, interpretationmodelport.ModelRef) (*domain.RuleSetSnapshot, error) {
	if s.snapshot == nil {
		return nil, domain.ErrNotFound
	}
	return s.snapshot, nil
}

func (s stubScaleRuleReader) FindPublishedByQuestionnaire(context.Context, string, string) (*domain.RuleSetSnapshot, error) {
	return nil, domain.ErrNotFound
}

type stubScaleFallbackCatalog struct {
	byRef *port.ScaleSnapshot
}

func (s stubScaleFallbackCatalog) GetScale(context.Context, string) (*port.ScaleSnapshot, error) {
	return nil, domain.ErrNotFound
}

func (s stubScaleFallbackCatalog) GetScaleByRef(context.Context, port.ModelRef) (*port.ScaleSnapshot, error) {
	if s.byRef == nil {
		return nil, domain.ErrNotFound
	}
	return s.byRef, nil
}

func TestInterpretationScaleCatalogPrefersRuleSetPayload(t *testing.T) {
	fromMongo := &port.ScaleSnapshot{
		Code:         "SCL-MONGO",
		ScaleVersion: "2.0.0",
		Title:        "Mongo Scale",
		Status:       "published",
	}
	payload, format, err := codec.EncodeScale(fromMongo)
	if err != nil {
		t.Fatalf("EncodeScale: %v", err)
	}
	reader := stubScaleRuleReader{snapshot: &domain.RuleSetSnapshot{
		SchemaVersion: domain.RuleSetSchemaVersionV1,
		PayloadFormat: format,
		Definition: domain.ModelDefinition{
			Kind:    domain.ModelKindScale,
			Code:    fromMongo.Code,
			Version: fromMongo.ScaleVersion,
		},
		Payload: payload,
	}}
	fallback := stubScaleFallbackCatalog{byRef: &port.ScaleSnapshot{
		Code:         "SCL-MONGO",
		ScaleVersion: "1.0.0",
		Title:        "Repo Scale",
		Status:       "published",
	}}
	catalog := NewInterpretationScaleCatalog(reader, fallback)
	got, err := catalog.GetScaleByRef(t.Context(), port.ModelRef{
		Kind:    port.EvaluationModelKindScale,
		Code:    "SCL-MONGO",
		Version: "2.0.0",
	})
	if err != nil {
		t.Fatalf("GetScaleByRef: %v", err)
	}
	if got.Title != "Mongo Scale" {
		t.Fatalf("Title = %s, want Mongo Scale", got.Title)
	}
}

func TestInterpretationScaleCatalogFallsBackToRepo(t *testing.T) {
	fallback := stubScaleFallbackCatalog{byRef: &port.ScaleSnapshot{
		Code:         "SCL-REPO",
		ScaleVersion: "1.0.0",
		Title:        "Repo Scale",
		Status:       "published",
	}}
	catalog := NewInterpretationScaleCatalog(stubScaleRuleReader{}, fallback)
	got, err := catalog.GetScaleByRef(t.Context(), port.ModelRef{
		Kind:    port.EvaluationModelKindScale,
		Code:    "SCL-REPO",
		Version: "1.0.0",
	})
	if err != nil {
		t.Fatalf("GetScaleByRef: %v", err)
	}
	if got.Title != "Repo Scale" {
		t.Fatalf("Title = %s, want Repo Scale", got.Title)
	}
}
