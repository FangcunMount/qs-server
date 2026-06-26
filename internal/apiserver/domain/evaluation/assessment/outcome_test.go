package assessment

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestAssessmentOutcomeRoundTripFromEvaluationResult(t *testing.T) {
	level := string(RiskLevelMedium)
	score := 12.5
	legacy := NewModelEvaluationResult(
		NewEvaluationModelRef(EvaluationModelKindScale, meta.ID(1), meta.NewCode("PHQ9"), "1.0.0", "PHQ9"),
		ResultSummary{
			PrimaryLabel: "medium",
			Score:        &score,
			Level:        &level,
		},
		EvaluationDetail{
			Kind: EvaluationModelKindScale,
			Payload: []FactorScoreResult{
				{
					FactorCode: NewFactorCode("total"),
					FactorName: "Total",
					RawScore:   12.5,
					RiskLevel:  RiskLevelMedium,
					IsTotalScore: true,
				},
			},
		},
	)
	legacy.TotalScore = score
	legacy.RiskLevel = RiskLevelMedium
	legacy.FactorScores = legacy.Detail.Payload.([]FactorScoreResult)

	outcome := AssessmentOutcomeFromEvaluationResult(legacy)
	if outcome == nil || outcome.Primary == nil || outcome.Primary.Value != score {
		t.Fatalf("outcome primary = %#v, want score %.1f", outcome, score)
	}
	if outcome.Level == nil || outcome.Level.Code != string(RiskLevelMedium) {
		t.Fatalf("outcome level = %#v", outcome.Level)
	}
	if len(outcome.Dimensions) != 1 {
		t.Fatalf("dimensions = %d, want 1", len(outcome.Dimensions))
	}

	back := outcome.ToEvaluationResult()
	if back.TotalScore != score || back.RiskLevel != RiskLevelMedium {
		t.Fatalf("legacy projection = (%v, %s)", back.TotalScore, back.RiskLevel)
	}
	if len(back.FactorScores) != 1 || back.FactorScores[0].FactorCode.String() != "total" {
		t.Fatalf("factor scores = %#v", back.FactorScores)
	}
}
