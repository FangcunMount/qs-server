package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestPublishUsesPublicationPublisherWhenAssessmentModelStoreConfigured(t *testing.T) {
	ctx := context.Background()
	legacyScale := newPublishableScaleForTest(t)
	model, err := legacyadapter.AssessmentModelFromMedicalScale(legacyScale, time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("AssessmentModelFromMedicalScale() error = %v", err)
	}
	model.Code = "SCL-001"

	modelRepo := &publishAssessmentModelRepoStub{model: model}
	publishedRepo := &publishPublishedModelRepoStub{}
	svc := newAuthoringLifecycleService(
		publishedQuestionnaireCatalogForScalePublish(),
		modelRepo,
		publishedRepo,
	)

	got, err := svc.Publish(ctx, "SCL-001")
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if got == nil || got.Status != "published" {
		t.Fatalf("result = %#v, want published scale", got)
	}
	if modelRepo.updateCount < 1 {
		t.Fatalf("model repo updates = %d, want at least 1", modelRepo.updateCount)
	}
	if len(publishedRepo.calls) != 2 || publishedRepo.calls[0] != "delete" || publishedRepo.calls[1] != "save" {
		t.Fatalf("published repo calls = %v, want delete+save", publishedRepo.calls)
	}
	if model.Status != domain.ModelStatusPublished {
		t.Fatalf("model status = %s, want published", model.Status)
	}
	if publishedRepo.lastSnapshot == nil || publishedRepo.lastSnapshot.Kind != domain.KindScale || publishedRepo.lastSnapshot.Code != "SCL-001" {
		t.Fatalf("saved snapshot = %#v, want scale SCL-001", publishedRepo.lastSnapshot)
	}
}

type publishAssessmentModelRepoStub struct {
	model       *domain.AssessmentModel
	updateCount int
}

func (r *publishAssessmentModelRepoStub) Create(context.Context, *domain.AssessmentModel) error {
	return nil
}

func (r *publishAssessmentModelRepoStub) Update(_ context.Context, model *domain.AssessmentModel) error {
	r.updateCount++
	r.model = model
	return nil
}

func (r *publishAssessmentModelRepoStub) FindByCode(_ context.Context, code string) (*domain.AssessmentModel, error) {
	if r.model == nil || r.model.Code != code {
		return nil, domain.ErrNotFound
	}
	return r.model, nil
}

func (r *publishAssessmentModelRepoStub) FindByQuestionnaireCode(context.Context, domain.Kind, string) (*domain.AssessmentModel, error) {
	return nil, domain.ErrNotFound
}

func (r *publishAssessmentModelRepoStub) List(context.Context, modelcatalogport.ListFilter) ([]*domain.AssessmentModel, int64, error) {
	return nil, 0, nil
}

func (r *publishAssessmentModelRepoStub) Delete(context.Context, string) error { return nil }

type publishPublishedModelRepoStub struct {
	calls        []string
	lastSnapshot *modelcatalogport.PublishedModel
}

func (r *publishPublishedModelRepoStub) Save(_ context.Context, snapshot *modelcatalogport.PublishedModel) error {
	r.calls = append(r.calls, "save")
	r.lastSnapshot = snapshot
	return nil
}

func (r *publishPublishedModelRepoStub) FindPublishedByModelCode(context.Context, domain.Kind, string) (*modelcatalogport.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (r *publishPublishedModelRepoStub) FindLatestPublishedByModelCode(context.Context, domain.Kind, string) (*modelcatalogport.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (r *publishPublishedModelRepoStub) FindPublishedByModelCodeVersion(context.Context, domain.Kind, string, string) (*modelcatalogport.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (r *publishPublishedModelRepoStub) ListPublished(context.Context, modelcatalogport.ListPublishedFilter) ([]*modelcatalogport.PublishedModel, int64, error) {
	return nil, 0, nil
}

func (r *publishPublishedModelRepoStub) DeletePublished(_ context.Context, _ domain.Kind, _ string) error {
	r.calls = append(r.calls, "delete")
	return nil
}

var _ modelcatalogport.ModelRepository = (*publishAssessmentModelRepoStub)(nil)
var _ modelcatalogport.PublishedModelRepository = (*publishPublishedModelRepoStub)(nil)

func newPublishableScaleForTest(t *testing.T) *scaledefinition.MedicalScale {
	t.Helper()
	factor, err := scaledefinition.NewFactor(
		scaledefinition.NewFactorCode("total"),
		"总分",
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
		meta.NewCode("SCL-001"),
		"Demo",
		scaledefinition.WithQuestionnaire(meta.NewCode("QNR-001"), "1.0.0"),
		scaledefinition.WithScaleVersion("1.0.0"),
		scaledefinition.WithFactors([]*scaledefinition.Factor{factor}),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale: %v", err)
	}
	return scale
}

func publishedQuestionnaireCatalogForScalePublish() *questionnaireCatalogBindingStub {
	return &questionnaireCatalogBindingStub{
		byCode: map[string]*questionnairecatalog.Item{
			"QNR-001": {Code: "QNR-001", Version: "1.0.0", Status: "published", Type: "MedicalScale"},
		},
		byVersion: map[string]*questionnairecatalog.Item{
			"QNR-001:1.0.0": {Code: "QNR-001", Version: "1.0.0", Status: "published", Type: "MedicalScale"},
		},
	}
}
