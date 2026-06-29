package query

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
)

type fakePublishedLister struct {
	snapshot *domain.Snapshot
}

func (f fakePublishedLister) GetPublishedByRef(context.Context, port.Ref) (*domain.Snapshot, error) {
	return f.snapshot, nil
}

func (f fakePublishedLister) FindPublishedByQuestionnaire(context.Context, string, string) (*domain.Snapshot, error) {
	return nil, domain.ErrNotFound
}

func (f fakePublishedLister) FindPublishedByModelCode(_ context.Context, _ domain.Kind, code string) (*domain.Snapshot, error) {
	if f.snapshot != nil && f.snapshot.Definition.Code == code {
		return f.snapshot, nil
	}
	return nil, domain.ErrNotFound
}

func (f fakePublishedLister) ListPublished(context.Context, port.ListPublishedFilter) ([]*domain.Snapshot, int64, error) {
	if f.snapshot == nil {
		return nil, 0, nil
	}
	return []*domain.Snapshot{f.snapshot}, 1, nil
}

func TestPublishedSnapshotStillReadableByCollectionQuery(t *testing.T) {
	snapshot := &domain.Snapshot{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Definition: domain.Definition{
			Kind:    domain.KindMBTIMigration,
			Code:    "MBTI_OEJTS",
			Version: "1.0.0",
			Title:   "MBTI",
			Status:  "published",
		},
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    "MBTI_OEJTS",
			QuestionnaireVersion: "1.0.0",
		},
		DecisionKind: domain.DecisionKindPoleComposition,
		Payload:      []byte(`{"code":"MBTI_OEJTS","algorithm":"mbti"}`),
	}
	svc := NewQueryService(fakePublishedLister{snapshot: snapshot})
	got, err := svc.GetPublishedByCode(context.Background(), "MBTI_OEJTS")
	if err != nil {
		t.Fatalf("GetPublishedByCode: %v", err)
	}
	if got == nil || got.Code != "MBTI_OEJTS" {
		t.Fatalf("result = %#v", got)
	}
}
