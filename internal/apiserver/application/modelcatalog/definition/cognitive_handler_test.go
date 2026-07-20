package definition

import (
	"context"
	"testing"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestCognitiveValidateForPublishRejectsMissingAbilityDecision(t *testing.T) {
	t.Parallel()
	model := publishableCognitiveShell()
	model.DefinitionV2 = cognitiveDefinitionWithoutAbility()
	handler := CognitiveDefinitionHandler{QuestionnaireQuery: publishedQuestionnaireStub("Q", "1",
		questionnaireapp.QuestionResult{Code: "Q1", Options: []questionnaireapp.OptionResult{{Value: "A"}}},
	)}
	issues := handler.ValidateForPublish(context.Background(), model)
	if !hasIssueCode(issues, "definition_v2.decision.invalid") {
		t.Fatalf("issues = %#v, want definition_v2.decision.invalid", issues)
	}
}

func TestCognitiveValidateForPublishAcceptsAbilityDecision(t *testing.T) {
	t.Parallel()
	model := publishableCognitiveShell()
	model.DefinitionV2 = cognitiveDefinitionWithAbility()
	handler := CognitiveDefinitionHandler{QuestionnaireQuery: publishedQuestionnaireStub("Q", "1",
		questionnaireapp.QuestionResult{Code: "Q1", Options: []questionnaireapp.OptionResult{{Value: "A"}}},
	)}
	issues := handler.ValidateForPublish(context.Background(), model)
	if domain.HasValidationErrors(issues) {
		t.Fatalf("ValidateForPublish issues = %#v", issues)
	}
}

func TestCognitiveValidateForPublishRejectsUnknownSPMOption(t *testing.T) {
	t.Parallel()
	model := publishableCognitiveShell()
	model.DefinitionV2 = cognitiveDefinitionWithAbility()
	handler := CognitiveDefinitionHandler{QuestionnaireQuery: publishedQuestionnaireStub("Q", "1",
		questionnaireapp.QuestionResult{Code: "Q1", Options: []questionnaireapp.OptionResult{{Value: "B"}}},
	)}
	issues := handler.ValidateForPublish(context.Background(), model)
	if !hasIssueCode(issues, "question_mapping.option_not_found") {
		t.Fatalf("issues = %#v, want question_mapping.option_not_found", issues)
	}
}

func TestCognitiveBuildSnapshotPayloadDefaultsAlgorithmAndDecision(t *testing.T) {
	t.Parallel()
	model := publishableCognitiveShell()
	model.Algorithm = ""
	model.DefinitionV2 = cognitiveDefinitionWithAbility()
	result, err := (CognitiveDefinitionHandler{}).BuildSnapshotPayload(context.Background(), model)
	if err != nil {
		t.Fatalf("BuildSnapshotPayload: %v", err)
	}
	if result.Algorithm != domain.AlgorithmSPM || result.DecisionKind != domain.DecisionKindAbilityLevel || len(result.Payload) == 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestCognitiveValidateForPublishRejectsMissingSPMExecution(t *testing.T) {
	t.Parallel()
	model := publishableCognitiveShell()
	model.DefinitionV2 = cognitiveDefinitionWithAbility()
	model.DefinitionV2.Execution.SPM = nil
	issues := (CognitiveDefinitionHandler{QuestionnaireQuery: publishedQuestionnaireStub("Q", "1",
		questionnaireapp.QuestionResult{Code: "Q1", Options: []questionnaireapp.OptionResult{{Value: "A"}}},
	)}).ValidateForPublish(context.Background(), model)
	if !hasIssueCode(issues, "spm.execution.required") {
		t.Fatalf("issues = %#v, want spm.execution.required", issues)
	}
}

func publishableCognitiveShell() *domain.AssessmentModel {
	return &domain.AssessmentModel{
		Kind:       domain.KindCognitive,
		Algorithm:  domain.AlgorithmSPM,
		Code:       "COG_SHELL",
		Title:      "Cognitive",
		Binding:    domain.QuestionnaireBinding{QuestionnaireCode: "Q", QuestionnaireVersion: "1"},
		Definition: domain.DefinitionPayload{Format: domain.PayloadFormatCognitiveDefaultV1, Data: []byte(`{}`)},
	}
}

func cognitiveDefinitionWithoutAbility() *modeldefinition.Definition {
	return &modeldefinition.Definition{
		Measure: modeldefinition.MeasureSpec{
			Factors: []factor.Factor{{Code: "TOTAL", Role: factor.FactorRoleTotal}},
		},
		Execution: modeldefinition.ExecutionSpec{SPM: &modeldefinition.SPMSpec{
			TimeLimitSeconds: 60,
			TotalFactorCode:  "TOTAL",
			ItemSets: []modeldefinition.SPMItemSet{{
				Code:  "A",
				Items: []modeldefinition.SPMItem{{QuestionCode: "Q1", CorrectOptionCode: "A"}},
			}},
		}},
	}
}

func cognitiveDefinitionWithAbility() *modeldefinition.Definition {
	def := cognitiveDefinitionWithoutAbility()
	def.Outcomes = []conclusion.Outcome{{Code: "average", Title: "Average"}}
	def.Conclusions = []conclusion.Conclusion{conclusion.AbilityConclusion{
		FactorCode: "TOTAL", ScoreBasis: conclusion.ScoreBasisRaw, Primary: true,
		Rules: []conclusion.ScoreRangeOutcome{{MinScore: 0, MaxScore: 10, OutcomeCode: "average", MaxInclusive: true}},
	}}
	return def
}
