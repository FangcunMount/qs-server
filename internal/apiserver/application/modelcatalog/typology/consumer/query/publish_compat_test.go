package query

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type fakePublishedLister struct {
	snapshot *domain.PublishedModelSnapshot
}

func (f fakePublishedLister) GetPublishedModelByRef(context.Context, port.Ref) (*domain.PublishedModelSnapshot, error) {
	return f.snapshot, nil
}

func (f fakePublishedLister) FindPublishedModelByQuestionnaire(context.Context, string, string) (*domain.PublishedModelSnapshot, error) {
	return nil, domain.ErrNotFound
}

func (f fakePublishedLister) FindPublishedModelByCode(_ context.Context, _ domain.Kind, code string) (*domain.PublishedModelSnapshot, error) {
	if f.snapshot != nil && f.snapshot.Model.Code == code {
		return f.snapshot, nil
	}
	return nil, domain.ErrNotFound
}

func (f fakePublishedLister) ListPublishedModels(context.Context, port.ListPublishedFilter) ([]*domain.PublishedModelSnapshot, int64, error) {
	if f.snapshot == nil {
		return nil, 0, nil
	}
	return []*domain.PublishedModelSnapshot{f.snapshot}, 1, nil
}

func TestPublishedModelSnapshotReadableByCollectionQuery(t *testing.T) {
	snapshot := &domain.PublishedModelSnapshot{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Model: domain.ModelDefinition{
			Kind:      domain.KindTypology,
			SubKind:   domain.SubKindTypology,
			Algorithm: domain.AlgorithmMBTI,
			Code:      "MBTI_OEJTS",
			Version:   "1.0.0",
			Title:     "MBTI",
			Status:    "published",
		},
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    "MBTI_OEJTS",
			QuestionnaireVersion: "1.0.0",
		},
		Decision: domain.DecisionSpec{Kind: domain.DecisionKindPoleComposition},
		Payload:  []byte(`{"code":"MBTI_OEJTS","algorithm":"mbti","runtime":{"factor_graph":{"factors":{"EI":{"id":"EI","code":"EI","name":"外向-内向","kind":"leaf","contributions":[{"question_code":"Q1"}]}},"roots":["EI"]}}}`),
	}
	svc := NewQueryService(fakePublishedLister{snapshot: snapshot})
	got, err := svc.GetPublishedByCode(context.Background(), "MBTI_OEJTS")
	if err != nil {
		t.Fatalf("GetPublishedByCode: %v", err)
	}
	if got == nil || got.Code != "MBTI_OEJTS" {
		t.Fatalf("result = %#v", got)
	}
	if got.QuestionCount != 1 {
		t.Fatalf("question count = %d, want 1", got.QuestionCount)
	}
}
