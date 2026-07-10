package query

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publishedmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
)

func TestGetByCodeReadsAssessmentModel(t *testing.T) {
	t.Parallel()

	model := newScaleAssessmentModelForQueryTest(t, "SCL_V2")
	modelRepo := &dualReadModelRepo{models: map[string]*domain.AssessmentModel{model.Code: model}}
	service := newDualReadQueryService(ModelCatalogSources{ModelRepo: modelRepo})

	got, err := service.GetByCode(context.Background(), "SCL_V2")
	if err != nil {
		t.Fatalf("GetByCode: %v", err)
	}
	if got.Code != "SCL_V2" {
		t.Fatalf("code = %q, want SCL_V2", got.Code)
	}
}

func TestGetByCodeReturnsNotFoundWhenAssessmentModelMissing(t *testing.T) {
	t.Parallel()

	service := newDualReadQueryService(ModelCatalogSources{ModelRepo: &dualReadModelRepo{}})

	_, err := service.GetByCode(context.Background(), "SCL_LEGACY")
	if err == nil {
		t.Fatal("GetByCode() error = nil, want not found")
	}
}

func TestGetPublishedByCodeReadsPublishedModel(t *testing.T) {
	t.Parallel()

	snapshot := newPublishedScaleSnapshotForQueryTest(t, "SCL_PUB")
	publishedRepo := &dualReadPublishedRepo{byCode: map[string]*port.PublishedModel{snapshot.Code: snapshot}}
	service := newDualReadQueryService(ModelCatalogSources{PublishedRepo: publishedRepo})

	got, err := service.GetPublishedByCode(context.Background(), "SCL_PUB")
	if err != nil {
		t.Fatalf("GetPublishedByCode: %v", err)
	}
	if got.Code != "SCL_PUB" {
		t.Fatalf("result=%#v, want v2 published", got)
	}
}

func TestResolveAssessmentScaleContextReadsPublishedModelByQuestionnaireVersion(t *testing.T) {
	t.Parallel()

	snapshot := newPublishedScaleSnapshotForQueryTest(t, "SCL_CTX")
	publishedRepo := &dualReadPublishedRepo{byQuestionnaire: map[string]*port.PublishedModel{
		"Q-SCL_CTX:1.0.0": snapshot,
	}}
	service := newDualReadQueryService(ModelCatalogSources{PublishedReader: publishedRepo})

	got, err := service.ResolveAssessmentScaleContext(context.Background(), "Q-SCL_CTX", "1.0.0")
	if err != nil {
		t.Fatalf("ResolveAssessmentScaleContext: %v", err)
	}
	if got.MedicalScaleCode == nil || *got.MedicalScaleCode != "SCL_CTX" ||
		got.ScaleVersion == nil || *got.ScaleVersion != "1.0.0" {
		t.Fatalf("context = %#v, want v2 published scale context", got)
	}
}

func newDualReadQueryService(sources ModelCatalogSources) *queryService {
	return NewQueryServiceWithModelCatalogSources(
		emptyScaleReader{},
		nil,
		nil,
		nil,
		nil,
		sources,
	).(*queryService)
}

type emptyScaleReader struct{}

func (emptyScaleReader) ListScales(context.Context, scalereadmodel.ScaleFilter, scalereadmodel.PageRequest) ([]scalereadmodel.ScaleSummaryRow, error) {
	return nil, nil
}

func (emptyScaleReader) CountScales(context.Context, scalereadmodel.ScaleFilter) (int64, error) {
	return 0, nil
}

func newScaleAssessmentModelForQueryTest(t *testing.T, code string) *domain.AssessmentModel {
	t.Helper()
	return newScaleAssessmentModelForQueryRefTest(t, code, "Scale "+code, "Q-"+code, "1.0.0", domain.ModelStatusPublished)
}

func newScaleAssessmentModelForQueryRefTest(t *testing.T, code, title, questionnaireCode, questionnaireVersion string, status domain.ModelStatus) *domain.AssessmentModel {
	t.Helper()
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	model, err := legacyadapter.AssessmentModelFromCreateDTO(shared.CreateScaleDTO{
		Code:                 code,
		Title:                title,
		Category:             "adhd",
		QuestionnaireCode:    questionnaireCode,
		QuestionnaireVersion: questionnaireVersion,
	}, now)
	if err != nil {
		t.Fatalf("AssessmentModelFromCreateDTO: %v", err)
	}
	snapshot := &scalesnapshot.ScaleSnapshot{
		Code:                 model.Code,
		ScaleVersion:         "1.0.0",
		Title:                model.Title,
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Status:               string(status),
		Factors: []scalesnapshot.FactorSnapshot{{
			Code:            "total",
			Title:           "Total",
			IsTotalScore:    true,
			QuestionCodes:   []string{"Q1"},
			ScoringStrategy: "sum",
			InterpretRules: []scalesnapshot.InterpretRuleSnapshot{{
				Min: 0, Max: 10, RiskLevel: "low", Conclusion: "low", Suggestion: "watch",
			}},
		}},
	}
	payload, err := legacyadapter.DefinitionPayloadFromScaleSnapshot(snapshot)
	if err != nil {
		t.Fatalf("DefinitionPayloadFromScaleSnapshot: %v", err)
	}
	if err := model.UpdateDefinitionWithV2(payload, scalesnapshot.DefinitionFromScaleSnapshot(snapshot), now); err != nil {
		t.Fatalf("UpdateDefinitionWithV2: %v", err)
	}
	if status == domain.ModelStatusPublished {
		if err := model.MarkPublished(now); err != nil {
			t.Fatalf("MarkPublished: %v", err)
		}
	}
	return model
}

func newPublishedScaleSnapshotForQueryTest(t *testing.T, code string) *port.PublishedModel {
	t.Helper()
	model := newScaleAssessmentModelForQueryTest(t, code)
	snapshot, err := publishedmodel.BuildAssessmentSnapshot(model)
	if err != nil {
		t.Fatalf("BuildAssessmentSnapshot: %v", err)
	}
	return snapshot
}

type dualReadModelRepo struct {
	models map[string]*domain.AssessmentModel
}

func (r *dualReadModelRepo) Create(context.Context, *domain.AssessmentModel) error { return nil }
func (r *dualReadModelRepo) Update(context.Context, *domain.AssessmentModel) error { return nil }

func (r *dualReadModelRepo) FindByCode(_ context.Context, code string) (*domain.AssessmentModel, error) {
	if item, ok := r.models[code]; ok {
		return item, nil
	}
	return nil, domain.ErrNotFound
}

func (r *dualReadModelRepo) FindByQuestionnaireCode(context.Context, domain.Kind, string) (*domain.AssessmentModel, error) {
	return nil, domain.ErrNotFound
}

func (r *dualReadModelRepo) List(context.Context, port.ListFilter) ([]*domain.AssessmentModel, int64, error) {
	return nil, 0, nil
}

func (r *dualReadModelRepo) Delete(context.Context, string) error { return nil }

type dualReadPublishedRepo struct {
	byCode          map[string]*port.PublishedModel
	byQuestionnaire map[string]*port.PublishedModel
}

func (r *dualReadPublishedRepo) Save(context.Context, *port.PublishedModel) error { return nil }

func (r *dualReadPublishedRepo) FindPublishedByModelCode(ctx context.Context, kind domain.Kind, code string) (*port.PublishedModel, error) {
	return r.FindLatestPublishedByModelCode(ctx, kind, code)
}

func (r *dualReadPublishedRepo) FindLatestPublishedByModelCode(_ context.Context, _ domain.Kind, code string) (*port.PublishedModel, error) {
	if item, ok := r.byCode[code]; ok {
		return item, nil
	}
	return nil, domain.ErrNotFound
}

func (r *dualReadPublishedRepo) FindPublishedByModelCodeVersion(context.Context, domain.Kind, string, string) (*port.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (r *dualReadPublishedRepo) ListPublished(context.Context, port.ListPublishedFilter) ([]*port.PublishedModel, int64, error) {
	return nil, 0, nil
}

func (r *dualReadPublishedRepo) DeletePublished(context.Context, domain.Kind, string) error {
	return nil
}

func (r *dualReadPublishedRepo) GetPublishedModelByRef(context.Context, port.Ref) (*port.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (r *dualReadPublishedRepo) FindPublishedModelByQuestionnaire(_ context.Context, code, version string) (*port.PublishedModel, error) {
	if item, ok := r.byQuestionnaire[code+":"+version]; ok {
		return item, nil
	}
	return nil, domain.ErrNotFound
}

var _ port.ModelRepository = (*dualReadModelRepo)(nil)
var _ port.PublishedModelRepository = (*dualReadPublishedRepo)(nil)
var _ port.PublishedModelReader = (*dualReadPublishedRepo)(nil)
var _ = shared.ScaleResult{}
