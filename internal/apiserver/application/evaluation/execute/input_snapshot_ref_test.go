package execute

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestInputSnapshotRefFromResolvedInputUsesModelCodeAndVersion(t *testing.T) {
	t.Parallel()

	ref := inputSnapshotRefFromResolvedInput(nil, &evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{Code: "PHQ9", Version: "1.0.0"},
	})
	if ref != "model:PHQ9@1.0.0" {
		t.Fatalf("ref = %q, want model:PHQ9@1.0.0", ref)
	}
}

func TestInputSnapshotRefFromResolvedInputFallsBackToAnswerSheet(t *testing.T) {
	t.Parallel()

	a, err := domainAssessment.NewAssessment(
		1,
		testee.NewID(1001),
		domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("QNR-1"), "1.0.0"),
		domainAssessment.NewAnswerSheetRef(meta.FromUint64(2001)),
		domainAssessment.NewAdhocOrigin(),
	)
	if err != nil {
		t.Fatal(err)
	}
	ref := inputSnapshotRefFromResolvedInput(a, &evaluationinput.InputSnapshot{})
	if ref != "answersheet:2001" {
		t.Fatalf("ref = %q, want answersheet:2001", ref)
	}
}
