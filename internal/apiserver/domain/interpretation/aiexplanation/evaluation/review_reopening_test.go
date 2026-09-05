package evaluation

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestReviewReopeningPreservesRejectedRoundAndReusesEvidence(t *testing.T) {
	e, a, b := contradictionEvidence(t)
	ar, br := a, b
	ar.SemanticReview = nil
	br.SemanticReview = nil
	require.NoError(t, e.AddHumanReviews([]CandidateHumanReview{ar, br}))
	at := a.ReviewedAt.Add(time.Hour)
	require.NoError(t, e.Finalize("user:admin", "human_review_finalized", at))
	require.Equal(t, EvidenceStatusRejected, e.Status)
	original := e.Clone()
	require.NoError(t, e.ReopenReview("user:admin", "judge contradiction requires explicit signatures", at.Add(time.Minute)))
	require.Len(t, e.HumanReviews, 68)
	require.Len(t, e.ReviewReopenings, 1)
	require.Equal(t, original.HumanReviews, e.ReviewReopenings[0].Reviews)
	require.Equal(t, *original.GateResult, e.ReviewReopenings[0].Gate)
	require.Equal(t, original.Slots, e.Slots)
	require.Equal(t, original.GenerationExecutions, e.GenerationExecutions)
	require.Equal(t, original.SemanticExecutions, e.SemanticExecutions)
	require.Equal(t, original.Release, e.Release)
	require.Error(t, e.AddHumanReview(a)) // old signature cannot be replayed
	a.ReviewedAt = at.Add(2 * time.Minute)
	b.ReviewedAt = a.ReviewedAt
	require.NoError(t, e.AddHumanReviews([]CandidateHumanReview{a, b}))
	require.NoError(t, e.Finalize("user:admin", "human_review_finalized", at.Add(3*time.Minute)))
	require.Equal(t, EvidenceStatusApproved, e.Status)
	require.NoError(t, e.Validate())
	require.Error(t, e.ReopenReview("user:admin", "cannot reopen approval", at.Add(4*time.Minute)))
	changed := e.Clone()
	changed.ReviewReopenings[0].Gate.Passed = true
	require.Error(t, changed.Validate())
	require.False(t, e.ReviewReopenings[0].Gate.Passed)
	changed = e.Clone()
	changed.ReviewReopenings[0].Reviews[0].Reason = "tampered"
	require.Error(t, changed.Validate()) // retained unaffected signatures must match
	changed = e.Clone()
	changed.ReviewReopenings = nil
	require.Error(t, changed.Validate())
}

func TestReviewReopeningRejectsHumanRejectionAndDeterministicFailure(t *testing.T) {
	for _, human := range []bool{false, true} {
		e, a, b := contradictionEvidence(t)
		a.SemanticReview = nil
		b.SemanticReview = nil
		if human {
			a.Decision = ReviewDecisionReject
		} else {
			e.Slots[0].Candidate.Assertions = append(e.Slots[0].Candidate.Assertions, AssertionReceipt{Type: "schema_valid", Scope: AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "deterministic", Status: AssertionFailed})
		}
		require.NoError(t, e.AddHumanReviews([]CandidateHumanReview{a, b}))
		at := a.ReviewedAt.Add(time.Hour)
		require.NoError(t, e.Finalize("user:admin", "human_review_finalized", at))
		before := e.Clone()
		require.Error(t, e.ReopenReview("user:admin", "not eligible", at.Add(time.Minute)))
		require.Equal(t, before, e)
	}
}

func TestCheckedFinalizationCannotSilentlyRejectAnExpectedApproval(t *testing.T) {
	e, a, b := contradictionEvidence(t)
	a.SemanticReview = nil
	b.SemanticReview = nil
	require.NoError(t, e.AddHumanReviews([]CandidateHumanReview{a, b}))
	before := e.Clone()
	at := a.ReviewedAt.Add(time.Hour)
	require.ErrorIs(t, e.FinalizeChecked("user:admin", "expected approval", e.Version(), true, at), ErrConflict)
	require.Equal(t, before, e)
	require.NoError(t, e.FinalizeChecked("user:admin", "explicit rejection", e.Version(), false, at))
	require.Equal(t, EvidenceStatusRejected, e.Status)
	require.Equal(t, "explicit rejection", e.StateTransitions[len(e.StateTransitions)-1].Reason)
}
