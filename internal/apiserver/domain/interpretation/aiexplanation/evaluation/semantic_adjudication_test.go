package evaluation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func contradictionEvidence(t *testing.T) (PromptEvaluationEvidenceV2, CandidateHumanReview, CandidateHumanReview) {
	t.Helper()
	e := completeEvidenceV2ForReview(t)
	freezeGateVersion(t, &e, "v2")
	// Remove the shared fixture's unrelated transport retries; this case isolates
	// one contradictory quality judgment in an otherwise healthy 35+35 run.
	e.GenerationExecutions = e.GenerationExecutions[1:]
	e.GenerationExecutions[0].ExecutionOrdinal = 1
	e.Slots[0].GenerationExecutionIDs = []string{e.GenerationExecutions[0].ID}
	e.SemanticExecutions = e.SemanticExecutions[1:]
	e.SemanticExecutions[0].ExecutionOrdinal = 1
	e.Slots[0].Candidate.SemanticExecutionIDs = []string{e.SemanticExecutions[0].ID}
	first, second := e.HumanReviews[0], e.HumanReviews[1]
	e.HumanReviews = e.HumanReviews[2:]
	c := e.Slots[0].Candidate
	var j *SemanticEvaluationExecution
	for i := range e.SemanticExecutions {
		if e.SemanticExecutions[i].ID == c.AcceptedSemanticExecutionID {
			j = &e.SemanticExecutions[i]
		}
	}
	require.NotNil(t, j)
	detail := "未发现诊断、因果或其他禁止声明。"
	j.Result.Decisions = append(j.Result.Decisions, SemanticDecision{Type: "forbidden_claims_absent", Scope: AssertionScopeDefault, Ordinal: 1, Status: AssertionFailed, Detail: detail})
	c.Assertions = append(c.Assertions, AssertionReceipt{Type: "forbidden_claims_absent", Scope: AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "semantic-" + j.Result.EvaluatorVersion, Status: AssertionFailed, Detail: detail})
	r := SemanticContradictionReview{PolicyVersion: SemanticAdjudicationPolicyV1, ExecutionID: j.ID, OutputFingerprint: j.Result.OutputFingerprint, AssertionOrdinal: 1, OriginalDetail: detail, CandidateExcerpt: "schema_version", Reason: "已逐条核对候选，原始裁判理由确认未发现禁止声明，却将该项标成失败。"}
	first.SemanticReview = &r
	copyReview := r
	second.SemanticReview = &copyReview
	require.NoError(t, e.Validate())
	return e, first, second
}

func TestSemanticContradictionRequiresTwoDistinctRoleSignaturesAndKeepsEvidence(t *testing.T) {
	e, first, second := contradictionEvidence(t)
	original := e.Clone()
	at := first.ReviewedAt.Add(time.Minute)
	gate, err := e.EvaluateGate(at)
	require.NoError(t, err)
	require.False(t, gate.GatePasses["G4"])
	require.NoError(t, e.AddHumanReview(first))
	gate, err = e.EvaluateGate(at)
	require.NoError(t, err)
	require.False(t, gate.GatePasses["G4"])
	require.Empty(t, gate.SemanticAdjudications)
	require.NoError(t, e.AddHumanReview(second))
	gate, err = e.EvaluateGate(at)
	require.NoError(t, err)
	require.True(t, gate.Passed, "%+v", gate.Reasons)
	require.Len(t, gate.SemanticAdjudications, 1)
	require.Equal(t, []string{first.Reviewer, second.Reviewer}, gate.SemanticAdjudications[0].Reviewers)
	require.Equal(t, original.SemanticExecutions, e.SemanticExecutions)
	require.Equal(t, original.Slots, e.Slots)
	require.Equal(t, original.Release, e.Release)
	require.NoError(t, e.Finalize("user:admin", "dual_role_semantic_adjudication", at))
	require.NoError(t, e.Validate())
	require.Error(t, e.AddHumanReview(first))
	// Caller-owned pointers and cloned evidence cannot mutate stored signatures.
	first.SemanticReview.Reason = "changed"
	require.NotEqual(t, first.SemanticReview.Reason, e.HumanReviews[68].SemanticReview.Reason)
	cloned := e.Clone()
	cloned.GateResult.SemanticAdjudications[0].Reviewers[0] = "other"
	require.Equal(t, first.Reviewer, e.GateResult.SemanticAdjudications[0].Reviewers[0])
}

func TestSemanticAdjudicationRejectsUnboundOrIncompleteReviewsAtomically(t *testing.T) {
	tests := map[string]func(*CandidateHumanReview){
		"policy":      func(r *CandidateHumanReview) { r.SemanticReview.PolicyVersion = "unknown" },
		"execution":   func(r *CandidateHumanReview) { r.SemanticReview.ExecutionID = "other" },
		"fingerprint": func(r *CandidateHumanReview) { r.SemanticReview.OutputFingerprint = "sha256:wrong" },
		"assertion":   func(r *CandidateHumanReview) { r.SemanticReview.AssertionOrdinal = 9 },
		"detail":      func(r *CandidateHumanReview) { r.SemanticReview.OriginalDetail = "invented" },
		"excerpt":     func(r *CandidateHumanReview) { r.SemanticReview.CandidateExcerpt = "not in candidate" },
		"reason":      func(r *CandidateHumanReview) { r.SemanticReview.Reason = "" },
		"rejection":   func(r *CandidateHumanReview) { r.Decision = ReviewDecisionReject },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			e, first, second := contradictionEvidence(t)
			before := e.Clone()
			mutate(&second)
			require.Error(t, e.AddHumanReviews([]CandidateHumanReview{first, second}))
			require.Equal(t, before, e)
		})
	}
	e, first, second := contradictionEvidence(t)
	second.Reviewer = first.Reviewer
	require.Error(t, e.AddHumanReviews([]CandidateHumanReview{first, second}))
}

func TestSemanticAdjudicationCannotOverrideDeterministicOrOtherFailures(t *testing.T) {
	e, first, second := contradictionEvidence(t)
	require.NoError(t, e.AddHumanReviews([]CandidateHumanReview{first, second}))
	e.Slots[0].Candidate.Assertions[0].Status = AssertionFailed
	gate, err := e.EvaluateGate(first.ReviewedAt.Add(time.Minute))
	require.NoError(t, err)
	require.False(t, gate.GatePasses["G4"])
	e, first, _ = contradictionEvidence(t)
	e.Slots[0].Candidate.Assertions[len(e.Slots[0].Candidate.Assertions)-1].Evaluator = "deterministic"
	require.Error(t, e.AddHumanReview(first))
}
