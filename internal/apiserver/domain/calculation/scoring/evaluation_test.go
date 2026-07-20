package scoring

import (
	"context"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestEvaluatorCalculatesSumAvgCntAndUsesTotalScoreFactor(t *testing.T) {
	input := scaleInputForTest()

	result, err := NewDefaultEvaluator().Score(context.Background(), input)
	if err != nil {
		t.Fatalf("Score returned error: %v", err)
	}

	assertFactorScore(t, result.FactorScores, "total", 8)
	assertFactorScore(t, result.FactorScores, "avg", 4)
	assertFactorScore(t, result.FactorScores, "cnt", 1)
	if result.TotalScore != 8 {
		t.Fatalf("total score = %.1f, want total factor score 8.0", result.TotalScore)
	}
}

func TestEvaluatorSumsFactorsWhenNoTotalScoreFactor(t *testing.T) {
	input := scaleInputForTest()
	for i := range input.Model.Factors {
		input.Model.Factors[i].IsTotalScore = false
	}

	result, err := NewDefaultEvaluator().Score(context.Background(), input)
	if err != nil {
		t.Fatalf("Score returned error: %v", err)
	}
	if result.TotalScore != 13 {
		t.Fatalf("total score = %.1f, want sum of factors 13.0", result.TotalScore)
	}
}

func TestEvaluatorRiskMatchingAndOverallFallback(t *testing.T) {
	input := scaleInputForTest()
	input.AnswerSheet.Answers[0].Score = 40
	input.AnswerSheet.Answers[1].Score = 50

	result, err := NewDefaultEvaluator().Score(context.Background(), input)
	if err != nil {
		t.Fatalf("Score returned error: %v", err)
	}
	if result.RiskLevel != RiskLevelSevere {
		t.Fatalf("overall risk = %s, want severe from total factor rule", result.RiskLevel)
	}

	input.Model.Factors[0].IsTotalScore = false
	for i := range input.Model.Factors {
		input.Model.Factors[i].InterpretRules = nil
	}
	result, err = NewDefaultEvaluator().Score(context.Background(), input)
	if err != nil {
		t.Fatalf("Score returned error: %v", err)
	}
	if result.RiskLevel != RiskLevelNone {
		t.Fatalf("overall risk = %s, want none without interpret rules or hardcoded fallback", result.RiskLevel)
	}
}

func TestEvaluatorReturnsCollectAndScoringErrors(t *testing.T) {
	t.Run("answer sheet required", func(t *testing.T) {
		input := scaleInputForTest()
		input.AnswerSheet = nil

		_, err := NewDefaultEvaluator().Score(context.Background(), input)
		if err == nil || !strings.Contains(err.Error(), "answer sheet is required") {
			t.Fatalf("Score error = %v, want answer sheet required", err)
		}
	})

	t.Run("cnt requires questionnaire", func(t *testing.T) {
		input := scaleInputForTest()
		input.Questionnaire = nil

		_, err := NewDefaultEvaluator().Score(context.Background(), input)
		if err == nil || !strings.Contains(err.Error(), "questionnaire is required") {
			t.Fatalf("Score error = %v, want questionnaire required", err)
		}
	})

	t.Run("unsupported strategy", func(t *testing.T) {
		input := scaleInputForTest()
		input.Model.Factors[0].ScoringStrategy = "unknown"

		_, err := NewDefaultEvaluator().Score(context.Background(), input)
		if err == nil || !strings.Contains(err.Error(), "unsupported factor scoring strategy") {
			t.Fatalf("Score error = %v, want unsupported strategy", err)
		}
	})
}

func scaleInputForTest() Input {
	return Input{
		Model: Model{
			Code: "S-001",
			Factors: []Factor{
				{
					Code:            "total",
					Title:           "total",
					IsTotalScore:    true,
					QuestionCodes:   []string{"q1", "q2"},
					ScoringStrategy: string(StrategySum),
					InterpretRules: []InterpretRule{
						{Min: 0, Max: 10, RiskLevel: string(RiskLevelLow), Conclusion: "overall low", Suggestion: "keep"},
						{Min: 10, Max: 100, RiskLevel: string(RiskLevelSevere), Conclusion: "overall severe", Suggestion: "help"},
					},
				},
				{
					Code:            "avg",
					Title:           "avg",
					QuestionCodes:   []string{"q1", "q2"},
					ScoringStrategy: string(StrategyAvg),
				},
				{
					Code:            "cnt",
					Title:           "cnt",
					QuestionCodes:   []string{"q1", "q2"},
					ScoringStrategy: string(StrategyCnt),
					ScoringParams:   CntParams{CntOptionContents: []string{"是"}},
				},
			},
		},
		AnswerSheet: &AnswerSheet{
			Answers: []Answer{
				{QuestionCode: meta.NewCode("q1"), Score: 3, Value: "a"},
				{QuestionCode: meta.NewCode("q2"), Score: 5, Value: "b"},
			},
		},
		Questionnaire: &Questionnaire{
			Questions: []Question{
				{Code: meta.NewCode("q1"), Options: []Option{{Code: "a", Content: "是"}}},
				{Code: meta.NewCode("q2"), Options: []Option{{Code: "b", Content: "否"}}},
			},
		},
	}
}

func assertFactorScore(t *testing.T, scores []FactorScore, code string, want float64) {
	t.Helper()
	got := findFactorScoreForTest(scores, code)
	if got.RawScore != want {
		t.Fatalf("factor %s raw score = %.1f, want %.1f", code, got.RawScore, want)
	}
}

func findFactorScoreForTest(scores []FactorScore, code string) FactorScore {
	for _, score := range scores {
		if score.FactorCode == code {
			return score
		}
	}
	return FactorScore{}
}
