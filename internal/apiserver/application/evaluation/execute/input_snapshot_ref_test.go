package execute

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestInputSnapshotRefFromResolvedInputUsesVerifiableIdentity(t *testing.T) {
	t.Parallel()

	snapshot := func() *evaluationinput.InputSnapshot {
		return &evaluationinput.InputSnapshot{
			Model: &evaluationinput.ModelSnapshot{Code: "PHQ9", Version: "1.0.0"},
			AnswerSheet: &evaluationinput.AnswerSheetSnapshot{
				ID: 2001, Answers: []evaluationinput.AnswerSnapshot{{QuestionCode: "q1", Score: 1}},
			},
		}
	}
	ref := inputSnapshotRefFromResolvedInput(nil, snapshot())
	if !evaluationinput.IsIdentityRef(ref) {
		t.Fatalf("ref = %q, want isn:v1 identity ref", ref)
	}
	if again := inputSnapshotRefFromResolvedInput(nil, snapshot()); again != ref {
		t.Fatalf("ref not deterministic: %q vs %q", ref, again)
	}

	changed := snapshot()
	changed.AnswerSheet.Answers[0].Score = 2
	if drifted := inputSnapshotRefFromResolvedInput(nil, changed); drifted == ref {
		t.Fatal("answer change must produce a different ref")
	}
}

func TestValidateInputSnapshotRefAcrossAttempts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		previous, current string
		wantErr           bool
	}{
		{"same identity ref", "isn:v1:aaa", "isn:v1:aaa", false},
		{"drifted identity ref", "isn:v1:aaa", "isn:v1:bbb", true},
		{"legacy previous is not comparable", "model:PHQ9@1.0.0", "isn:v1:bbb", false},
		{"empty previous (first attempt)", "", "isn:v1:bbb", false},
	}
	for _, test := range cases {
		err := validateInputSnapshotRefAcrossAttempts(test.previous, test.current)
		if (err != nil) != test.wantErr {
			t.Fatalf("%s: err = %v, wantErr = %v", test.name, err, test.wantErr)
		}
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
