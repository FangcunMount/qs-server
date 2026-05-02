package assessment

import (
	"math"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestToReportResultIncludesCreatedAt(t *testing.T) {
	createdAt := time.Date(2026, time.April, 19, 18, 8, 30, 0, time.Local)
	rpt := domainReport.ReconstructInterpretReport(
		domainReport.NewID(615830360323797550),
		"SNAP-IV量表（18项）",
		"3adyDE",
		31,
		domainReport.RiskLevelMedium,
		"总体症状负担中度偏高，控制不理想。",
		nil,
		nil,
		createdAt,
		nil,
	)

	got := toReportResult(rpt)
	if got == nil {
		t.Fatal("expected report result")
	}
	if !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected createdAt %v, got %v", createdAt, got.CreatedAt)
	}
}

func TestToAssessmentResultRejectsNegativeOrgID(t *testing.T) {
	a := domainAssessment.Reconstruct(
		meta.FromUint64(1001),
		-1,
		testee.NewID(2001),
		domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v1"),
		domainAssessment.NewAnswerSheetRef(meta.FromUint64(3001)),
		nil,
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
