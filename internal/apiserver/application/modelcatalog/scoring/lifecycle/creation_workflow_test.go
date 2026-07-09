package lifecycle

import (
	"context"
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestCreateUsesAssessmentModelRepositoryWhenConfigured(t *testing.T) {
	ctx := context.Background()
	scaleRepo := &createScaleRepoStub{}
	modelRepo := &createAssessmentModelRepoStub{}
	svc := NewService(
		scaleRepo,
		nil,
		nil,
		nil,
		WithAssessmentModelRepository(modelRepo),
	)

	got, err := svc.Create(ctx, shared.CreateScaleDTO{
		Code:           "SCL_CREATE",
		Title:          "Create Scale",
		Description:    "draft scale",
		Category:       "mental",
		Stages:         []string{"child"},
		ApplicableAges: []string{"6-12"},
		Reporters:      []string{"parent"},
		Tags:           []string{"screening"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if scaleRepo.createCount != 0 {
		t.Fatalf("legacy scale repo Create calls = %d, want 0", scaleRepo.createCount)
	}
	if modelRepo.createCount != 1 {
		t.Fatalf("model repo Create calls = %d, want 1", modelRepo.createCount)
	}
	model := modelRepo.created
	if model == nil {
		t.Fatal("created model = nil")
	}
	if model.Code != "SCL_CREATE" || model.Kind != domain.KindScale || model.ProductChannel != domain.ProductChannelMedicalScale {
		t.Fatalf("created model identity = %#v, want scale medical channel", model)
	}
	if model.Definition.Format != domain.PayloadFormatAssessmentScaleV1 || len(model.Definition.Data) == 0 {
		t.Fatalf("definition payload = format %q len %d, want assessment_scale_v1 bytes", model.Definition.Format, len(model.Definition.Data))
	}
	if model.DefinitionV2 == nil {
		t.Fatal("DefinitionV2 = nil, want materialized target definition")
	}
	if !reflect.DeepEqual(model.Stages, []string{"child"}) ||
		!reflect.DeepEqual(model.ApplicableAges, []string{"6-12"}) ||
		!reflect.DeepEqual(model.Reporters, []string{"parent"}) {
		t.Fatalf("created model audience metadata stages=%v ages=%v reporters=%v",
			model.Stages, model.ApplicableAges, model.Reporters)
	}
	if got == nil || got.Code != "SCL_CREATE" || got.Title != "Create Scale" || got.Status != scaledefinition.StatusDraft.String() {
		t.Fatalf("result = %#v, want created draft scale", got)
	}
	if len(got.Stages) != 1 || got.Stages[0] != "child" ||
		len(got.ApplicableAges) != 1 || got.ApplicableAges[0] != "6-12" ||
		len(got.Reporters) != 1 || got.Reporters[0] != "parent" {
		t.Fatalf("classification result = stages %#v ages %#v reporters %#v, want legacy response fields preserved",
			got.Stages, got.ApplicableAges, got.Reporters)
	}
}

type createScaleRepoStub struct {
	createCount int
}

func (r *createScaleRepoStub) Create(context.Context, *scaledefinition.MedicalScale) error {
	r.createCount++
	return nil
}

func (r *createScaleRepoStub) CreatePublishedSnapshot(context.Context, *scaledefinition.MedicalScale, bool) error {
	return nil
}

func (r *createScaleRepoStub) FindByCode(context.Context, string) (*scaledefinition.MedicalScale, error) {
	return nil, scaledefinition.ErrNotFound
}

func (r *createScaleRepoStub) FindByQuestionnaireCode(context.Context, string) (*scaledefinition.MedicalScale, error) {
	return nil, scaledefinition.ErrNotFound
}

func (r *createScaleRepoStub) Update(context.Context, *scaledefinition.MedicalScale) error {
	return nil
}

func (r *createScaleRepoStub) SetActivePublishedVersion(context.Context, string, string) error {
	return nil
}

func (r *createScaleRepoStub) ClearActivePublishedVersion(context.Context, string) error {
	return nil
}

func (r *createScaleRepoStub) Remove(context.Context, string) error {
	return nil
}

type createAssessmentModelRepoStub struct {
	createCount int
	created     *domain.AssessmentModel
}

func (r *createAssessmentModelRepoStub) Create(_ context.Context, model *domain.AssessmentModel) error {
	r.createCount++
	r.created = model
	return nil
}

func (r *createAssessmentModelRepoStub) Update(context.Context, *domain.AssessmentModel) error {
	return nil
}

func (r *createAssessmentModelRepoStub) FindByCode(context.Context, string) (*domain.AssessmentModel, error) {
	return nil, domain.ErrNotFound
}

func (r *createAssessmentModelRepoStub) List(context.Context, modelcatalogport.ListFilter) ([]*domain.AssessmentModel, int64, error) {
	return nil, 0, nil
}

func (r *createAssessmentModelRepoStub) Delete(context.Context, string) error {
	return nil
}

var _ modelcatalogport.ModelRepository = (*createAssessmentModelRepoStub)(nil)
