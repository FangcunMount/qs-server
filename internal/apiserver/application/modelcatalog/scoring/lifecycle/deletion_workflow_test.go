package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestDeleteUsesAssessmentModelRepositoryWhenConfigured(t *testing.T) {
	ctx := context.Background()
	scale, err := scaledefinition.NewMedicalScale(
		meta.NewCode("SCL_DELETE"),
		"Draft Scale",
		scaledefinition.WithStatus(scaledefinition.StatusDraft),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}
	model, err := legacyadapter.AssessmentModelFromMedicalScale(scale, time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("AssessmentModelFromMedicalScale() error = %v", err)
	}

	legacyRepo := &deleteScaleRepoStub{scale: scale}
	modelRepo := &deleteAssessmentModelRepoStub{model: model}
	svc := NewService(
		legacyRepo,
		nil,
		nil,
		nil,
		WithAssessmentModelRepository(modelRepo),
	)

	if err := svc.Delete(ctx, "SCL_DELETE"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if legacyRepo.removeCount != 0 {
		t.Fatalf("legacy scale repo Remove calls = %d, want 0", legacyRepo.removeCount)
	}
	if modelRepo.deleteCount != 1 {
		t.Fatalf("model repo Delete calls = %d, want 1", modelRepo.deleteCount)
	}
}

func TestDeleteRejectsNonDraftAssessmentModel(t *testing.T) {
	ctx := context.Background()
	scale, err := scaledefinition.NewMedicalScale(
		meta.NewCode("SCL_PUBLISHED"),
		"Published Scale",
		scaledefinition.WithStatus(scaledefinition.StatusPublished),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}
	model, err := legacyadapter.AssessmentModelFromMedicalScale(scale, time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("AssessmentModelFromMedicalScale() error = %v", err)
	}

	modelRepo := &deleteAssessmentModelRepoStub{model: model}
	svc := NewService(
		&deleteScaleRepoStub{},
		nil,
		nil,
		nil,
		WithAssessmentModelRepository(modelRepo),
	)

	if err := svc.Delete(ctx, "SCL_PUBLISHED"); err == nil {
		t.Fatal("Delete() error = nil, want invalid argument")
	}
	if modelRepo.deleteCount != 0 {
		t.Fatalf("model repo Delete calls = %d, want 0", modelRepo.deleteCount)
	}
}

type deleteScaleRepoStub struct {
	scale       *scaledefinition.MedicalScale
	removeCount int
}

func (r *deleteScaleRepoStub) Create(context.Context, *scaledefinition.MedicalScale) error { return nil }

func (r *deleteScaleRepoStub) CreatePublishedSnapshot(context.Context, *scaledefinition.MedicalScale, bool) error {
	return nil
}

func (r *deleteScaleRepoStub) FindByCode(_ context.Context, code string) (*scaledefinition.MedicalScale, error) {
	if r.scale != nil && r.scale.GetCode().String() == code {
		return r.scale, nil
	}
	return nil, scaledefinition.ErrNotFound
}

func (r *deleteScaleRepoStub) FindByQuestionnaireCode(context.Context, string) (*scaledefinition.MedicalScale, error) {
	return nil, scaledefinition.ErrNotFound
}

func (r *deleteScaleRepoStub) Update(context.Context, *scaledefinition.MedicalScale) error { return nil }

func (r *deleteScaleRepoStub) SetActivePublishedVersion(context.Context, string, string) error { return nil }

func (r *deleteScaleRepoStub) ClearActivePublishedVersion(context.Context, string) error { return nil }

func (r *deleteScaleRepoStub) Remove(context.Context, string) error {
	r.removeCount++
	return nil
}

type deleteAssessmentModelRepoStub struct {
	model       *domain.AssessmentModel
	deleteCount int
}

func (r *deleteAssessmentModelRepoStub) Create(context.Context, *domain.AssessmentModel) error { return nil }

func (r *deleteAssessmentModelRepoStub) Update(context.Context, *domain.AssessmentModel) error { return nil }

func (r *deleteAssessmentModelRepoStub) FindByCode(_ context.Context, code string) (*domain.AssessmentModel, error) {
	if r.model != nil && r.model.Code == code {
		return r.model, nil
	}
	return nil, domain.ErrNotFound
}

func (r *deleteAssessmentModelRepoStub) List(context.Context, modelcatalogport.ListFilter) ([]*domain.AssessmentModel, int64, error) {
	return nil, 0, nil
}

func (r *deleteAssessmentModelRepoStub) Delete(context.Context, string) error {
	r.deleteCount++
	return nil
}

var _ modelcatalogport.ModelRepository = (*deleteAssessmentModelRepoStub)(nil)
