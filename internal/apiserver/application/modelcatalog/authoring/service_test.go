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

func TestSaveDefinitionBuildsPayloadFromDefinitionV2(t *testing.T) {
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
	if repo.updates != 1 || model.DefinitionV2 != definition || string(model.Definition.Data) != `{"source":"definition_v2"}` {
		t.Fatalf("model update = %#v, updates = %d", model, repo.updates)
	}
}

func TestValidateDefinitionUsesThePublishValidationHandler(t *testing.T) {
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code: "TYPOLOGY-1", Kind: domain.KindTypology, SubKind: domain.SubKindTypology,
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

func (h *recordingDefinitionHandler) BuildSnapshotPayload(context.Context, *domain.AssessmentModel) (appdefinition.SnapshotBuildResult, error) {
	h.called = true
	return appdefinition.SnapshotBuildResult{PayloadFormat: domain.PayloadFormatAssessmentScaleV1, Payload: []byte(`{"source":"definition_v2"}`)}, nil
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
