package evaluation

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestCancelEvidenceV2RetainsEvidenceAndStopsQueuedWork(t *testing.T) {
	e := validCollectingEvidenceV2(t)
	before := e.Clone()
	at := e.Audit.CreatedAt.Add(24 * time.Hour)
	require.NoError(t, e.Cancel("user:42", " stop this release ", false, at))
	require.Equal(t, EvidenceStatusCanceled, e.Status)
	require.Equal(t, before.Version()+1, e.Version())
	require.Equal(t, before.GenerationExecutions, e.GenerationExecutions)
	require.Equal(t, before.SemanticExecutions, e.SemanticExecutions)
	require.Equal(t, before.Slots, e.Slots)
	require.Equal(t, "stop this release", e.StateTransitions[len(e.StateTransitions)-1].Reason)
	require.Equal(t, "user:42", e.StateTransitions[len(e.StateTransitions)-1].Actor)
	require.Equal(t, at, *e.Audit.CanceledAt)
	action, err := e.NextAction()
	require.NoError(t, err)
	require.Equal(t, EvidenceNextActionNone, action.Kind)
	frozen := e.Clone()
	require.ErrorIs(t, e.Cancel("user:42", "again", false, at), ErrConflict)
	require.Equal(t, frozen, e.Clone())
}

func TestCancelEvidenceV2OnlyAbandonsUndispatchedPreparation(t *testing.T) {
	for _, dispatch := range []bool{false, true} {
		t.Run(map[bool]string{false: "prepared", true: "dispatching"}[dispatch], func(t *testing.T) {
			e := newEmptyCollectingEvidenceV2(t)
			at := e.Audit.CreatedAt.Add(time.Minute)
			cp := runtimeCheckpoint(EvidenceExecutionGeneration, "cancel:checkpoint", "case-1", 1, "", 1, at)
			require.NoError(t, e.BeginNextExecution(cp))
			if dispatch {
				require.NoError(t, e.MarkExecutionDispatching(cp.Owner, at.Add(time.Second)))
			}
			before := e.Clone()
			err := e.Cancel("user:42", "stop", false, at.Add(time.Hour))
			if dispatch {
				require.ErrorIs(t, err, ErrConflict)
				require.Equal(t, before, e.Clone())
				return
			}
			require.NoError(t, err)
			require.Nil(t, e.Execution())
			require.Equal(t, []string{cp.ID}, e.StateTransitions[len(e.StateTransitions)-1].EvidenceRefs)
			require.Error(t, e.MarkExecutionDispatching(cp.Owner, at.Add(2*time.Hour)))
		})
	}
}

func TestDiscardEvidenceV2RequiresReviewStateAndPreservesCandidates(t *testing.T) {
	e := completeEvidenceV2ForReview(t)
	before := e.Clone()
	at := e.Audit.CreatedAt.Add(24 * time.Hour)
	require.ErrorIs(t, e.Cancel("user:42", "stop", false, at), ErrConflict)
	require.NoError(t, e.Cancel("user:42", "superseded", true, at))
	require.Equal(t, before.Slots, e.Slots)
	require.Equal(t, before.HumanReviews, e.HumanReviews)
	require.Equal(t, "operator_discarded", e.StateTransitions[len(e.StateTransitions)-1].CauseCode)
	require.Nil(t, e.GateResult)
}

func TestCancelEvidenceV2RejectsUnknownWithoutMutation(t *testing.T) {
	e := newEmptyCollectingEvidenceV2(t)
	e.UnresolvedResultUnknownCount = 1
	before := e.Clone()
	require.ErrorIs(t, e.Cancel("user:42", "stop", false, e.Audit.CreatedAt.Add(time.Hour)), ErrConflict)
	require.Equal(t, before, e.Clone())
}
