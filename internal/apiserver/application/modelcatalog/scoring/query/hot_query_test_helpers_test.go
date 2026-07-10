package query

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publishedmodel"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel/hotrank"
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

func publishedSnapshotFromScale(t *testing.T, scale scalereadmodel.ScaleSummaryRow) *port.PublishedModel {
	t.Helper()
	model := newScaleAssessmentModelForQueryRefTest(t, scale.Code, scale.Title, scale.QuestionnaireCode, "1.0.0", domain.ModelStatusPublished)
	snapshot, err := publishedmodel.BuildAssessmentSnapshot(model)
	if err != nil {
		t.Fatalf("BuildAssessmentSnapshot: %v", err)
	}
	return snapshot
}

func hotPublishedSources(t *testing.T, scales ...scalereadmodel.ScaleSummaryRow) (*hotPublishedReaderStub, *hotPublishedRepoStub) {
	reader := &hotPublishedReaderStub{byQuestionnaire: map[string]*port.PublishedModel{}}
	repo := &hotPublishedRepoStub{byCode: map[string]*port.PublishedModel{}}
	for _, scale := range scales {
		if scale.Code == "" {
			continue
		}
		snapshot := publishedSnapshotFromScale(t, scale)
		reader.byQuestionnaire[scale.QuestionnaireCode] = snapshot
		reader.byQuestionnaire[scale.QuestionnaireCode+":1.0.0"] = snapshot
		repo.byCode[scale.Code] = snapshot
	}
	return reader, repo
}
