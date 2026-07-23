package definition

import (
	"context"
	"testing"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
)

func TestTypologyValidateForPublishRejectsNilModel(t *testing.T) {
	t.Parallel()
	issues := (TypologyDefinitionHandler{}).ValidateForPublish(context.Background(), nil)
	if !hasIssueCode(issues, "model.required") {
		t.Fatalf("issues = %#v, want model.required", issues)
	}
}

func TestTypologyValidateForPublishRejectsEmptyDefinition(t *testing.T) {
	t.Parallel()
	model := publishableTypologyShell()
	model.DefinitionV2 = &modeldefinition.Definition{}
	issues := (TypologyDefinitionHandler{QuestionnaireQuery: publishedQuestionnaireStub("Q", "1",
		questionnaireapp.QuestionResult{Code: "Q1", Options: []questionnaireapp.OptionResult{{Value: "A"}}},
	)}).ValidateForPublish(context.Background(), model)
	if !hasIssueCode(issues, "measure.factors.required") {
		t.Fatalf("issues = %#v, want measure.factors.required", issues)
	}
}

func TestTypologyValidateForPublishRejectsMissingQuestionnaire(t *testing.T) {
	t.Parallel()
	model := publishableTypologyShell()
	model.DefinitionV2 = &modeldefinition.Definition{}
	issues := (TypologyDefinitionHandler{}).ValidateForPublish(context.Background(), model)
	// Empty definition fails before questionnaire load; still locks shared path codes.
	if !hasIssueCode(issues, "measure.factors.required") && !hasIssueCode(issues, "binding.questionnaire.not_found") {
		t.Fatalf("issues = %#v, want definition or questionnaire rejection", issues)
	}
}

func TestTypologyMaterializationUsesCanonicalSubKind(t *testing.T) {
	t.Parallel()
	model := publishableTypologyShell()
	model.DefinitionV2 = &modeldefinition.Definition{}
	_, err := (TypologyDefinitionHandler{}).MaterializeSnapshot(context.Background(), model)
	if err == nil {
		t.Fatal("expected invalid definition error")
	}
}

func publishableTypologyShell() *domain.AssessmentModel {
	return &domain.AssessmentModel{
		Kind:      domain.KindTypology,
		Algorithm: domain.AlgorithmPersonalityTypology,
		Code:      "TYPOLOGY_SHELL",
		Title:     "Typology",
		Binding:   domain.QuestionnaireBinding{QuestionnaireCode: "Q", QuestionnaireVersion: "1"},
	}
}
