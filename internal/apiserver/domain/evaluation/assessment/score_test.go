package assessment

import (
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestScaleScoreProjectionFromOutcomeMatchesLegacyPath(t *testing.T) {
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
					FactorCode:   NewFactorCode("total"),
					FactorName:   "Total",
					RawScore:     12.5,
					RiskLevel:    RiskLevelMedium,
					IsTotalScore: true,
				},
				{
					FactorCode: NewFactorCode("sleep"),
					FactorName: "睡眠",
					RawScore:   2,
					RiskLevel:  RiskLevelLow,
				},
			},
		},
	)
	legacy.TotalScore = score
	legacy.RiskLevel = RiskLevelMedium
	legacy.FactorScores = legacy.Detail.Payload.([]FactorScoreResult)

	outcome := AssessmentOutcomeFromEvaluationResult(legacy)
	assessmentID := NewID(42)

	native := ScaleScoreProjectionFromOutcome(assessmentID, outcome)
	viaLegacy := ScaleScoreProjectionFromEvaluationResult(assessmentID, legacy)
	assertScaleScoreProjectionEqual(t, native, viaLegacy)
}

func assertScaleScoreProjectionEqual(t *testing.T, got, want *ScaleScoreProjection) {
	t.Helper()
	if got == nil || want == nil {
		t.Fatalf("projection = (%v, %v), want both non-nil", got, want)
	}
	if got.AssessmentID() != want.AssessmentID() {
		t.Fatalf("assessment id = %v, want %v", got.AssessmentID(), want.AssessmentID())
	}
	if got.TotalScore() != want.TotalScore() {
		t.Fatalf("total score = %v, want %v", got.TotalScore(), want.TotalScore())
	}
	if got.RiskLevel() != want.RiskLevel() {
		t.Fatalf("risk level = %s, want %s", got.RiskLevel(), want.RiskLevel())
	}
	if !reflect.DeepEqual(got.FactorScores(), want.FactorScores()) {
		t.Fatalf("factor scores = %#v, want %#v", got.FactorScores(), want.FactorScores())
	}
}
