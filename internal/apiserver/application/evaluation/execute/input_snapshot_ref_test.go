package execute

import (
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

func TestInputSnapshotRefFromResolvedInputUsesVerifiableIdentity(t *testing.T) {
	t.Parallel()

	snapshot := func() *evaluationinput.InputSnapshot {
		return &evaluationinput.InputSnapshot{
			Model: &evaluationinput.ModelSnapshot{
				Kind: evaluationinput.EvaluationModelKindScale, Algorithm: string(modelcatalog.AlgorithmScaleDefault),
				AlgorithmFamily: string(modelcatalog.AlgorithmFamilyFactorScoring), DecisionKind: string(modelcatalog.DecisionKindScoreRange),
				Code: "PHQ9", Version: "1.0.0",
			},
			DefinitionV2: &modeldefinition.Definition{},
			Questionnaire: &evaluationinput.QuestionnaireSnapshot{
				Code: "PHQ9-Q", Version: "1.0.0", Questions: []evaluationinput.QuestionSnapshot{{Code: "q1"}},
			},
			AnswerSheet: &evaluationinput.AnswerSheetSnapshot{
				ID: 2001, QuestionnaireCode: "PHQ9-Q", QuestionnaireVersion: "1.0.0",
				Answers: []evaluationinput.AnswerSnapshot{{QuestionCode: "q1", Score: 1}},
			},
		}
	}
	ref, err := inputSnapshotRefFromResolvedInput(snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if !evaluationinput.IsIdentityRef(ref) {
		t.Fatalf("ref = %q, want versioned identity ref", ref)
	}
	again, err := inputSnapshotRefFromResolvedInput(snapshot())
	if err != nil || again != ref {
		t.Fatalf("ref not deterministic: %q vs %q", ref, again)
	}

	changed := snapshot()
	changed.AnswerSheet.Answers[0].Score = 2
	drifted, err := inputSnapshotRefFromResolvedInput(changed)
	if err != nil {
		t.Fatal(err)
	}
	if drifted == ref {
		t.Fatal("answer change must produce a different ref")
	}
}

func TestInputSnapshotRefFromResolvedInputRejectsIncompleteMaterial(t *testing.T) {
	t.Parallel()
	if ref, err := inputSnapshotRefFromResolvedInput(&evaluationinput.InputSnapshot{}); err == nil || ref != "" {
		t.Fatalf("ref = %q, err = %v, want identity-required error", ref, err)
	}
}

func TestValidateInputSnapshotRefAcrossAttempts(t *testing.T) {
	t.Parallel()

	refA := "isn:v2:" + strings.Repeat("a", 64)
	refB := "isn:v2:" + strings.Repeat("b", 64)
	cases := []struct {
		name              string
		previous, current string
		origin            retrygovernance.AttemptOrigin
		wantErr           bool
	}{
		{"same identity ref", refA, refA, retrygovernance.AttemptOriginAutomatic, false},
		{"drifted identity ref", refA, refB, retrygovernance.AttemptOriginAutomatic, true},
		{"manual drift is rejected", refA, refB, retrygovernance.AttemptOriginManual, true},
		{"force v2 drift is accepted", refA, refB, retrygovernance.AttemptOriginForce, false},
		{"force v1 ref is rejected", "isn:v1:" + strings.Repeat("a", 64), refB, retrygovernance.AttemptOriginForce, true},
		{"readable previous is rejected", "model:PHQ9@1.0.0", refB, retrygovernance.AttemptOriginAutomatic, true},
		{"empty previous first attempt", "", refB, retrygovernance.AttemptOriginInitial, false},
		{"malformed current is rejected", "", "isn:v2:abc", retrygovernance.AttemptOriginInitial, true},
	}
	for _, test := range cases {
		err := validateInputSnapshotRefAcrossAttempts(test.previous, test.current, test.origin)
		if (err != nil) != test.wantErr {
			t.Fatalf("%s: err = %v, wantErr = %v", test.name, err, test.wantErr)
		}
	}
}
