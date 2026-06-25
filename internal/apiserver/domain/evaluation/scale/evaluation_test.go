package scale

import (
	"context"
	"strings"
	"testing"

	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/snapshot"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestEvaluatorCalculatesSumAvgCntAndUsesTotalScoreFactor(t *testing.T) {
	input := scaleInterpretationInputForTest()

	result, err := NewDefaultEvaluator().Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	assertFactorScore(t, result.FactorScores, "total", 8)
	assertFactorScore(t, result.FactorScores, "avg", 4)
	assertFactorScore(t, result.FactorScores, "cnt", 1)
	if result.TotalScore != 8 {
		t.Fatalf("total score = %.1f, want total factor score 8.0", result.TotalScore)
	}
}

func TestEvaluatorSumsFactorsWhenNoTotalScoreFactor(t *testing.T) {
	input := scaleInterpretationInputForTest()
	for i := range input.Scale.Factors {
		input.Scale.Factors[i].IsTotalScore = false
	}

	result, err := NewDefaultEvaluator().Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if result.TotalScore != 13 {
		t.Fatalf("total score = %.1f, want sum of factors 13.0", result.TotalScore)
	}
}

func TestEvaluatorRiskMatchingAndOverallFallback(t *testing.T) {
	input := scaleInterpretationInputForTest()
	input.AnswerSheet.Answers[0].Score = 40
	input.AnswerSheet.Answers[1].Score = 50

	result, err := NewDefaultEvaluator().Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if result.RiskLevel != RiskLevelSevere {
		t.Fatalf("overall risk = %s, want severe from total factor rule", result.RiskLevel)
	}

	input.Scale.Factors[0].IsTotalScore = false
	input.Scale.Factors[0].InterpretRules = nil
	result, err = NewDefaultEvaluator().Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if result.RiskLevel != RiskLevelSevere {
		t.Fatalf("overall risk = %s, want severe from highest factor default risk", result.RiskLevel)
	}
}

func TestEvaluatorInterpretationUsesRulesAndDefaults(t *testing.T) {
	input := scaleInterpretationInputForTest()

	result, err := NewDefaultEvaluator().Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if result.Conclusion != "overall low" || result.Suggestion != "keep" {
		t.Fatalf("overall interpretation = %q/%q, want rule content", result.Conclusion, result.Suggestion)
	}
	got := findFactorScoreForTest(result.FactorScores, "avg")
	if got.Conclusion != "avg得分4.0分，处于正常水平" {
		t.Fatalf("factor default conclusion = %q", got.Conclusion)
	}
	if got.Suggestion != "状态良好，继续保持" {
		t.Fatalf("factor default suggestion = %q", got.Suggestion)
	}
}

func TestEvaluatorReturnsCollectAndScoringErrors(t *testing.T) {
	t.Run("answer sheet required", func(t *testing.T) {
		input := scaleInterpretationInputForTest()
		input.AnswerSheet = nil

		_, err := NewDefaultEvaluator().Evaluate(context.Background(), input)
		if err == nil || !strings.Contains(err.Error(), "answer sheet is required") {
			t.Fatalf("Evaluate error = %v, want answer sheet required", err)
		}
	})

	t.Run("cnt requires questionnaire", func(t *testing.T) {
		input := scaleInterpretationInputForTest()
		input.Questionnaire = nil

		_, err := NewDefaultEvaluator().Evaluate(context.Background(), input)
		if err == nil || !strings.Contains(err.Error(), "questionnaire is required") {
			t.Fatalf("Evaluate error = %v, want questionnaire required", err)
		}
	})

	t.Run("unsupported strategy", func(t *testing.T) {
		input := scaleInterpretationInputForTest()
		input.Scale.Factors[0].ScoringStrategy = "unknown"

		_, err := NewDefaultEvaluator().Evaluate(context.Background(), input)
		if err == nil || !strings.Contains(err.Error(), "unsupported factor scoring strategy") {
			t.Fatalf("Evaluate error = %v, want unsupported strategy", err)
		}
	})
}

func scaleInterpretationInputForTest() ScaleInterpretationInput {
	return ScaleInterpretationInput{
		Scale: ScaleInterpretationModel{
			Code: "S-001",
			Factors: []scalesnapshot.FactorSnapshot{
				{
					Code:            "total",
					Title:           "total",
					IsTotalScore:    true,
					QuestionCodes:   []string{"q1", "q2"},
					ScoringStrategy: string(ScoringStrategySum),
					InterpretRules: []scalesnapshot.InterpretRuleSnapshot{
						{Min: 0, Max: 10, RiskLevel: string(RiskLevelLow), Conclusion: "overall low", Suggestion: "keep"},
						{Min: 10, Max: 100, RiskLevel: string(RiskLevelSevere), Conclusion: "overall severe", Suggestion: "help"},
					},
				},
				{
					Code:            "avg",
					Title:           "avg",
					QuestionCodes:   []string{"q1", "q2"},
					ScoringStrategy: string(ScoringStrategyAvg),
				},
				{
					Code:            "cnt",
					Title:           "cnt",
					QuestionCodes:   []string{"q1", "q2"},
					ScoringStrategy: string(ScoringStrategyCnt),
					ScoringParams:   scalesnapshot.ScoringParamsSnapshot{CntOptionContents: []string{"是"}},
				},
			},
		},
		AnswerSheet: &ScaleAnswerSheetSnapshot{
			Answers: []ScaleAnswerSnapshot{
				{QuestionCode: meta.NewCode("q1"), Score: 3, Value: "a"},
				{QuestionCode: meta.NewCode("q2"), Score: 5, Value: "b"},
			},
		},
		Questionnaire: &ScaleQuestionnaireSnapshot{
			Questions: []ScaleQuestionSnapshot{
				{Code: meta.NewCode("q1"), Options: []ScaleOptionSnapshot{{Code: "a", Content: "是"}}},
				{Code: meta.NewCode("q2"), Options: []ScaleOptionSnapshot{{Code: "b", Content: "否"}}},
			},
		},
	}
}

func assertFactorScore(t *testing.T, scores []ScaleFactorScore, code string, want float64) {
	t.Helper()
	got := findFactorScoreForTest(scores, code)
	if got.RawScore != want {
		t.Fatalf("factor %s raw score = %.1f, want %.1f", code, got.RawScore, want)
	}
}

func findFactorScoreForTest(scores []ScaleFactorScore, code string) ScaleFactorScore {
	for _, score := range scores {
		if score.FactorCode == code {
			return score
		}
	}
	return ScaleFactorScore{}
}
