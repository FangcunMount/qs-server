package characterization_test

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	factorscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/scoring"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rendering"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

// V1 contract (default interpretation path): when factors have no matching
// interpret rule, the report falls back to the default conclusion/suggestion
// text. This locks the exact wording so it survives moving the text generation
// from evaluation to interpretation.
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

	report := buildPreviewReport(t, interpretationreporting.NewFactorScoringBuilder(domainreport.NewDefaultReportBuilder(nil)), previewOutcome(t, a, snapshot, execution, evaluationfact.RuntimeIdentity{}))

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
				},
				{
					Code:            "mood",
					Title:           "情绪",
					QuestionCodes:   []string{"q2"},
					ScoringStrategy: "sum",
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
