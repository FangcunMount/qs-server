package ruleset

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type runtimeStubStore struct {
	byQuestionnaire *port.PublishedModel
	byRef           *port.PublishedModel
}

func (s runtimeStubStore) FindPublishedModelByCode(_ context.Context, _ domain.Kind, _ string) (*port.PublishedModel, error) {
	if s.byRef != nil {
		return s.byRef, nil
	}
	return nil, domain.ErrNotFound
}

func (s runtimeStubStore) ListPublishedModels(context.Context, port.ListPublishedFilter) ([]*port.PublishedModel, int64, error) {
	return nil, 0, nil
}

func (s runtimeStubStore) GetPublishedModelByRef(_ context.Context, ref port.Ref) (*port.PublishedModel, error) {
	if s.byRef != nil {
		return s.byRef, nil
	}
	return nil, domain.ErrNotFound
}

func (s runtimeStubStore) FindPublishedModelByQuestionnaire(_ context.Context, _, _ string) (*port.PublishedModel, error) {
	if s.byQuestionnaire != nil {
		return s.byQuestionnaire, nil
	}
	return nil, domain.ErrNotFound
}

func TestRuntimePublishedCatalogReturnsNotFoundWithoutStaticFallback(t *testing.T) {
	catalog := NewRuntimePublishedCatalogWithStore(runtimeStubStore{})

	ref, ok, err := catalog.ResolveByQuestionnaire(context.Background(), "retired-questionnaire", "1.0.0")
	if err != nil {
		t.Fatalf("ResolveByQuestionnaire: %v", err)
	}
	if ok {
		t.Fatalf("expected miss without static fallback, got ref=%#v", ref)
	}

	_, err = catalog.GetPublishedModelByRef(context.Background(), port.Ref{
		Kind:      domain.KindTypology,
		SubKind:   domain.SubKindTypology,
		Algorithm: domain.AlgorithmPersonalityTypology,
		Code:      "retired-model",
		Version:   "1.0.0",
	})
	if err == nil || !domain.IsNotFound(err) {
		t.Fatalf("GetPublishedModelByRef() err = %v, want not found", err)
	}
}

func TestRuntimePublishedCatalogReturnsPublishedDefinitionV2Snapshot(t *testing.T) {
	mongoSnapshot := &port.PublishedModel{
		Kind:                 domain.KindTypology,
		Code:                 "personality_demo",
		Version:              "9.9.9",
		Title:                "from mongo",
		QuestionnaireCode:    "demo-q",
		QuestionnaireVersion: "1.0.0",
	}
	catalog := NewRuntimePublishedCatalogWithStore(runtimeStubStore{byQuestionnaire: mongoSnapshot, byRef: mongoSnapshot})

	ref, ok, err := catalog.ResolveByQuestionnaire(context.Background(), "demo-q", "1.0.0")
	if err != nil || !ok {
		t.Fatalf("ResolveByQuestionnaire: ok=%v err=%v", ok, err)
	}
	if ref.Version != "9.9.9" {
		t.Fatalf("ref version = %s, want 9.9.9", ref.Version)
	}
}
