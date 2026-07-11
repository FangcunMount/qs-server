package evaluation

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestApplyAssessmentOutcomeV2FieldsProjectsScaleRiskLevel(t *testing.T) {
	a := newSubmittedScaleAssessment(t)
	score := 18.5
	level := string(assessment.RiskLevelHigh)
	outcome := assessment.NewAssessmentOutcome(
		assessment.EvaluationModelRef{},
		assessment.ResultSummary{
			PrimaryLabel: "high",
			Score:        &score,
			Level:        &level,
		},
		assessment.EvaluationDetail{Kind: assessment.EvaluationModelKindScale},
	)
	outcome.Primary = &assessment.OutcomeScoreValue{
		Kind:  assessment.OutcomeScoreKindRawTotal,
		Value: score,
	}
	outcome.Level = &assessment.OutcomeResultLevel{
		Code:     "high",
		Label:    "high",
		Severity: "high",
	}
	if err := a.ApplyScoringOutcome(outcome); err != nil {
		t.Fatalf("ApplyScoringOutcome returned error: %v", err)
	}

	po := NewAssessmentMapper().ToPO(a)
	if po.PrimaryScoreKind == nil || *po.PrimaryScoreKind != string(assessment.OutcomeScoreKindRawTotal) {
		t.Fatalf("primary score kind = %v, want %s", po.PrimaryScoreKind, assessment.OutcomeScoreKindRawTotal)
	}
	if po.PrimaryScoreValue == nil || *po.PrimaryScoreValue != 18.5 {
		t.Fatalf("primary score value = %v, want 18.5", po.PrimaryScoreValue)
	}
	if po.LevelCode == nil || *po.LevelCode != "high" {
		t.Fatalf("level code = %v, want high", po.LevelCode)
	}
	if po.Severity == nil || *po.Severity != "high" {
		t.Fatalf("severity = %v, want high", po.Severity)
	}
	if po.EvaluationModelAlgorithm == nil || *po.EvaluationModelAlgorithm != "scale_default" {
		t.Fatalf("algorithm = %v, want scale_default", po.EvaluationModelAlgorithm)
	}
}

func TestApplyAssessmentOutcomeV2FieldsAtEvaluatedState(t *testing.T) {
	t.Parallel()

	a := newSubmittedScaleAssessment(t)
	outcome := assessment.NewAssessmentOutcome(
		*a.EvaluationModelRef(),
		assessment.ResultSummary{PrimaryLabel: "high"},
		assessment.EvaluationDetail{Kind: assessment.EvaluationModelKindScale},
	)
	outcome.Primary = &assessment.OutcomeScoreValue{Kind: assessment.OutcomeScoreKindRawTotal, Value: 18.5}
	outcome.Level = &assessment.OutcomeResultLevel{Code: "high", Label: "高风险", Severity: "high"}
	if err := a.ApplyScoringOutcome(outcome); err != nil {
		t.Fatal(err)
	}

	po := NewAssessmentMapper().ToPO(a)
	if po.Status != "evaluated" || po.PrimaryScoreKind == nil || *po.PrimaryScoreKind != string(assessment.OutcomeScoreKindRawTotal) {
		t.Fatalf("evaluated projection status=%s primary_kind=%v", po.Status, po.PrimaryScoreKind)
	}
	if po.PrimaryScoreValue == nil || *po.PrimaryScoreValue != 18.5 || po.LevelCode == nil || *po.LevelCode != "high" || po.Severity == nil || *po.Severity != "high" {
		t.Fatalf("evaluated v2 fields: score=%v level=%v severity=%v", po.PrimaryScoreValue, po.LevelCode, po.Severity)
	}
}

func TestApplyAssessmentOutcomeV2FieldsKeepsTypologyLevelWhenRiskIsNone(t *testing.T) {
	modelRef := assessment.NewEvaluationModelRefWithIdentity(
		assessment.EvaluationModelKindPersonality,
		modelcatalog.SubKindTypology,
		modelcatalog.AlgorithmMBTI,
		meta.ID(0),
		meta.NewCode("MBTI-16P"),
		"1.0.0",
		"MBTI",
	)
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(2001),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-MBTI"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(5001)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(102)),
		assessment.WithEvaluationModel(modelRef),
	)
	if err != nil {
		t.Fatalf("NewAssessment returned error: %v", err)
	}
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}
	score := 92.0
	outcome := assessment.NewAssessmentOutcome(
		modelRef,
		assessment.ResultSummary{PrimaryLabel: "INTJ", Score: &score},
		assessment.EvaluationDetail{Kind: assessment.EvaluationModelKindPersonality},
	)
	outcome.Primary = &assessment.OutcomeScoreValue{
		Kind:  assessment.OutcomeScoreKindMatchPercent,
		Value: score,
		Label: "INTJ",
	}
	outcome.Level = &assessment.OutcomeResultLevel{
		Code:     "INTJ",
		Label:    "INTJ",
		Severity: "none",
	}
	if err := a.ApplyScoringOutcome(outcome); err != nil {
		t.Fatalf("ApplyScoringOutcome returned error: %v", err)
	}

	po := NewAssessmentMapper().ToPO(a)
	if po.PrimaryScoreKind == nil || *po.PrimaryScoreKind != string(assessment.OutcomeScoreKindMatchPercent) {
		t.Fatalf("primary score kind = %v, want %s", po.PrimaryScoreKind, assessment.OutcomeScoreKindMatchPercent)
	}
	if po.LevelCode == nil || *po.LevelCode != "INTJ" {
		t.Fatalf("level code = %v, want INTJ", po.LevelCode)
	}
	if po.Severity == nil || *po.Severity != "none" {
		t.Fatalf("severity = %v, want none", po.Severity)
	}
	if po.EvaluationModelAlgorithm == nil || *po.EvaluationModelAlgorithm != "mbti" {
		t.Fatalf("algorithm = %v, want mbti", po.EvaluationModelAlgorithm)
	}
}

func newSubmittedScaleAssessment(t *testing.T) *assessment.Assessment {
	t.Helper()
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(1003),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v3"),
		assessment.NewAnswerSheetRef(meta.FromUint64(2003)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(5002)),
		assessment.WithEvaluationModel(assessment.NewScaleEvaluationModelRef(meta.ID(0), meta.NewCode("SDS"), "", "抑郁自评")),
	)
	if err != nil {
		t.Fatalf("NewAssessment returned error: %v", err)
	}
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}
	return a
}
