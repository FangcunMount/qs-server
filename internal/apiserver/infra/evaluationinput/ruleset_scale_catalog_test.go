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
	catalog := NewPublishedScaleCatalog(reader)
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
	)

	got, err := catalog.GetScale(t.Context(), "SCL-V2")
	if err != nil {
		t.Fatalf("GetScale: %v", err)
	}
	if got.Title != "Definition Scale" || got.Factors[0].Code != "total" {
		t.Fatalf("scale = %#v", got)
	}
}

func TestPublishedScaleCatalogRejectsMissingPublishedModel(t *testing.T) {
	catalog := NewPublishedScaleCatalog(stubScalePublishedReader{})
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
}
