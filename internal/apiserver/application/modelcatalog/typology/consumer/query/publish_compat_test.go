package query

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type fakePublishedLister struct {
	snapshot *port.PublishedModel
}

func (f fakePublishedLister) GetPublishedModelByRef(context.Context, port.Ref) (*port.PublishedModel, error) {
	return f.snapshot, nil
}

func (f fakePublishedLister) FindPublishedModelByQuestionnaire(context.Context, string, string) (*port.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (f fakePublishedLister) FindPublishedModelByCode(_ context.Context, _ domain.Kind, code string) (*port.PublishedModel, error) {
	if f.snapshot != nil && f.snapshot.Code == code {
		return f.snapshot, nil
	}
	return nil, domain.ErrNotFound
}

func (f fakePublishedLister) ListPublishedModels(context.Context, port.ListPublishedFilter) ([]*port.PublishedModel, int64, error) {
	if f.snapshot == nil {
		return nil, 0, nil
	}
	return []*port.PublishedModel{f.snapshot}, 1, nil
}

func TestPublishedModelReadableByCollectionQuery(t *testing.T) {
	snapshot := &port.PublishedModel{
		SchemaVersion:        domain.SchemaVersionV2,
		PayloadFormat:        domain.PayloadFormatPersonalityTypologyV1,
		Kind:                 domain.KindTypology,
		SubKind:              domain.SubKindTypology,
		Algorithm:            domain.AlgorithmMBTI,
		Code:                 "MBTI_OEJTS",
		Version:              "1.0.0",
		Title:                "MBTI",
		Status:               "published",
		QuestionnaireCode:    "MBTI_OEJTS",
		QuestionnaireVersion: "1.0.0",
		DecisionKind:         domain.DecisionKindPoleComposition,
		Payload:              []byte(`{"code":"MBTI_OEJTS","algorithm":"mbti","runtime":{"factor_graph":{"factors":{"EI":{"id":"EI","code":"EI","name":"外向-内向","kind":"leaf","contributions":[{"question_code":"Q1"}]}},"roots":["EI"]}}}`),
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
