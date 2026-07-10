package lifecycle

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestDeleteUsesAssessmentModelRepository(t *testing.T) {
	ctx := context.Background()
	model := newLifecycleScaleAssessmentModel(
		t,
		"SCL_DELETE",
		"Draft Scale",
		"",
		"",
		domain.ModelStatusDraft,
		nil,
	)

	modelRepo := &deleteAssessmentModelRepoStub{model: model}
	svc := newAuthoringLifecycleService(nil, modelRepo, nil)

	if err := svc.Delete(ctx, "SCL_DELETE"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if modelRepo.deleteCount != 1 {
		t.Fatalf("model repo Delete calls = %d, want 1", modelRepo.deleteCount)
	}
}

func TestDeleteRejectsNonDraftAssessmentModel(t *testing.T) {
	ctx := context.Background()
	model := newLifecycleScaleAssessmentModel(
		t,
		"SCL_PUBLISHED",
		"Published Scale",
		"",
		"",
		domain.ModelStatusPublished,
		nil,
	)

	modelRepo := &deleteAssessmentModelRepoStub{model: model}
	svc := newAuthoringLifecycleService(nil, modelRepo, nil)

	if err := svc.Delete(ctx, "SCL_PUBLISHED"); err == nil {
		t.Fatal("Delete() error = nil, want invalid argument")
	}
	if modelRepo.deleteCount != 0 {
		t.Fatalf("model repo Delete calls = %d, want 0", modelRepo.deleteCount)
	}
}

type deleteAssessmentModelRepoStub struct {
	model       *domain.AssessmentModel
	deleteCount int
}

func (r *deleteAssessmentModelRepoStub) Create(context.Context, *domain.AssessmentModel) error {
	return nil
}

func (r *deleteAssessmentModelRepoStub) Update(context.Context, *domain.AssessmentModel) error {
	return nil
}

func (r *deleteAssessmentModelRepoStub) FindByCode(_ context.Context, code string) (*domain.AssessmentModel, error) {
	if r.model != nil && r.model.Code == code {
		return r.model, nil
	}
	return nil, domain.ErrNotFound
}

func (r *deleteAssessmentModelRepoStub) FindByQuestionnaireCode(context.Context, domain.Kind, string) (*domain.AssessmentModel, error) {
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
