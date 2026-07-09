package query

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publishedmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition/hotrank"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type hotPublishedReaderStub struct {
	byQuestionnaire map[string]*port.PublishedModel
	err             error
	calls           []string
}

func (s *hotPublishedReaderStub) GetPublishedModelByRef(context.Context, port.Ref) (*port.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (s *hotPublishedReaderStub) FindPublishedModelByQuestionnaire(_ context.Context, code, version string) (*port.PublishedModel, error) {
	s.calls = append(s.calls, code+":"+version)
	if s.err != nil {
		return nil, s.err
	}
	if version != "" {
		if item, ok := s.byQuestionnaire[code+":"+version]; ok {
			return item, nil
		}
	}
	if item, ok := s.byQuestionnaire[code]; ok {
		return item, nil
	}
	return nil, domain.ErrNotFound
}

type hotPublishedRepoStub struct {
	byCode map[string]*port.PublishedModel
}

func (s *hotPublishedRepoStub) Save(context.Context, *port.PublishedModel) error { return nil }

func (s *hotPublishedRepoStub) FindPublishedByModelCode(ctx context.Context, kind domain.Kind, code string) (*port.PublishedModel, error) {
	return s.FindLatestPublishedByModelCode(ctx, kind, code)
}

func (s *hotPublishedRepoStub) FindLatestPublishedByModelCode(_ context.Context, _ domain.Kind, code string) (*port.PublishedModel, error) {
	if item, ok := s.byCode[code]; ok {
		return item, nil
	}
	return nil, domain.ErrNotFound
}

func (s *hotPublishedRepoStub) FindPublishedByModelCodeVersion(context.Context, domain.Kind, string, string) (*port.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (s *hotPublishedRepoStub) ListPublished(context.Context, port.ListPublishedFilter) ([]*port.PublishedModel, int64, error) {
	return nil, 0, nil
}

func (s *hotPublishedRepoStub) DeletePublished(context.Context, domain.Kind, string) error {
	return nil
}

func newHotQueryService(reader *hotScaleReaderStub, publishedReader *hotPublishedReaderStub, publishedRepo *hotPublishedRepoStub, rank hotrank.ReadModel) *queryService {
	return NewQueryServiceWithModelCatalogSources(
		reader,
		nil,
		nil,
		nil,
		nil,
		ModelCatalogSources{PublishedRepo: publishedRepo, PublishedReader: publishedReader},
		rank,
	).(*queryService)
}

func publishedSnapshotFromScale(t *testing.T, scale *scaledefinition.MedicalScale) *port.PublishedModel {
	t.Helper()
	model, err := legacyadapter.AssessmentModelFromMedicalScale(scale, time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("AssessmentModelFromMedicalScale: %v", err)
	}
	model.Code = scale.GetCode().String()
	snapshot, err := publishedmodel.BuildAssessmentSnapshot(model)
	if err != nil {
		t.Fatalf("BuildAssessmentSnapshot: %v", err)
	}
	return snapshot
}

func hotPublishedSources(t *testing.T, scales ...*scaledefinition.MedicalScale) (*hotPublishedReaderStub, *hotPublishedRepoStub) {
	reader := &hotPublishedReaderStub{byQuestionnaire: map[string]*port.PublishedModel{}}
	repo := &hotPublishedRepoStub{byCode: map[string]*port.PublishedModel{}}
	for _, scale := range scales {
		if scale == nil {
			continue
		}
		snapshot := publishedSnapshotFromScale(t, scale)
		reader.byQuestionnaire[scale.GetQuestionnaireCode().String()] = snapshot
		reader.byQuestionnaire[scale.GetQuestionnaireCode().String()+":"+scale.GetQuestionnaireVersion()] = snapshot
		repo.byCode[scale.GetCode().String()] = snapshot
	}
	return reader, repo
}
