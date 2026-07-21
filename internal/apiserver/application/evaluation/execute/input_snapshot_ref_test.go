package execute

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
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
		t.Fatalf("ref = %q, want versioned identity ref", ref)
	}
	if !evaluationinput.IsV2IdentityRef(ref) {
		t.Fatalf("ref = %q, want isn:v2 identity ref", ref)
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

func TestInputSnapshotRefForAttemptKeepsExistingV1Chain(t *testing.T) {
	t.Parallel()
	snapshot := &evaluationinput.InputSnapshot{
		Model:       &evaluationinput.ModelSnapshot{Code: "PHQ9", Version: "1.0.0"},
		AnswerSheet: &evaluationinput.AnswerSheetSnapshot{ID: 2001},
	}
	previousIdentity, _ := evaluationinput.NewLegacyV1InputSnapshotIdentity(snapshot)
	ordinary := inputSnapshotRefForAttempt(nil, snapshot, previousIdentity.Ref(), retrygovernance.AttemptOriginAutomatic)
	if !evaluationinput.IsV1IdentityRef(ordinary) || ordinary != previousIdentity.Ref() {
		t.Fatalf("ordinary ref = %q, want unchanged v1", ordinary)
	}
	forced := inputSnapshotRefForAttempt(nil, snapshot, previousIdentity.Ref(), retrygovernance.AttemptOriginForce)
	if !evaluationinput.IsV2IdentityRef(forced) {
		t.Fatalf("force ref = %q, want v2", forced)
	}
}

func TestValidateInputSnapshotRefAcrossAttempts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		previous, current string
		origin            retrygovernance.AttemptOrigin
		wantErr           bool
	}{
		{"same identity ref", "isn:v1:aaa", "isn:v1:aaa", retrygovernance.AttemptOriginAutomatic, false},
		{"drifted identity ref", "isn:v1:aaa", "isn:v1:bbb", retrygovernance.AttemptOriginAutomatic, true},
		{"manual drift is rejected", "isn:v2:aaa", "isn:v2:bbb", retrygovernance.AttemptOriginManual, true},
		{"force drift is accepted", "isn:v2:aaa", "isn:v2:bbb", retrygovernance.AttemptOriginForce, false},
		{"force v1 to v2 revision is accepted", "isn:v1:aaa", "isn:v2:bbb", retrygovernance.AttemptOriginForce, false},
		{"same v2 identity ref", "isn:v2:aaa", "isn:v2:aaa", retrygovernance.AttemptOriginAutomatic, false},
		{"different identity versions", "isn:v1:aaa", "isn:v2:aaa", retrygovernance.AttemptOriginAutomatic, true},
		{"legacy previous is not comparable", "model:PHQ9@1.0.0", "isn:v1:bbb", retrygovernance.AttemptOriginAutomatic, false},
		{"empty previous (first attempt)", "", "isn:v1:bbb", retrygovernance.AttemptOriginInitial, false},
	}
	for _, test := range cases {
		err := validateInputSnapshotRefAcrossAttempts(test.previous, test.current, test.origin)
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
