package characterization_test

import (
	"context"
	"testing"

	factorscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/scoring"
	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	interpretationbuilder "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/builder"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rendering"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

// V1 contract: scale risk/conclusion text comes from configured interpret rules.
// Hardcoded absolute-score risk fallback was removed (MC-R004); fixtures must
// encode the ranges that produce stable report wording.
func TestV1ScaleDefaultInterpretationTextIsStable(t *testing.T) {
	a := submittedScaleAssessment(t)
	snapshot := scaleDefaultInterpretInputSnapshot()

	execution, err := factorscoring.NewExecutor(nil).Execute(context.Background(), evaluationexecute.ExecutionInput{
		Assessment: a,
		Input:      snapshot,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	report := buildPreviewReport(t, interpretationreporting.NewFactorScoringBuilder(interpretationbuilder.NewDefaultReportBuilder()), previewOutcome(t, a, snapshot, execution, evaluationfact.RuntimeIdentity{}))

	if report.Conclusion() != "总分得分5.0分，处于正常水平" {
		t.Fatalf("Conclusion = %q", report.Conclusion())
	}

	dims := report.Dimensions()
	if len(dims) != 2 {
		t.Fatalf("len(Dimensions) = %d, want 2", len(dims))
	}
	assertDimensionField(t, dims[0], "总分", 5, domainreport.RiskLevelNone, "总分得分5.0分，处于正常水平")
	assertDimensionField(t, dims[1], "情绪", 45, domainreport.RiskLevelMedium, "情绪得分45.0分，处于中等水平")

	suggestions := report.Suggestions()
	assertSuggestionExists(t, suggestions, domainreport.SuggestionCategoryGeneral, "状态良好，继续保持")
	assertSuggestionExists(t, suggestions, domainreport.SuggestionCategoryDimension, "建议关注相关方面，适当调整生活方式")
}

func scaleDefaultInterpretInputSnapshot() *evaluationinput.InputSnapshot {
	return &evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:    evaluationinput.EvaluationModelKindScale,
			Code:    "S-001",
			Version: "1.0.0",
			Title:   "Scale",
		},
		ModelPayload: evaluationinput.ScaleModelPayload{Scale: &scalesnapshot.ScaleSnapshot{
			Code:                 "S-001",
			Title:                "Scale",
			ScaleVersion:         "1.0.0",
			QuestionnaireCode:    "Q-001",
			QuestionnaireVersion: "1.0.0",
			Status:               "published",
			Factors: []scalesnapshot.FactorSnapshot{
				{
					Code:            "total",
					Title:           "总分",
					IsTotalScore:    true,
					QuestionCodes:   []string{"q1"},
					ScoringStrategy: "sum",
					InterpretRules: []scalesnapshot.InterpretRuleSnapshot{
						{Min: 0, Max: 20, RiskLevel: "none", Conclusion: "总分得分5.0分，处于正常水平", Suggestion: "状态良好，继续保持"},
						{Min: 20, Max: 100, RiskLevel: "medium", Conclusion: "总分偏高", Suggestion: "建议复查"},
					},
				},
				{
					Code:            "mood",
					Title:           "情绪",
					QuestionCodes:   []string{"q2"},
					ScoringStrategy: "sum",
					InterpretRules: []scalesnapshot.InterpretRuleSnapshot{
						{Min: 0, Max: 40, RiskLevel: "low", Conclusion: "情绪得分较低", Suggestion: "保持"},
						{Min: 40, Max: 60, RiskLevel: "medium", Conclusion: "情绪得分45.0分，处于中等水平", Suggestion: "建议关注相关方面，适当调整生活方式"},
						{Min: 60, Max: 100, RiskLevel: "high", Conclusion: "情绪得分偏高", Suggestion: "及时干预"},
					},
				},
			},
		}},
		AnswerSheet: &evaluationinput.AnswerSheetSnapshot{
			QuestionnaireCode:    "Q-001",
			QuestionnaireVersion: "1.0.0",
			Answers: []evaluationinput.AnswerSnapshot{
				{QuestionCode: "q1", Score: 5},
				{QuestionCode: "q2", Score: 45},
			},
		},
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "Q-001", Version: "1.0.0"},
	}
}
