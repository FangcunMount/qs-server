package definition

import (
	"context"
	"testing"
	"time"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestScaleDefinitionHandlerMaterializesDefinitionV2(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{Code: "SCALE_A", Kind: domain.KindScale, Algorithm: domain.AlgorithmScaleDefault, Title: "Scale A", Now: now})
	if err != nil {
		t.Fatalf("NewAssessmentModel: %v", err)
	}
	definition := completeScaleDefinition()
	result, err := (ScaleDefinitionHandler{}).MaterializeSnapshot(context.Background(), modelWithDefinition(model, definition))
	if err != nil {
		t.Fatalf("MaterializeSnapshot: %v", err)
	}
	if result.AlgorithmFamily != domain.AlgorithmFamilyFactorScoring || result.DecisionKind != domain.DecisionKindScoreRange {
		t.Fatalf("snapshot result = %#v", result)
	}
}

func TestScaleValidateForPublishRejectsMissingFactorsAndDecision(t *testing.T) {
	t.Parallel()
	model := publishableScaleShell()
	model.DefinitionV2 = &modeldefinition.Definition{}
	issues := (ScaleDefinitionHandler{}).ValidateForPublish(context.Background(), model)
	if !hasIssueCode(issues, "measure.factors.required") {
		t.Fatalf("issues = %#v, want measure.factors.required", issues)
	}
	if !hasIssueCode(issues, "definition_v2.decision.invalid") {
		t.Fatalf("issues = %#v, want definition_v2.decision.invalid", issues)
	}
}

func TestScaleValidateForPublishAcceptsRiskDecision(t *testing.T) {
	t.Parallel()
	model := publishableScaleShell()
	model.DefinitionV2 = completeScaleDefinition()
	handler := ScaleDefinitionHandler{QuestionnaireQuery: publishedQuestionnaireStub("Q", "1",
		questionnaireapp.QuestionResult{Code: "Q1", Type: "single_choice", Options: []questionnaireapp.OptionResult{{Value: "A"}, {Value: "B"}}},
	)}
	issues := handler.ValidateForPublish(context.Background(), model)
	if domain.HasValidationErrors(issues) {
		t.Fatalf("ValidateForPublish issues = %#v", issues)
	}
}

func TestScaleValidateForPublishRejectsUnsupportedStrategy(t *testing.T) {
	t.Parallel()
	model := publishableScaleShell()
	model.DefinitionV2 = completeScaleDefinition()
	model.DefinitionV2.Measure.Scoring[0].Strategy = factor.ScoringStrategyWeightedSum
	handler := ScaleDefinitionHandler{QuestionnaireQuery: publishedQuestionnaireStub("Q", "1",
		questionnaireapp.QuestionResult{Code: "Q1", Type: "single_choice", Options: []questionnaireapp.OptionResult{{Value: "A"}, {Value: "B"}}},
	)}
	issues := handler.ValidateForPublish(context.Background(), model)
	if !hasIssueCode(issues, "strategy.unsupported_for_path") {
		t.Fatalf("issues = %#v, want strategy.unsupported_for_path", issues)
	}
}

func TestScaleValidateForPublishRejectsMissingExecutableScoring(t *testing.T) {
	t.Parallel()
	model := publishableScaleShell()
	model.DefinitionV2 = completeScaleDefinition()
	model.DefinitionV2.Measure.Scoring = nil
	handler := ScaleDefinitionHandler{QuestionnaireQuery: publishedQuestionnaireStub("Q", "1",
		questionnaireapp.QuestionResult{Code: "Q1", Type: "single_choice", Options: []questionnaireapp.OptionResult{{Value: "A"}, {Value: "B"}}},
	)}
	issues := handler.ValidateForPublish(context.Background(), model)
	if !hasIssueCode(issues, "factor.scoring.executable_required") {
		t.Fatalf("issues = %#v, want factor.scoring.executable_required", issues)
	}
}

func TestScaleValidateForPublishRejectsNonZeroScoringConstant(t *testing.T) {
	t.Parallel()
	model := publishableScaleShell()
	model.DefinitionV2 = completeScaleDefinition()
	model.DefinitionV2.Measure.Scoring[0].Constant = 10
	handler := ScaleDefinitionHandler{QuestionnaireQuery: publishedQuestionnaireStub("Q", "1",
		questionnaireapp.QuestionResult{Code: "Q1", Type: "single_choice", Options: []questionnaireapp.OptionResult{{Value: "A"}, {Value: "B"}}},
	)}
	issues := handler.ValidateForPublish(context.Background(), model)
	if !hasIssueCode(issues, "factor.scoring.constant.unsupported") {
		t.Fatalf("issues = %#v, want factor.scoring.constant.unsupported", issues)
	}
}

func TestScaleValidateForPublishRejectsUnknownQuestionRef(t *testing.T) {
	t.Parallel()
	model := publishableScaleShell()
	model.DefinitionV2 = completeScaleDefinition()
	handler := ScaleDefinitionHandler{QuestionnaireQuery: publishedQuestionnaireStub("Q", "1",
		questionnaireapp.QuestionResult{Code: "OTHER", Type: "single_choice", Options: []questionnaireapp.OptionResult{{Value: "A"}}},
	)}
	issues := handler.ValidateForPublish(context.Background(), model)
	if !hasIssueCode(issues, "question_mapping.question_not_found") {
		t.Fatalf("issues = %#v, want question_mapping.question_not_found", issues)
	}
}

func publishableScaleShell() *domain.AssessmentModel {
	return &domain.AssessmentModel{
		Kind:      domain.KindScale,
		Algorithm: domain.AlgorithmScaleDefault,
		Code:      "SCALE_SHELL",
		Title:     "Scale",
		Binding:   domain.QuestionnaireBinding{QuestionnaireCode: "Q", QuestionnaireVersion: "1"},
	}
}

func completeScaleDefinition() *modeldefinition.Definition {
	return &modeldefinition.Definition{
		Measure: modeldefinition.MeasureSpec{
			Factors: []factor.Factor{{Code: "TOTAL", Title: "Total", Role: factor.FactorRoleTotal}},
			Scoring: []factor.Scoring{{FactorCode: "TOTAL", Strategy: factor.ScoringStrategySum, Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "Q1"}}}},
		},
		Outcomes: []conclusion.Outcome{{Code: "low", Title: "Low"}},
		Conclusions: []conclusion.Conclusion{conclusion.RiskConclusion{
			FactorCode: "TOTAL",
			Rules:      []conclusion.ScoreRangeOutcome{{MinScore: 0, MaxScore: 10, OutcomeCode: "low", MaxInclusive: true}},
		}},
	}
}

func hasIssueCode(issues []domain.DomainValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func modelWithDefinition(model *domain.AssessmentModel, value *modeldefinition.Definition) *domain.AssessmentModel {
	clone := *model
	clone.DefinitionV2 = value
	return &clone
}
