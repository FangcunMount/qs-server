package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/assessmentstore"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/ports"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
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

func newLifecycleScaleAssessmentModel(
	t *testing.T,
	code, title, questionnaireCode, questionnaireVersion string,
	status domain.ModelStatus,
	factors []scalesnapshot.FactorSnapshot,
) *domain.AssessmentModel {
	t.Helper()
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	model, err := legacyadapter.AssessmentModelFromCreateDTO(shared.CreateScaleDTO{
		Code:                 code,
		Title:                title,
		QuestionnaireCode:    questionnaireCode,
		QuestionnaireVersion: questionnaireVersion,
	}, now)
	if err != nil {
		t.Fatalf("AssessmentModelFromCreateDTO() error = %v", err)
	}
	snapshot := &scalesnapshot.ScaleSnapshot{
		Code:                 model.Code,
		ScaleVersion:         "1.0.0",
		Title:                model.Title,
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Status:               string(status),
		Factors:              cloneLifecycleFactorSnapshots(factors),
	}
	payload, err := legacyadapter.DefinitionPayloadFromScaleSnapshot(snapshot)
	if err != nil {
		t.Fatalf("DefinitionPayloadFromScaleSnapshot() error = %v", err)
	}
	if err := model.UpdateDefinitionWithV2(payload, scalesnapshot.DefinitionFromScaleSnapshot(snapshot), now); err != nil {
		t.Fatalf("UpdateDefinitionWithV2() error = %v", err)
	}
	switch status {
	case domain.ModelStatusPublished:
		if err := model.MarkPublished(now); err != nil {
			t.Fatalf("MarkPublished() error = %v", err)
		}
	case domain.ModelStatusArchived:
		if err := model.MarkArchived(now); err != nil {
			t.Fatalf("MarkArchived() error = %v", err)
		}
	default:
		model.Status = domain.ModelStatusDraft
	}
	return model
}

func lifecycleDefaultFactorSnapshots() []scalesnapshot.FactorSnapshot {
	return []scalesnapshot.FactorSnapshot{{
		Code:            "F1",
		Title:           "Factor 1",
		QuestionCodes:   []string{"Q1"},
		ScoringStrategy: "sum",
		InterpretRules: []scalesnapshot.InterpretRuleSnapshot{{
			Min: 0, Max: 10, RiskLevel: "low", Conclusion: "low", Suggestion: "watch",
		}},
	}}
}

func lifecyclePublishableFactorSnapshots() []scalesnapshot.FactorSnapshot {
	return []scalesnapshot.FactorSnapshot{{
		Code:            "total",
		Title:           "总分",
		IsTotalScore:    true,
		QuestionCodes:   []string{"Q1"},
		ScoringStrategy: "sum",
		InterpretRules: []scalesnapshot.InterpretRuleSnapshot{{
			Min: 0, Max: 10, RiskLevel: "low", Conclusion: "low", Suggestion: "watch",
		}},
	}}
}

func cloneLifecycleFactorSnapshots(factors []scalesnapshot.FactorSnapshot) []scalesnapshot.FactorSnapshot {
	out := make([]scalesnapshot.FactorSnapshot, 0, len(factors))
	for _, factor := range factors {
		copied := factor
		copied.QuestionCodes = append([]string(nil), factor.QuestionCodes...)
		copied.ScoringParams.CntOptionContents = append([]string(nil), factor.ScoringParams.CntOptionContents...)
		copied.InterpretRules = append([]scalesnapshot.InterpretRuleSnapshot(nil), factor.InterpretRules...)
		out = append(out, copied)
	}
	return out
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
