package assessment

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestApplyScoringProjectionDoesNotTreatTypeCodeAsRiskLevel(t *testing.T) {
	modelRef := NewEvaluationModelRefWithIdentity(
		EvaluationModelKindTypology,
		modelcatalog.SubKindTypology,
		modelcatalog.AlgorithmPersonalityTypology,
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
	if err := a.ApplyScoringProjectionAt(ScoringProjection{
		ModelRef: modelRef, Summary: ResultSummary{PrimaryLabel: "INTJ", Score: &score}, Score: &score, Level: "INTJ",
	}, time.Unix(100, 0)); err != nil {
		t.Fatalf("ApplyScoringProjectionAt: %v", err)
	}
	if a.RiskLevel() != nil {
		t.Fatalf("risk level = %v, want nil for typology type code", *a.RiskLevel())
	}
}

func TestApplyScoringProjectionKeepsCanonicalNonRiskLevelInSummary(t *testing.T) {
	modelRef := NewEvaluationModelRefWithIdentity(
		modelcatalog.KindBehavioralRating,
		modelcatalog.SubKindEmpty,
		modelcatalog.AlgorithmBrief2,
		meta.ID(0),
		meta.NewCode("gXkk9W"),
		"v22",
		"BRIEF-2",
	)
	a, err := NewAssessment(
		1,
		testee.NewID(2),
		NewQuestionnaireRefByCode(meta.NewCode("gXkk9W"), "7.0.1"),
		NewAnswerSheetRef(meta.FromUint64(2)),
		NewAdhocOrigin(),
		WithEvaluationModel(modelRef),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	legacyLevel := "none"
	if err := a.ApplyScoringProjectionAt(ScoringProjection{
		ModelRef: modelRef,
		Summary:  ResultSummary{PrimaryLabel: "中度执行功能障碍", Level: &legacyLevel},
		Level:    "moderate",
	}, time.Unix(100, 0)); err != nil {
		t.Fatalf("ApplyScoringProjectionAt: %v", err)
	}

	if a.RiskLevel() != nil {
		t.Fatalf("risk level = %v, want nil for BRIEF-2 level", *a.RiskLevel())
	}
	summary := a.ResultSummary()
	if summary == nil || summary.Level == nil || *summary.Level != "moderate" {
		t.Fatalf("summary level = %#v, want moderate", summary)
	}
}
