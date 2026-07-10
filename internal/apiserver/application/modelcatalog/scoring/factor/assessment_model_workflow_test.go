package factor

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/authoring"
	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/assessmentstore"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

func TestAddFactorUsesAssessmentModelRepositoryWhenConfigured(t *testing.T) {
	ctx := context.Background()
	model := newDraftAssessmentModelForFactorTest(t)
	modelRepo := &factorAssessmentModelRepoStub{model: model}
	publisher := &scaleEventPublisherStub{}
	svc := NewService(modelRepo, nil, publisher)

	got, err := svc.AddFactor(ctx, shared.AddFactorDTO{
		ScaleCode:     model.Code,
		Code:          "F2",
		Title:         "Factor 2",
		QuestionCodes: []string{"Q2"},
	})
	if err != nil {
		t.Fatalf("AddFactor() error = %v", err)
	}
	if modelRepo.updateCount < 1 {
		t.Fatalf("model repo Update calls = %d, want at least 1", modelRepo.updateCount)
	}
	if len(model.DefinitionV2.Measure.Factors) != 2 {
		t.Fatalf("measure factors = %#v, want 2 factors", model.DefinitionV2.Measure.Factors)
	}
	if got == nil || len(got.Factors) != 2 {
		t.Fatalf("result factors = %#v, want 2 factors", got)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("published event count = %d, want 1", len(publisher.events))
	}
}

func TestReplaceFactorsWithActorSavesDefinitionV2ThroughAuthoring(t *testing.T) {
	t.Parallel()
	model := newDraftAssessmentModelForFactorTest(t)
	modelRepo := &factorAssessmentModelRepoStub{model: model}
	authoringService := authoring.Service{
		ModelRepo:  modelRepo,
		Authorizer: allowFactorAuthorizer{},
		Registry:   appdefinition.NewRegistry(assessmentstore.DefinitionHandler{}),
	}
	svc := NewService(modelRepo, nil, &scaleEventPublisherStub{}, WithDefinitionAuthoring(authoringService))

	result, err := svc.ReplaceFactorsWithActor(context.Background(), modelcatalog.ActorContext{}, model.Code, []shared.FactorDTO{{
		Code: "total", Title: "Total", IsTotalScore: true, QuestionCodes: []string{"Q1"}, ScoringStrategy: "sum",
		InterpretRules: []shared.InterpretRuleDTO{{MinScore: 0, MaxScore: 10, RiskLevel: "low", Conclusion: "low"}},
	}})
	if err != nil {
		t.Fatalf("ReplaceFactorsWithActor() error = %v", err)
	}
	if modelRepo.updateCount != 1 || model.DefinitionV2 == nil || len(model.DefinitionV2.Measure.Factors) != 1 || model.DefinitionV2.Measure.Factors[0].Code != "total" {
		t.Fatalf("saved definition = %#v, updates = %d", model.DefinitionV2, modelRepo.updateCount)
	}
	if result == nil || len(result.Factors) != 1 || result.Factors[0].Code != "total" {
		t.Fatalf("result = %#v", result)
	}
}

type allowFactorAuthorizer struct{}

func (allowFactorAuthorizer) Authorize(context.Context, modelcatalog.ActorContext, modelcatalog.Action, modelcatalog.Resource) error {
	return nil
}

func TestAddFactorForksPublishedAssessmentModelDraft(t *testing.T) {
	ctx := context.Background()
	model := newDraftAssessmentModelForFactorTest(t)
	model.Status = domain.ModelStatusPublished
	publishedAt := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	model.PublishedAt = &publishedAt
	modelRepo := &factorAssessmentModelRepoStub{model: model}
	svc := NewService(modelRepo, nil, &scaleEventPublisherStub{})

	if _, err := svc.AddFactor(ctx, shared.AddFactorDTO{
		ScaleCode:     model.Code,
		Code:          "F2",
		Title:         "Factor 2",
		QuestionCodes: []string{"Q2"},
	}); err != nil {
		t.Fatalf("AddFactor() error = %v", err)
	}
	if model.Status != domain.ModelStatusDraft {
		t.Fatalf("status = %s, want draft after fork", model.Status)
	}
	var snapshot scalesnapshot.ScaleSnapshot
	if err := json.Unmarshal(model.Definition.Data, &snapshot); err != nil {
		t.Fatalf("unmarshal definition payload: %v", err)
	}
	if snapshot.ScaleVersion != "1.0.1" || snapshot.Status != "draft" {
		t.Fatalf("forked payload = version %q status %q, want 1.0.1 draft", snapshot.ScaleVersion, snapshot.Status)
	}
}

func newDraftAssessmentModelForFactorTest(t *testing.T) *domain.AssessmentModel {
	t.Helper()
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	model, err := legacyadapter.AssessmentModelFromCreateDTO(shared.CreateScaleDTO{
		Code:                 "SCL_FACTOR",
		Title:                "Factor Scale",
		QuestionnaireCode:    "Q1",
		QuestionnaireVersion: "1.0",
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
		Status:               string(model.Status),
		Factors: []scalesnapshot.FactorSnapshot{{
			Code:            "F1",
			Title:           "Factor 1",
			QuestionCodes:   []string{"Q1"},
			ScoringStrategy: "sum",
			InterpretRules: []scalesnapshot.InterpretRuleSnapshot{{
				Min: 0, Max: 10, RiskLevel: "low", Conclusion: "low", Suggestion: "watch",
			}},
		}},
	}
	payload, err := legacyadapter.DefinitionPayloadFromScaleSnapshot(snapshot)
	if err != nil {
		t.Fatalf("DefinitionPayloadFromScaleSnapshot() error = %v", err)
	}
	if err := model.UpdateDefinitionWithV2(payload, scalesnapshot.DefinitionFromScaleSnapshot(snapshot), now); err != nil {
		t.Fatalf("UpdateDefinitionWithV2() error = %v", err)
	}
	return model
}

type factorAssessmentModelRepoStub struct {
	model       *domain.AssessmentModel
	updateCount int
}

func (r *factorAssessmentModelRepoStub) Create(context.Context, *domain.AssessmentModel) error {
	return nil
}

func (r *factorAssessmentModelRepoStub) Update(_ context.Context, model *domain.AssessmentModel) error {
	r.updateCount++
	r.model = model
	return nil
}

func (r *factorAssessmentModelRepoStub) FindByCode(_ context.Context, code string) (*domain.AssessmentModel, error) {
	if r.model == nil || r.model.Code != code {
		return nil, domain.ErrNotFound
	}
	return r.model, nil
}

func (r *factorAssessmentModelRepoStub) FindByQuestionnaireCode(context.Context, domain.Kind, string) (*domain.AssessmentModel, error) {
	return nil, domain.ErrNotFound
}

func (r *factorAssessmentModelRepoStub) List(context.Context, modelcatalogport.ListFilter) ([]*domain.AssessmentModel, int64, error) {
	return nil, 0, nil
}

func (r *factorAssessmentModelRepoStub) Delete(context.Context, string) error { return nil }

var _ modelcatalogport.ModelRepository = (*factorAssessmentModelRepoStub)(nil)
