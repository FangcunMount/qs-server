package assessment

import (
	"math"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestToAssessmentResultMapsEvaluatedAt(t *testing.T) {
	evaluatedAt := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	a := domainAssessment.Reconstruct(
		meta.FromUint64(1001),
		1,
		testee.NewID(2001),
		domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v1"),
		domainAssessment.NewAnswerSheetRef(meta.FromUint64(3001)),
		domainAssessment.NewAdhocOrigin(),
		domainAssessment.StatusEvaluated,
		nil,
		nil,
		nil,
		&evaluatedAt,
		nil,
		nil,
	)

	got, err := toAssessmentResult(a)
	if err != nil {
		t.Fatalf("toAssessmentResult: %v", err)
	}
	if got.EvaluatedAt == nil || !got.EvaluatedAt.Equal(evaluatedAt) {
		t.Fatalf("EvaluatedAt = %#v, want %v", got.EvaluatedAt, evaluatedAt)
	}
	if got.InterpretedAt != nil {
		t.Fatalf("InterpretedAt = %#v, want nil", got.InterpretedAt)
	}
}

func TestAssessmentRowToResultMapsEvaluatedAt(t *testing.T) {
	evaluatedAt := time.Date(2026, 7, 11, 13, 0, 0, 0, time.UTC)
	got, err := assessmentRowToResult(evaluationreadmodel.AssessmentRow{
		ID:                   1001,
		OrgID:                1,
		TesteeID:             2001,
		QuestionnaireCode:    "q-code",
		QuestionnaireVersion: "v1",
		AnswerSheetID:        3001,
		OriginType:           "adhoc",
		Status:               domainAssessment.StatusEvaluated.String(),
		EvaluatedAt:          &evaluatedAt,
	})
	if err != nil {
		t.Fatalf("assessmentRowToResult: %v", err)
	}
	if got.EvaluatedAt == nil || !got.EvaluatedAt.Equal(evaluatedAt) {
		t.Fatalf("EvaluatedAt = %#v, want %v", got.EvaluatedAt, evaluatedAt)
	}
	if got.InterpretedAt != nil {
		t.Fatalf("InterpretedAt = %#v, want nil", got.InterpretedAt)
	}
}

func TestToAssessmentResultRejectsNegativeOrgID(t *testing.T) {
	a := domainAssessment.Reconstruct(
		meta.FromUint64(1001),
		-1,
		testee.NewID(2001),
		domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v1"),
		domainAssessment.NewAnswerSheetRef(meta.FromUint64(3001)),
		domainAssessment.NewAdhocOrigin(),
		domainAssessment.StatusPending,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	if _, err := toAssessmentResult(a); err == nil {
		t.Fatal("expected negative org id to be rejected")
	}
}

func TestBuildCreateRequestRejectsOverflowOrgID(t *testing.T) {
	_, err := assessmentCreateRequestAssembler{}.Assemble(CreateAssessmentDTO{
		OrgID:                uint64(math.MaxInt64) + 1,
		TesteeID:             2001,
		QuestionnaireCode:    "q-code",
		QuestionnaireVersion: "v1",
		AnswerSheetID:        3001,
	})
	if err == nil {
		t.Fatal("expected overflow org id to be rejected")
	}
}

func TestBuildCreateRequestMapsGenericEvaluationModelRef(t *testing.T) {
	kind := domainAssessment.EvaluationModelKindPersonality.String()
	subKind := "typology"
	algorithm := "sbti"
	code := "SBTI_FUN"
	version := "1.0.0"
	title := "SBTI 趣味人格测评"
	req, err := assessmentCreateRequestAssembler{}.Assemble(CreateAssessmentDTO{
		OrgID:                1,
		TesteeID:             2001,
		QuestionnaireCode:    "SBTI_FUN",
		QuestionnaireVersion: "1.0.0",
		AnswerSheetID:        3001,
		ModelKind:            &kind,
		ModelSubKind:         &subKind,
		ModelAlgorithm:       &algorithm,
		ModelCode:            &code,
		ModelVersion:         &version,
		ModelTitle:           &title,
	})
	if err != nil {
		t.Fatalf("Assemble returned error: %v", err)
	}
	if req.ModelRef == nil {
		t.Fatal("RuleSetRef is nil")
	}
	if req.ModelRef.Kind() != domainAssessment.EvaluationModelKindPersonality ||
		req.ModelRef.SubKind() != modelcatalog.SubKindTypology ||
		req.ModelRef.Algorithm() != modelcatalog.AlgorithmSBTI ||
		req.ModelRef.Code().String() != "SBTI_FUN" ||
		req.ModelRef.Version() != "1.0.0" ||
		req.ModelRef.Title() != title {
		t.Fatalf("RuleSetRef = %#v, want SBTI_FUN typology identity", req.ModelRef)
	}
}

func TestBuildCreateRequestMapsExplicitTypologyIdentity(t *testing.T) {
	kind := domainAssessment.EvaluationModelKindPersonality.String()
	subKind := string(modelcatalog.SubKindTypology)
	algorithm := string(modelcatalog.AlgorithmMBTI)
	code := "MBTI_OEJTS"
	req, err := assessmentCreateRequestAssembler{}.Assemble(CreateAssessmentDTO{
		OrgID:                1,
		TesteeID:             2001,
		QuestionnaireCode:    "MBTI_OEJTS",
		QuestionnaireVersion: "1.0.0",
		AnswerSheetID:        3001,
		ModelKind:            &kind,
		ModelSubKind:         &subKind,
		ModelAlgorithm:       &algorithm,
		ModelCode:            &code,
	})
	if err != nil {
		t.Fatalf("Assemble returned error: %v", err)
	}
	if req.ModelRef == nil {
		t.Fatal("ModelRef is nil")
	}
	if req.ModelRef.Kind() != domainAssessment.EvaluationModelKindPersonality ||
		req.ModelRef.SubKind() != modelcatalog.SubKindTypology ||
		req.ModelRef.Algorithm() != modelcatalog.AlgorithmMBTI {
		t.Fatalf("ModelRef = %#v, want personality/typology/mbti", req.ModelRef)
	}
}
