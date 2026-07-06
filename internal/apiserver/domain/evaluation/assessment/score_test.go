package assessment

import (
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
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

func TestScaleScoreProjectionFromOutcomeRejectsNonScaleOutcome(t *testing.T) {
	outcome := NewAssessmentOutcome(
		NewEvaluationModelRefWithIdentity(
			EvaluationModelKindPersonality,
			modelcatalog.SubKindTypology,
			modelcatalog.AlgorithmMBTI,
			meta.ID(0),
			meta.NewCode("MBTI_TEST"),
			"1.0.0",
			"MBTI",
		),
		ResultSummary{PrimaryLabel: "INTJ"},
		EvaluationDetail{Kind: EvaluationModelKindPersonality},
	)
	outcome.Primary = &OutcomeScoreValue{Kind: OutcomeScoreKindMatchPercent, Value: 92}
	outcome.Level = &OutcomeResultLevel{Code: "INTJ", Label: "建筑师", Severity: "none"}

	if got := ScaleScoreProjectionFromOutcome(NewID(1), outcome); got != nil {
		t.Fatalf("projection = %#v, want nil for non-scale outcome", got)
	}
}

func TestApplyOutcomeDoesNotTreatTypeCodeAsRiskLevel(t *testing.T) {
	modelRef := NewEvaluationModelRefWithIdentity(
		EvaluationModelKindPersonality,
		modelcatalog.SubKindTypology,
		modelcatalog.AlgorithmMBTI,
		meta.ID(0),
		meta.NewCode("MBTI_TEST"),
		"1.0.0",
		"MBTI",
	)
	a, err := NewAssessment(
		1,
		testee.NewID(1),
		NewQuestionnaireRefByCode(meta.NewCode("MBTI_TEST"), "1.0.0"),
		NewAnswerSheetRef(meta.FromUint64(1)),
		NewAdhocOrigin(),
		WithEvaluationModel(modelRef),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	score := 92.0
	outcome := NewAssessmentOutcome(
		modelRef,
		ResultSummary{PrimaryLabel: "INTJ", Score: &score},
		EvaluationDetail{Kind: EvaluationModelKindPersonality},
	)
	outcome.Primary = &OutcomeScoreValue{Kind: OutcomeScoreKindMatchPercent, Value: score, Label: "INTJ"}
	outcome.Level = &OutcomeResultLevel{Code: "INTJ", Label: "建筑师", Severity: "none"}
	outcome.Profile = &ProfileResult{Kind: ProfileKindPersonalityType, Code: "INTJ", Name: "建筑师"}

	if err := a.ApplyOutcome(outcome); err != nil {
		t.Fatalf("ApplyOutcome: %v", err)
	}
	if a.RiskLevel() != nil {
		t.Fatalf("risk level = %v, want nil for typology type code", *a.RiskLevel())
	}
}

func TestToEvaluationResultDoesNotProjectTypeCodeToRiskLevel(t *testing.T) {
	outcome := NewAssessmentOutcome(
		EvaluationModelRef{},
		ResultSummary{},
		EvaluationDetail{Kind: EvaluationModelKindPersonality},
	)
	outcome.Level = &OutcomeResultLevel{Code: "INTJ", Label: "建筑师", Severity: "none"}

	legacy := outcome.ToEvaluationResult()
	if legacy.RiskLevel != "" && legacy.RiskLevel != RiskLevelNone {
		t.Fatalf("RiskLevel = %s, want empty/none for type code", legacy.RiskLevel)
	}
	if legacy.Summary.PrimaryLabel != "建筑师" {
		t.Fatalf("PrimaryLabel = %q, want 建筑师", legacy.Summary.PrimaryLabel)
	}
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
