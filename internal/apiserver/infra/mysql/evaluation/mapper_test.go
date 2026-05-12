package evaluation

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestAssessmentMapperWritesAndReadsScaleEvaluationModelRef(t *testing.T) {
	scaleRef := assessment.NewMedicalScaleRef(meta.FromUint64(3001), meta.NewCode("SDS"), "抑郁自评")
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(2001),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-SDS"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(5001)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(101)),
		assessment.WithMedicalScale(scaleRef),
	)
	if err != nil {
		t.Fatalf("NewAssessment returned error: %v", err)
	}

	mapper := NewAssessmentMapper()
	po := mapper.ToPO(a)
	if po.EvaluationModelKind == nil || *po.EvaluationModelKind != "scale" {
		t.Fatalf("evaluation model kind = %v, want scale", po.EvaluationModelKind)
	}
	if po.EvaluationModelCode == nil || *po.EvaluationModelCode != "SDS" {
		t.Fatalf("evaluation model code = %v, want SDS", po.EvaluationModelCode)
	}
	if po.EvaluationModelTitle == nil || *po.EvaluationModelTitle != "抑郁自评" {
		t.Fatalf("evaluation model title = %v, want scale title", po.EvaluationModelTitle)
	}
	if po.MedicalScaleCode == nil || *po.MedicalScaleCode != "SDS" {
		t.Fatalf("legacy scale code = %v, want SDS", po.MedicalScaleCode)
	}

	roundTrip := mapper.ToDomain(po)
	if roundTrip.EvaluationModelRef() == nil {
		t.Fatal("round trip assessment should have evaluation model ref")
	}
	if roundTrip.EvaluationModelRef().Kind() != assessment.EvaluationModelKindScale ||
		roundTrip.EvaluationModelRef().Code().String() != "SDS" ||
		roundTrip.EvaluationModelRef().Title() != "抑郁自评" {
		t.Fatalf("unexpected round trip model ref: %#v", roundTrip.EvaluationModelRef())
	}
	if roundTrip.MedicalScaleRef() == nil || roundTrip.MedicalScaleRef().Code().String() != "SDS" {
		t.Fatalf("legacy scale ref should remain available, got %#v", roundTrip.MedicalScaleRef())
	}
}

func TestAssessmentMapperWritesModelOnlyAssessmentWithoutLegacyScaleFields(t *testing.T) {
	modelRef := assessment.NewEvaluationModelRefByCode(assessment.EvaluationModelKindMBTI, meta.NewCode("MBTI-16P"), "1.0.0", "MBTI")
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

	mapper := NewAssessmentMapper()
	po := mapper.ToPO(a)
	if po.EvaluationModelKind == nil || *po.EvaluationModelKind != "mbti" {
		t.Fatalf("evaluation model kind = %v, want mbti", po.EvaluationModelKind)
	}
	if po.EvaluationModelCode == nil || *po.EvaluationModelCode != "MBTI-16P" {
		t.Fatalf("evaluation model code = %v, want MBTI-16P", po.EvaluationModelCode)
	}
	if po.EvaluationModelVersion == nil || *po.EvaluationModelVersion != "1.0.0" {
		t.Fatalf("evaluation model version = %v, want 1.0.0", po.EvaluationModelVersion)
	}
	if po.MedicalScaleID != nil || po.MedicalScaleCode != nil || po.MedicalScaleName != nil {
		t.Fatalf("model-only assessment should not populate legacy scale fields: %#v", po)
	}

	roundTrip := mapper.ToDomain(po)
	if roundTrip.EvaluationModelRef() == nil {
		t.Fatal("round trip assessment should have evaluation model ref")
	}
	if roundTrip.EvaluationModelRef().Kind() != assessment.EvaluationModelKindMBTI ||
		roundTrip.EvaluationModelRef().Code().String() != "MBTI-16P" ||
		roundTrip.MedicalScaleRef() != nil {
		t.Fatalf("unexpected round trip assessment refs: model=%#v scale=%#v", roundTrip.EvaluationModelRef(), roundTrip.MedicalScaleRef())
	}
}
