package ruleset

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type runtimeV2StubStore struct {
	byQuestionnaire *port.PublishedModel
	byRef           *port.PublishedModel
}

func (s runtimeV2StubStore) GetPublishedModelByRef(_ context.Context, ref port.Ref) (*port.PublishedModel, error) {
	if s.byRef != nil {
		return s.byRef, nil
	}
	return nil, domain.ErrNotFound
}

func (s runtimeV2StubStore) FindPublishedModelByQuestionnaire(_ context.Context, _, _ string) (*port.PublishedModel, error) {
	if s.byQuestionnaire != nil {
		return s.byQuestionnaire, nil
	}
	return nil, domain.ErrNotFound
}

func TestRuntimePublishedCatalogResolveByQuestionnaireUsesV2Ref(t *testing.T) {
	published := &port.PublishedModel{
		Kind:                 domain.KindTypology,
		SubKind:              domain.SubKindTypology,
		Algorithm:            domain.AlgorithmSBTI,
		Code:                 "personality_demo",
		Version:              "9.9.9",
		Title:                "from mongo",
		QuestionnaireCode:    "demo-q",
		QuestionnaireVersion: "1.0.0",
	}
	catalog := NewRuntimePublishedCatalogWithStore(runtimeV2StubStore{byQuestionnaire: published})

	ref, ok, err := catalog.ResolveByQuestionnaire(context.Background(), "demo-q", "1.0.0")
	if err != nil || !ok {
		t.Fatalf("ResolveByQuestionnaire: ok=%v err=%v", ok, err)
	}
	if ref.Kind != domain.KindTypology || ref.SubKind != domain.SubKindTypology || ref.Algorithm != domain.AlgorithmSBTI {
		t.Fatalf("ref identity = %#v", ref)
	}
	if ref.Code != "personality_demo" || ref.Version != "9.9.9" {
		t.Fatalf("ref = %#v", ref)
	}
}
