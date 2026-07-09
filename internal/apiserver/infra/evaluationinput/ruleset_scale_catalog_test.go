package evaluationinput

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/codec"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
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

type stubScaleFallbackCatalog struct {
	byRef *scalesnapshot.ScaleSnapshot
}

func (s stubScaleFallbackCatalog) GetScale(context.Context, string) (*scalesnapshot.ScaleSnapshot, error) {
	return nil, domain.ErrNotFound
}

func (s stubScaleFallbackCatalog) GetScaleByRef(context.Context, port.ModelRef) (*scalesnapshot.ScaleSnapshot, error) {
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

func TestPublishedScaleCatalogFallsBackToRepo(t *testing.T) {
	fallback := stubScaleFallbackCatalog{byRef: &scalesnapshot.ScaleSnapshot{
		Code:         "SCL-REPO",
		ScaleVersion: "1.0.0",
		Title:        "Repo Scale",
		Status:       "published",
	}}
	catalog := NewPublishedScaleCatalog(stubScalePublishedReader{}, fallback)
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
