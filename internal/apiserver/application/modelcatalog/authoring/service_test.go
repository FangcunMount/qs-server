package authoring

import (
	"context"
	"testing"
	"time"

	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestSaveDefinitionMaterializesDefinitionV2(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code: "SCL-1", Kind: domain.KindScale, Algorithm: domain.AlgorithmScaleDefault, Title: "Scale", Now: now,
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel: %v", err)
	}
	definition := &modeldefinition.Definition{Measure: modeldefinition.MeasureSpec{
		Factors: []factor.Factor{{Code: "total", Title: "Total", Role: factor.FactorRoleTotal}},
		Scoring: []factor.Scoring{{FactorCode: "total", Strategy: factor.ScoringStrategySum, Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "q1"}}}},
	}}
	handler := &recordingDefinitionHandler{}
	repo := &authoringModelRepo{model: model}
	service := Service{
		ModelRepo:  repo,
		Authorizer: allowDefinitionAuthorizer{},
		Registry:   appdefinition.NewRegistry(handler),
		Now:        func() time.Time { return now },
	}

	got, err := service.SaveDefinition(context.Background(), modelcatalog.ActorContext{}, model.Code, definition)
	if err != nil {
		t.Fatalf("SaveDefinition: %v", err)
	}
	if got != definition || !handler.called {
		t.Fatalf("SaveDefinition result = %#v, handler called = %t", got, handler.called)
	}
	if repo.updates != 1 || model.DefinitionV2 != definition {
		t.Fatalf("model update = %#v, updates = %d", model, repo.updates)
	}
}

func TestSaveDefinitionForksPublishedModelToDraft(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code: "MEDICAL-SCALE", Kind: domain.KindScale, Algorithm: domain.AlgorithmScaleDefault, Title: "Scale", Now: now,
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel: %v", err)
	}
	if err := model.MarkPublished(now.Add(time.Minute)); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}
	publishedRevision := model.Revision()
	definition := &modeldefinition.Definition{Measure: modeldefinition.MeasureSpec{
		Factors: []factor.Factor{{Code: "total", Title: "Total", Role: factor.FactorRoleTotal}},
		Scoring: []factor.Scoring{{FactorCode: "total", Strategy: factor.ScoringStrategySum, Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "q1"}}}},
	}}
	repo := &authoringModelRepo{model: model}
	service := Service{
		ModelRepo: repo, Authorizer: allowDefinitionAuthorizer{}, Registry: appdefinition.NewRegistry(&recordingDefinitionHandler{}),
		Now: func() time.Time { return now.Add(2 * time.Minute) },
	}

	if _, err := service.SaveDefinition(context.Background(), modelcatalog.ActorContext{}, model.Code, definition); err != nil {
		t.Fatalf("SaveDefinition: %v", err)
	}
	if !model.IsDraft() {
		t.Fatalf("status = %s, want draft", model.Status)
	}
	if model.PublishedAt != nil {
		t.Fatalf("published_at = %v, want nil for the draft head", model.PublishedAt)
	}
	if got, want := model.Revision(), publishedRevision+1; got != want {
		t.Fatalf("revision = %d, want %d", got, want)
	}
}

func TestValidateDefinitionUsesThePublishValidationHandler(t *testing.T) {
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code: "TYPOLOGY-1", Kind: domain.KindTypology,
		Algorithm: domain.AlgorithmPersonalityTypology, Title: "Typology", Now: time.Now(),
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel: %v", err)
	}
	model.DefinitionV2 = &modeldefinition.Definition{}
	handler := &recordingDefinitionHandler{validateIssues: []domain.DomainValidationIssue{{
		Field: "decision.poles", Code: "decision.poles.required", Message: "poles required", Level: domain.ValidationLevelError,
	}}}
	service := Service{ModelRepo: &authoringModelRepo{model: model}, Authorizer: allowDefinitionAuthorizer{}, Registry: appdefinition.NewRegistry(handler)}

	result, err := service.ValidateDefinition(context.Background(), modelcatalog.ActorContext{}, model.Code)
	if err != nil {
		t.Fatalf("ValidateDefinition: %v", err)
	}
	if !handler.validateCalled || result.Passed || len(result.Issues) != 1 || result.Issues[0].Code != "decision.poles.required" {
		t.Fatalf("result=%#v validateCalled=%t", result, handler.validateCalled)
	}
}

func TestValidateDefinitionReturnsWarningsWithoutFailing(t *testing.T) {
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code: "TYPOLOGY-WARNING", Kind: domain.KindTypology,
		Algorithm: domain.AlgorithmPersonalityTypology, Title: "Typology", Now: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := &recordingDefinitionHandler{validateIssues: []domain.DomainValidationIssue{{
		Field: "factor_graph", Code: "question_contribution.legacy_implicit", Message: "legacy", Level: domain.ValidationLevelWarning,
	}}}
	service := Service{ModelRepo: &authoringModelRepo{model: model}, Authorizer: allowDefinitionAuthorizer{}, Registry: appdefinition.NewRegistry(handler)}
	result, err := service.ValidateDefinition(context.Background(), modelcatalog.ActorContext{}, model.Code)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed || !result.Valid || len(result.Issues) != 1 || len(result.Errors) != 0 {
		t.Fatalf("result = %#v, want passed result with warning", result)
	}
}

type recordingDefinitionHandler struct {
	called         bool
	validateCalled bool
	validateIssues []domain.DomainValidationIssue
}

func (*recordingDefinitionHandler) Supports(identity domain.Identity) bool {
	return identity.Kind == domain.KindScale || identity.Kind == domain.KindTypology
}

func (h *recordingDefinitionHandler) ValidateForPublish(context.Context, *domain.AssessmentModel) []domain.DomainValidationIssue {
	h.validateCalled = true
	return h.validateIssues
}

func (h *recordingDefinitionHandler) MaterializeSnapshot(context.Context, *domain.AssessmentModel) (appdefinition.Materialization, error) {
	h.called = true
	return appdefinition.Materialization{}, nil
}

type allowDefinitionAuthorizer struct{}

func (allowDefinitionAuthorizer) Authorize(context.Context, modelcatalog.ActorContext, modelcatalog.Action, modelcatalog.Resource) error {
	return nil
}

type authoringModelRepo struct {
	model   *domain.AssessmentModel
	updates int
}

func (r *authoringModelRepo) Create(context.Context, *domain.AssessmentModel) error { return nil }

func (r *authoringModelRepo) Update(context.Context, *domain.AssessmentModel) error {
	r.updates++
	return nil
}

func (r *authoringModelRepo) FindByCode(context.Context, string) (*domain.AssessmentModel, error) {
	return r.model, nil
}

func (*authoringModelRepo) FindByQuestionnaireCode(context.Context, domain.Kind, string) (*domain.AssessmentModel, error) {
	return nil, domain.ErrNotFound
}

func (*authoringModelRepo) List(context.Context, modelcatalogport.ListFilter) ([]*domain.AssessmentModel, int64, error) {
	return nil, 0, nil
}

func (*authoringModelRepo) Delete(context.Context, string) error { return nil }
