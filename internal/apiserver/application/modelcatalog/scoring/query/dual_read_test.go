package query

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publishedmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestGetByCodeReadsAssessmentModelBeforeLegacyScales(t *testing.T) {
	t.Parallel()

	model := newScaleAssessmentModelForQueryTest(t, "SCL_V2")
	repo := &dualReadScaleRepo{byCode: map[string]*scaledefinition.MedicalScale{
		"SCL_V2": newLegacyScaleForQueryTest(t, "SCL_LEGACY"),
	}}
	modelRepo := &dualReadModelRepo{models: map[string]*domain.AssessmentModel{model.Code: model}}
	service := newDualReadQueryService(repo, ModelCatalogSources{ModelRepo: modelRepo})

	got, err := service.GetByCode(context.Background(), "SCL_V2")
	if err != nil {
		t.Fatalf("GetByCode: %v", err)
	}
	if got.Code != "SCL_V2" {
		t.Fatalf("code = %q, want SCL_V2", got.Code)
	}
	if repo.findByCodeCalls != 0 {
		t.Fatalf("legacy FindByCode calls = %d, want 0", repo.findByCodeCalls)
	}
}

func TestGetByCodeFallsBackToLegacyScalesWhenAssessmentModelMissing(t *testing.T) {
	t.Parallel()

	repo := &dualReadScaleRepo{byCode: map[string]*scaledefinition.MedicalScale{
		"SCL_LEGACY": newLegacyScaleForQueryTest(t, "SCL_LEGACY"),
	}}
	service := newDualReadQueryService(repo, ModelCatalogSources{ModelRepo: &dualReadModelRepo{}})

	got, err := service.GetByCode(context.Background(), "SCL_LEGACY")
	if err != nil {
		t.Fatalf("GetByCode: %v", err)
	}
	if got.Code != "SCL_LEGACY" || repo.findByCodeCalls != 1 {
		t.Fatalf("result=%#v legacy calls=%d, want legacy fallback", got, repo.findByCodeCalls)
	}
}

func TestGetPublishedByCodeReadsPublishedModelBeforeLegacyScales(t *testing.T) {
	t.Parallel()

	snapshot := newPublishedScaleSnapshotForQueryTest(t, "SCL_PUB")
	repo := &dualReadScaleRepo{publishedByCode: map[string]*scaledefinition.MedicalScale{
		"SCL_PUB": newLegacyScaleForQueryTest(t, "SCL_LEGACY"),
	}}
	publishedRepo := &dualReadPublishedRepo{byCode: map[string]*port.PublishedModel{snapshot.Code: snapshot}}
	service := newDualReadQueryService(repo, ModelCatalogSources{PublishedRepo: publishedRepo})

	got, err := service.GetPublishedByCode(context.Background(), "SCL_PUB")
	if err != nil {
		t.Fatalf("GetPublishedByCode: %v", err)
	}
	if got.Code != "SCL_PUB" || repo.findPublishedByCodeCalls != 0 {
		t.Fatalf("result=%#v legacy published calls=%d, want v2 published", got, repo.findPublishedByCodeCalls)
	}
}

func TestResolveAssessmentScaleContextReadsPublishedModelByQuestionnaireVersion(t *testing.T) {
	t.Parallel()

	snapshot := newPublishedScaleSnapshotForQueryTest(t, "SCL_CTX")
	repo := &dualReadScaleRepo{byQuestionnaireRef: map[string]*scaledefinition.MedicalScale{
		"Q-SCL_CTX:1.0.0": newLegacyScaleForQueryTest(t, "SCL_LEGACY"),
	}}
	publishedRepo := &dualReadPublishedRepo{byQuestionnaire: map[string]*port.PublishedModel{
		"Q-SCL_CTX:1.0.0": snapshot,
	}}
	service := newDualReadQueryService(repo, ModelCatalogSources{PublishedReader: publishedRepo})

	got, err := service.ResolveAssessmentScaleContext(context.Background(), "Q-SCL_CTX", "1.0.0")
	if err != nil {
		t.Fatalf("ResolveAssessmentScaleContext: %v", err)
	}
	if got.MedicalScaleCode == nil || *got.MedicalScaleCode != "SCL_CTX" ||
		got.ScaleVersion == nil || *got.ScaleVersion != "1.0.0" {
		t.Fatalf("context = %#v, want v2 published scale context", got)
	}
	if repo.findByQuestionnaireRefCalls != 0 {
		t.Fatalf("legacy questionnaire ref calls = %d, want 0", repo.findByQuestionnaireRefCalls)
	}
}

func newDualReadQueryService(repo *dualReadScaleRepo, sources ModelCatalogSources) *queryService {
	return NewQueryServiceWithModelCatalogSources(repo, repo, nil, nil, nil, nil, sources).(*queryService)
}

func newScaleAssessmentModelForQueryTest(t *testing.T, code string) *domain.AssessmentModel {
	t.Helper()
	scale := newLegacyScaleForQueryTest(t, code)
	model, err := legacyadapter.AssessmentModelFromMedicalScale(scale, time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("AssessmentModelFromMedicalScale: %v", err)
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

func newLegacyScaleForQueryTest(t *testing.T, code string) *scaledefinition.MedicalScale {
	t.Helper()
	factor, err := scaledefinition.NewFactor(
		scaledefinition.NewFactorCode("total"),
		"Total",
		scaledefinition.WithIsTotalScore(true),
		scaledefinition.WithQuestionCodes([]meta.Code{meta.NewCode("Q1")}),
		scaledefinition.WithScoringStrategy(scaledefinition.ScoringStrategySum),
		scaledefinition.WithInterpretRules([]scaledefinition.InterpretationRule{
			scaledefinition.NewInterpretationRule(scaledefinition.NewScoreRange(0, 10), scaledefinition.RiskLevelLow, "low", "watch"),
		}),
	)
	if err != nil {
		t.Fatalf("NewFactor: %v", err)
	}
	scale, err := scaledefinition.NewMedicalScale(
		meta.NewCode(code),
		"Scale "+code,
		scaledefinition.WithQuestionnaire(meta.NewCode("Q-"+code), "1.0.0"),
		scaledefinition.WithScaleVersion("1.0.0"),
		scaledefinition.WithStatus(scaledefinition.StatusPublished),
		scaledefinition.WithFactors([]*scaledefinition.Factor{factor}),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale: %v", err)
	}
	return scale
}

type dualReadScaleRepo struct {
	byCode                      map[string]*scaledefinition.MedicalScale
	publishedByCode             map[string]*scaledefinition.MedicalScale
	byQuestionnaireCode         map[string]*scaledefinition.MedicalScale
	publishedByQuestionnaire    map[string]*scaledefinition.MedicalScale
	byQuestionnaireRef          map[string]*scaledefinition.MedicalScale
	findByCodeCalls             int
	findPublishedByCodeCalls    int
	findByQuestionnaireRefCalls int
}

func (r *dualReadScaleRepo) FindByCode(_ context.Context, code string) (*scaledefinition.MedicalScale, error) {
	r.findByCodeCalls++
	if item, ok := r.byCode[code]; ok {
		return item, nil
	}
	return nil, scaledefinition.ErrNotFound
}

func (r *dualReadScaleRepo) FindPublishedByCode(_ context.Context, code string) (*scaledefinition.MedicalScale, error) {
	r.findPublishedByCodeCalls++
	if item, ok := r.publishedByCode[code]; ok {
		return item, nil
	}
	return nil, scaledefinition.ErrNotFound
}

func (r *dualReadScaleRepo) FindByQuestionnaireCode(_ context.Context, code string) (*scaledefinition.MedicalScale, error) {
	if item, ok := r.byQuestionnaireCode[code]; ok {
		return item, nil
	}
	return nil, scaledefinition.ErrNotFound
}

func (r *dualReadScaleRepo) FindPublishedByQuestionnaireCode(_ context.Context, code string) (*scaledefinition.MedicalScale, error) {
	if item, ok := r.publishedByQuestionnaire[code]; ok {
		return item, nil
	}
	return nil, scaledefinition.ErrNotFound
}

func (r *dualReadScaleRepo) FindByQuestionnaireRef(_ context.Context, code, version string) (*scaledefinition.MedicalScale, error) {
	r.findByQuestionnaireRefCalls++
	if item, ok := r.byQuestionnaireRef[code+":"+version]; ok {
		return item, nil
	}
	return nil, scaledefinition.ErrNotFound
}

func (r *dualReadScaleRepo) ListScales(context.Context, scalereadmodel.ScaleFilter, scalereadmodel.PageRequest) ([]scalereadmodel.ScaleSummaryRow, error) {
	return nil, nil
}

func (r *dualReadScaleRepo) CountScales(context.Context, scalereadmodel.ScaleFilter) (int64, error) {
	return 0, nil
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
