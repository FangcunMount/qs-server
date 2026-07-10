package evaluationinput

import (
	"context"
	"errors"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/codec"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

type stubScalePublishedReader struct {
	snapshot *rulesetport.PublishedModel
}

func (s stubScalePublishedReader) GetPublishedModelByRef(context.Context, rulesetport.Ref) (*rulesetport.PublishedModel, error) {
	if s.snapshot == nil {
		return nil, domain.ErrNotFound
	}
	return s.snapshot, nil
}

func (s stubScalePublishedReader) FindPublishedModelByQuestionnaire(context.Context, string, string) (*rulesetport.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (s stubScalePublishedReader) FindPublishedModelByCode(context.Context, domain.Kind, string) (*rulesetport.PublishedModel, error) {
	if s.snapshot == nil {
		return nil, domain.ErrNotFound
	}
	return s.snapshot, nil
}

func (s stubScalePublishedReader) ListPublishedModels(context.Context, rulesetport.ListPublishedFilter) ([]*rulesetport.PublishedModel, int64, error) {
	return nil, 0, domain.ErrNotFound
}

type stubScaleFallbackCatalog struct {
	byRef *scalesnapshot.ScaleSnapshot
	calls *int
}

func (s stubScaleFallbackCatalog) GetScale(context.Context, string) (*scalesnapshot.ScaleSnapshot, error) {
	if s.calls != nil {
		(*s.calls)++
	}
	return nil, domain.ErrNotFound
}

func (s stubScaleFallbackCatalog) GetScaleByRef(context.Context, port.ModelRef) (*scalesnapshot.ScaleSnapshot, error) {
	if s.calls != nil {
		(*s.calls)++
	}
	if s.byRef == nil {
		return nil, domain.ErrNotFound
	}
	return s.byRef, nil
}

func TestPublishedScaleCatalogPrefersPublishedPayload(t *testing.T) {
	fromMongo := &scalesnapshot.ScaleSnapshot{
		Code:         "SCL-MONGO",
		ScaleVersion: "2.0.0",
		Title:        "Mongo Scale",
		Status:       "published",
	}
	payload, format, err := codec.EncodeScale(fromMongo)
	if err != nil {
		t.Fatalf("EncodeScale: %v", err)
	}
	reader := stubScalePublishedReader{snapshot: &rulesetport.PublishedModel{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: format,
		Kind:          domain.KindScale,
		Code:          fromMongo.Code,
		Version:       fromMongo.ScaleVersion,
		Title:         fromMongo.Title,
		Status:        fromMongo.Status,
		Payload:       payload,
		DefinitionV2:  scalesnapshot.DefinitionFromScaleSnapshot(fromMongo),
	}}
	fallback := stubScaleFallbackCatalog{byRef: &scalesnapshot.ScaleSnapshot{
		Code:         "SCL-MONGO",
		ScaleVersion: "1.0.0",
		Title:        "Repo Scale",
		Status:       "published",
	}}
	catalog := NewPublishedScaleCatalog(reader, fallback)
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

func TestPublishedScaleCatalogGetScalePrefersDefinitionV2(t *testing.T) {
	fromDefinition := &scalesnapshot.ScaleSnapshot{
		Code:         "SCL-V2",
		ScaleVersion: "2.0.0",
		Title:        "Definition Scale",
		Status:       "published",
		Factors: []scalesnapshot.FactorSnapshot{{
			Code:            "total",
			Title:           "Total",
			IsTotalScore:    true,
			QuestionCodes:   []string{"q1"},
			ScoringStrategy: "sum",
		}},
	}
	fallbackCalls := 0
	catalog := NewPublishedScaleCatalog(
		stubScalePublishedReader{snapshot: &rulesetport.PublishedModel{
			SchemaVersion: domain.SchemaVersionV2,
			PayloadFormat: domain.PayloadFormatAssessmentScaleV1,
			Kind:          domain.KindScale,
			Code:          fromDefinition.Code,
			Version:       fromDefinition.ScaleVersion,
			Title:         fromDefinition.Title,
			Status:        fromDefinition.Status,
			Payload:       []byte(`not-json`),
			DefinitionV2:  scalesnapshot.DefinitionFromScaleSnapshot(fromDefinition),
		}},
		stubScaleFallbackCatalog{byRef: &scalesnapshot.ScaleSnapshot{Title: "Repo Scale"}, calls: &fallbackCalls},
	)

	got, err := catalog.GetScale(t.Context(), "SCL-V2")
	if err != nil {
		t.Fatalf("GetScale: %v", err)
	}
	if got.Title != "Definition Scale" || got.Factors[0].Code != "total" {
		t.Fatalf("scale = %#v", got)
	}
	if fallbackCalls != 0 {
		t.Fatalf("fallback calls = %d, want 0", fallbackCalls)
	}
}

func TestPublishedScaleCatalogDoesNotReadLegacyRepoFallback(t *testing.T) {
	fallbackCalls := 0
	fallback := stubScaleFallbackCatalog{byRef: &scalesnapshot.ScaleSnapshot{
		Code:         "SCL-REPO",
		ScaleVersion: "1.0.0",
		Title:        "Repo Scale",
		Status:       "published",
	}, calls: &fallbackCalls}
	catalog := NewPublishedScaleCatalog(stubScalePublishedReader{}, fallback)
	_, err := catalog.GetScaleByRef(t.Context(), port.ModelRef{
		Kind:    port.EvaluationModelKindScale,
		Code:    "SCL-REPO",
		Version: "1.0.0",
	})
	if err == nil {
		t.Fatal("expected missing published model to fail")
	}
	var kindCarrier port.FailureKindCarrier
	if !errors.As(err, &kindCarrier) {
		t.Fatalf("expected resolve error, got %T %v", err, err)
	}
	if got := kindCarrier.FailureKind(); got != port.FailureKindModelNotFound {
		t.Fatalf("failure kind = %s, want %s", got, port.FailureKindModelNotFound)
	}
	if fallbackCalls != 0 {
		t.Fatalf("fallback calls = %d, want 0", fallbackCalls)
	}
}
