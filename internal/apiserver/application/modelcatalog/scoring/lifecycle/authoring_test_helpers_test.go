package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/assessmentstore"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/ports"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
)

func authoringLifecycleOptions(modelRepo modelcatalogport.ModelRepository, publishedRepo modelcatalogport.PublishedModelRepository) []ServiceOption {
	if modelRepo == nil {
		modelRepo = &authoringModelRepoStub{}
	}
	if publishedRepo == nil {
		publishedRepo = &authoringPublishedRepoStub{}
	}
	return []ServiceOption{
		WithAssessmentModelRepository(modelRepo),
		WithPublishedModelRepository(publishedRepo),
		WithPublicationPublisher(assessmentstore.NewPublicationPublisher(modelRepo, publishedRepo)),
	}
}

func newAuthoringLifecycleService(
	catalog questionnairecatalog.Catalog,
	modelRepo modelcatalogport.ModelRepository,
	publishedRepo modelcatalogport.PublishedModelRepository,
	opts ...ServiceOption,
) ports.ScaleLifecycleService {
	base := authoringLifecycleOptions(modelRepo, publishedRepo)
	return NewService(catalog, event.NewNopEventPublisher(), nil, append(base, opts...)...)
}

func assessmentModelFromScale(t *testing.T, scale *scaledefinition.MedicalScale) *domain.AssessmentModel {
	t.Helper()
	model, err := legacyadapter.AssessmentModelFromMedicalScale(scale, time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("AssessmentModelFromMedicalScale() error = %v", err)
	}
	model.Code = scale.GetCode().String()
	return model
}

type authoringModelRepoStub struct {
	model       *domain.AssessmentModel
	createCount int
	updateCount int
	deleteCount int
	created     *domain.AssessmentModel
	byQuestion  map[string]*domain.AssessmentModel
}

func (r *authoringModelRepoStub) Create(_ context.Context, model *domain.AssessmentModel) error {
	r.createCount++
	r.created = model
	if r.byQuestion == nil {
		r.byQuestion = map[string]*domain.AssessmentModel{}
	}
	if model != nil && model.Binding.QuestionnaireCode != "" {
		r.byQuestion[model.Binding.QuestionnaireCode] = model
	}
	return nil
}

func (r *authoringModelRepoStub) Update(_ context.Context, model *domain.AssessmentModel) error {
	r.updateCount++
	r.model = model
	if r.byQuestion == nil {
		r.byQuestion = map[string]*domain.AssessmentModel{}
	}
	if model != nil && model.Binding.QuestionnaireCode != "" {
		r.byQuestion[model.Binding.QuestionnaireCode] = model
	}
	return nil
}

func (r *authoringModelRepoStub) FindByCode(_ context.Context, code string) (*domain.AssessmentModel, error) {
	if r.model != nil && r.model.Code == code {
		return r.model, nil
	}
	return nil, domain.ErrNotFound
}

func (r *authoringModelRepoStub) FindByQuestionnaireCode(_ context.Context, kind domain.Kind, questionnaireCode string) (*domain.AssessmentModel, error) {
	if r.byQuestion != nil {
		if model, ok := r.byQuestion[questionnaireCode]; ok && (kind == "" || model.Kind == kind) {
			return model, nil
		}
	}
	if r.model != nil && r.model.Binding.QuestionnaireCode == questionnaireCode && (kind == "" || r.model.Kind == kind) {
		return r.model, nil
	}
	return nil, domain.ErrNotFound
}

func (r *authoringModelRepoStub) List(context.Context, modelcatalogport.ListFilter) ([]*domain.AssessmentModel, int64, error) {
	return nil, 0, nil
}

func (r *authoringModelRepoStub) Delete(context.Context, string) error {
	r.deleteCount++
	return nil
}

type authoringPublishedRepoStub struct {
	deleteCount int
}

func (r *authoringPublishedRepoStub) Save(context.Context, *modelcatalogport.PublishedModel) error {
	return nil
}

func (r *authoringPublishedRepoStub) FindPublishedByModelCode(context.Context, domain.Kind, string) (*modelcatalogport.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (r *authoringPublishedRepoStub) FindLatestPublishedByModelCode(context.Context, domain.Kind, string) (*modelcatalogport.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (r *authoringPublishedRepoStub) FindPublishedByModelCodeVersion(context.Context, domain.Kind, string, string) (*modelcatalogport.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (r *authoringPublishedRepoStub) ListPublished(context.Context, modelcatalogport.ListPublishedFilter) ([]*modelcatalogport.PublishedModel, int64, error) {
	return nil, 0, nil
}

func (r *authoringPublishedRepoStub) DeletePublished(context.Context, domain.Kind, string) error {
	r.deleteCount++
	return nil
}

var (
	_ modelcatalogport.ModelRepository          = (*authoringModelRepoStub)(nil)
	_ modelcatalogport.PublishedModelRepository = (*authoringPublishedRepoStub)(nil)
)
