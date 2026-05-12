package evaluation

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestEvaluatorCalculatesSumAvgCntAndUsesTotalScoreFactor(t *testing.T) {
	input := scaleEvaluationInputForTest()

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
	input := scaleEvaluationInputForTest()
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
	input := scaleEvaluationInputForTest()
	input.AnswerSheet.Answers[0].Score = 40
	input.AnswerSheet.Answers[1].Score = 50

	result, err := NewDefaultEvaluator().Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if result.RiskLevel != scale.RiskLevelSevere {
		t.Fatalf("overall risk = %s, want severe from total factor rule", result.RiskLevel)
	}

	input.Scale.Factors[0].IsTotalScore = false
	input.Scale.Factors[0].InterpretRules = nil
	result, err = NewDefaultEvaluator().Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if result.RiskLevel != scale.RiskLevelSevere {
		t.Fatalf("overall risk = %s, want severe from highest factor default risk", result.RiskLevel)
	}
}

func TestEvaluatorInterpretationUsesRulesAndDefaults(t *testing.T) {
	input := scaleEvaluationInputForTest()

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

func scaleEvaluationInputForTest() ScaleEvaluationInput {
	return ScaleEvaluationInput{
		Scale: ScaleEvaluationModel{
			Code: "S-001",
			Factors: []scale.FactorSnapshot{
				{
					Code:            scale.NewFactorCode("total"),
					Title:           "total",
					IsTotalScore:    true,
					QuestionCodes:   []meta.Code{meta.NewCode("q1"), meta.NewCode("q2")},
					ScoringStrategy: scale.ScoringStrategySum,
					InterpretRules: []scale.InterpretationRule{
						scale.NewInterpretationRule(scale.NewScoreRange(0, 10), scale.RiskLevelLow, "overall low", "keep"),
						scale.NewInterpretationRule(scale.NewScoreRange(10, 100), scale.RiskLevelSevere, "overall severe", "help"),
					},
				},
				{
					Code:            scale.NewFactorCode("avg"),
					Title:           "avg",
					QuestionCodes:   []meta.Code{meta.NewCode("q1"), meta.NewCode("q2")},
					ScoringStrategy: scale.ScoringStrategyAvg,
				},
				{
					Code:            scale.NewFactorCode("cnt"),
					Title:           "cnt",
					QuestionCodes:   []meta.Code{meta.NewCode("q1"), meta.NewCode("q2")},
					ScoringStrategy: scale.ScoringStrategyCnt,
					ScoringParams:   scale.NewScoringParams().WithCntOptionContents([]string{"是"}),
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
		if string(score.FactorCode) == code {
			return score
		}
	}
	return ScaleFactorScore{}
}
