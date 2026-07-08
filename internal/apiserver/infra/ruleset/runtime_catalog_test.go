package ruleset

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	cataloglegacy "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/legacy"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type runtimeStubStore struct {
	byQuestionnaire *domain.Snapshot
	byRef           *domain.Snapshot
}

func (s runtimeStubStore) GetPublishedByRef(_ context.Context, ref port.Ref) (*domain.Snapshot, error) {
	if s.byRef != nil {
		return s.byRef, nil
	}
	return nil, domain.ErrNotFound
}

func (s runtimeStubStore) FindPublishedByQuestionnaire(_ context.Context, _, _ string) (*domain.Snapshot, error) {
	if s.byQuestionnaire != nil {
		return s.byQuestionnaire, nil
	}
	return nil, domain.ErrNotFound
}

func (s runtimeStubStore) GetPublishedModelByRef(ctx context.Context, ref port.Ref) (*domain.PublishedModelSnapshot, error) {
	snapshot, err := s.GetPublishedByRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	return domain.PublishedFromLegacy(snapshot), nil
}

func (s runtimeStubStore) FindPublishedModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.PublishedModelSnapshot, error) {
	snapshot, err := s.FindPublishedByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	return domain.PublishedFromLegacy(snapshot), nil
}

func TestRuntimePublishedCatalogReturnsNotFoundWithoutStaticFallback(t *testing.T) {
	static, err := NewDefaultStaticCatalog(nil)
	if err != nil {
		t.Fatalf("NewDefaultStaticCatalog: %v", err)
	}
	catalog := NewRuntimePublishedCatalogWithStore(runtimeStubStore{})

	ref, ok, err := catalog.ResolveByQuestionnaire(context.Background(), cataloglegacy.SBTIQuestionnaireCode, cataloglegacy.SBTIModelVersion)
	if err != nil {
		t.Fatalf("ResolveByQuestionnaire: %v", err)
	}
	if ok {
		t.Fatalf("expected miss without static fallback, got ref=%#v", ref)
	}

	staticRef, staticOK, err := static.ResolveByQuestionnaire(context.Background(), cataloglegacy.SBTIQuestionnaireCode, cataloglegacy.SBTIModelVersion)
	if err != nil {
		t.Fatalf("static ResolveByQuestionnaire: %v", err)
	}
	if !staticOK {
		t.Fatal("static catalog should still resolve embedded seed")
	}
	if staticRef.Code == "" {
		t.Fatal("static catalog ref should be populated")
	}

	_, err = catalog.GetPublishedByRef(context.Background(), staticRef)
	if err == nil || !domain.IsNotFound(err) {
		t.Fatalf("GetPublishedByRef() err = %v, want not found", err)
	}
}

func TestRuntimePublishedCatalogPrefersV2Snapshot(t *testing.T) {
	mongoSnapshot := &domain.Snapshot{
		Definition: domain.Definition{
			Kind:    domain.KindPersonality,
			Code:    "personality_demo",
			Version: "9.9.9",
			Title:   "from mongo",
		},
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    "demo-q",
			QuestionnaireVersion: "1.0.0",
		},
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
