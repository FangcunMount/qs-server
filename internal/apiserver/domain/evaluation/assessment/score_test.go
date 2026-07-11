package assessment

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestApplyScoringProjectionDoesNotTreatTypeCodeAsRiskLevel(t *testing.T) {
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
	if err := a.ApplyScoringProjection(ScoringProjection{
		ModelRef: modelRef, Summary: ResultSummary{PrimaryLabel: "INTJ", Score: &score}, Score: &score, Level: "INTJ",
	}); err != nil {
		t.Fatalf("ApplyScoringProjection: %v", err)
	}
	if a.RiskLevel() != nil {
		t.Fatalf("risk level = %v, want nil for typology type code", *a.RiskLevel())
	}
}
