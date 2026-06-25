package ruleset

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleset"
)

type stubStore struct {
	byQuestionnaire *domain.RuleSetSnapshot
	byRef           *domain.RuleSetSnapshot
}

func (s stubStore) GetPublishedByRef(ctx context.Context, ref port.RuleSetRef) (*domain.RuleSetSnapshot, error) {
	if s.byRef != nil {
		return s.byRef, nil
	}
	return nil, domain.ErrNotFound
}

func (s stubStore) FindPublishedByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.RuleSetSnapshot, error) {
	if s.byQuestionnaire != nil {
		return s.byQuestionnaire, nil
	}
	return nil, domain.ErrNotFound
}

func TestLayeredCatalog_FallsBackToStatic(t *testing.T) {
	static, err := NewDefaultStaticCatalog(nil)
	if err != nil {
		t.Fatalf("NewDefaultStaticCatalog: %v", err)
	}
	catalog := NewLayeredCatalog(stubStore{}, static)

	ref, ok, err := catalog.ResolveByQuestionnaire(context.Background(), "SBTI_FUN", "1.0.0")
	if err != nil {
		t.Fatalf("ResolveByQuestionnaire: %v", err)
	}
	if !ok || ref.Code == "" {
		t.Fatalf("expected static binding, got ok=%v ref=%+v", ok, ref)
	}

	snapshot, err := catalog.GetPublishedByRef(context.Background(), ref)
	if err != nil {
		t.Fatalf("GetPublishedByRef: %v", err)
	}
	if snapshot.Definition.Code != ref.Code {
		t.Fatalf("snapshot code = %s, want %s", snapshot.Definition.Code, ref.Code)
	}
}

func TestLayeredCatalog_PrefersMongo(t *testing.T) {
	mongoSnapshot := &domain.RuleSetSnapshot{
		Definition: domain.RuleSetDefinition{
			Kind:    domain.RuleSetKindMBTI,
			Code:    "MBTI_FROM_MONGO",
			Version: "9.9.9",
			Status:  "published",
		},
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    "MBTI_OEJTS",
			QuestionnaireVersion: "1.0.0",
		},
		DecisionKind: domain.DecisionKindPoleComposition,
		Payload:      []byte(`{}`),
	}
	static, err := NewDefaultStaticCatalog(nil)
	if err != nil {
		t.Fatalf("NewDefaultStaticCatalog: %v", err)
	}
	catalog := NewLayeredCatalog(stubStore{byQuestionnaire: mongoSnapshot, byRef: mongoSnapshot}, static)

	ref, ok, err := catalog.ResolveByQuestionnaire(context.Background(), "MBTI_OEJTS", "1.0.0")
	if err != nil {
		t.Fatalf("ResolveByQuestionnaire: %v", err)
	}
	if !ok || ref.Code != "MBTI_FROM_MONGO" {
		t.Fatalf("ref = %+v, want MBTI_FROM_MONGO", ref)
	}
}
