package evaluation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestPromptEvaluationRunApprovesOnlyCompleteReviewedEvidence(t *testing.T) {
	startedAt := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	run, err := New(meta.ID(9001), validRelease(), startedAt)
	require.NoError(t, err)

	addCompleteAttempts(t, run, startedAt)
	require.NoError(t, run.CloseCollection(startedAt.Add(2*time.Hour)))
	addApprovingReviews(t, run, startedAt.Add(3*time.Hour))
	require.NoError(t, run.Finalize("release-owner", "all v1 gates reviewed", startedAt.Add(4*time.Hour)))

	require.Equal(t, StatusApproved, run.Status())
	require.True(t, run.IsPublishEvidence())
	gate := run.Gate()
	require.True(t, gate.Passed)
	require.Empty(t, gate.Reasons)
	require.Equal(t, RequiredGenerationAttempts, gate.Metrics.GenerationAttempts)
	require.Equal(t, RequiredGenerationAttempts, gate.Metrics.CaseAssertionPasses)
	require.Equal(t, RequiredGenerationAttempts*2, gate.Metrics.HumanReviews)
	require.Equal(t, 5.0, gate.Metrics.FaithfulnessAverage)

	require.Error(t, run.AddHumanReview(HumanReview{}))
	require.Error(t, run.AddAttempt(AttemptRecord{}))
}

func TestPromptEvaluationRunRejectsHumanDisagreement(t *testing.T) {
	startedAt := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	run, err := New(meta.ID(9002), validRelease(), startedAt)
	require.NoError(t, err)
	addCompleteAttempts(t, run, startedAt)
	require.NoError(t, run.CloseCollection(startedAt.Add(2*time.Hour)))
	addApprovingReviews(t, run, startedAt.Add(3*time.Hour))

	// A role cannot rewrite its decision in the same immutable run.
	require.Error(t, run.AddHumanReview(HumanReview{
		CaseID: "generation-1", Attempt: 1, Role: ReviewRoleSafetyProduct,
		Reviewer: "safety-2", Decision: ReviewDecisionReject,
		ReviewedAt: startedAt.Add(3 * time.Hour), Reason: "unsafe claim",
	}))

	// Restore a clean run and record an explicit cross-role disagreement.
	run, err = New(meta.ID(9003), validRelease(), startedAt)
	require.NoError(t, err)
	addCompleteAttempts(t, run, startedAt)
	require.NoError(t, run.CloseCollection(startedAt.Add(2*time.Hour)))
	for _, attempt := range run.sortedGenerationAttempts() {
		semanticsDecision := ReviewDecisionApprove
		if attempt.CaseID == "generation-1" && attempt.Attempt == 1 {
			semanticsDecision = ReviewDecisionReject
		}
		require.NoError(t, run.AddHumanReview(HumanReview{
			CaseID: attempt.CaseID, Attempt: attempt.Attempt, Role: ReviewRoleAssessmentSemantics,
			Reviewer: "semantic-reviewer", Decision: semanticsDecision,
			ReviewedAt: startedAt.Add(3 * time.Hour), Reason: "recorded assessment-semantics decision",
		}))
		require.NoError(t, run.AddHumanReview(HumanReview{
			CaseID: attempt.CaseID, Attempt: attempt.Attempt, Role: ReviewRoleSafetyProduct,
			Reviewer: "safety-reviewer", Decision: ReviewDecisionApprove,
			ReviewedAt: startedAt.Add(3 * time.Hour), Reason: "recorded safety-product decision",
		}))
	}
	require.NoError(t, run.Finalize("release-owner", "disagreement rejects this immutable run", startedAt.Add(4*time.Hour)))
	require.Equal(t, StatusRejected, run.Status())
	require.False(t, run.IsPublishEvidence())
	require.Contains(t, reasonCodes(run.Gate()), "human_review_rejected")
}

func TestPromptEvaluationRunRequiresDistinctReviewersAcrossRoles(t *testing.T) {
	startedAt := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	run, err := New(meta.ID(9011), validRelease(), startedAt)
	require.NoError(t, err)
	addCompleteAttempts(t, run, startedAt)
	require.NoError(t, run.CloseCollection(startedAt.Add(2*time.Hour)))

	require.NoError(t, run.AddHumanReview(HumanReview{
		CaseID: "generation-1", Attempt: 1, Role: ReviewRoleAssessmentSemantics,
		Reviewer: "reviewer-42", Decision: ReviewDecisionApprove,
		ReviewedAt: startedAt.Add(3 * time.Hour), Reason: "assessment semantics reviewed",
	}))
	err = run.AddHumanReview(HumanReview{
		CaseID: "generation-1", Attempt: 1, Role: ReviewRoleSafetyProduct,
		Reviewer: " reviewer-42 ", Decision: ReviewDecisionApprove,
		ReviewedAt: startedAt.Add(3 * time.Hour), Reason: "safety reviewed",
	})
	require.ErrorContains(t, err, "distinct reviewers")

	reviews := run.Reviews()
	reviews = append(reviews, HumanReview{
		CaseID: "generation-1", Attempt: 1, Role: ReviewRoleSafetyProduct,
		Reviewer: "reviewer-42", Decision: ReviewDecisionApprove,
		ReviewedAt: startedAt.Add(3 * time.Hour), Reason: "tampered persisted review",
	})
	_, err = Restore(PersistedInput{
		ID: run.ID(), Release: run.Release(), Status: run.Status(), Version: run.Version(),
		Attempts: run.Attempts(), Reviews: reviews, CreatedAt: run.CreatedAt(), ClosedAt: run.ClosedAt(),
	})
	require.Error(t, err)
}

func TestPromptEvaluationRunRejectsCaseStabilityAndSemanticThreshold(t *testing.T) {
	startedAt := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	run, err := New(meta.ID(9004), validRelease(), startedAt)
	require.NoError(t, err)
	release := run.Release()
	for caseIndex, caseID := range release.GenerationCaseIDs {
		for attempt := 1; attempt <= release.RepetitionsPerCase; attempt++ {
			record := generationAttempt(caseID, attempt, startedAt.Add(time.Duration(caseIndex*10+attempt)*time.Minute))
			if caseID == "generation-1" && attempt <= 2 {
				record.Assertions[1].Status = AssertionFailed
				record.Assertions[1].Detail = "case goal missed"
			}
			if caseID == "generation-2" && attempt == 1 {
				record.Semantic.Scores.CrossDimensionQuality = 2
			}
			require.NoError(t, run.AddAttempt(record))
		}
	}
	require.NoError(t, run.AddAttempt(preflightAttempt(release, startedAt.Add(90*time.Minute))))
	require.NoError(t, run.CloseCollection(startedAt.Add(2*time.Hour)))
	addApprovingReviews(t, run, startedAt.Add(3*time.Hour))
	require.NoError(t, run.Finalize("release-owner", "record failed thresholds", startedAt.Add(4*time.Hour)))

	require.Equal(t, StatusRejected, run.Status())
	codes := reasonCodes(run.Gate())
	require.Contains(t, codes, "case_assertion_stability_failed")
	require.Contains(t, codes, "semantic_score_below_minimum")
}

func TestPromptEvaluationRunRestoreRejectsTamperedGate(t *testing.T) {
	startedAt := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	run, err := New(meta.ID(9005), validRelease(), startedAt)
	require.NoError(t, err)
	addCompleteAttempts(t, run, startedAt)
	closedAt := startedAt.Add(2 * time.Hour)
	finalizedAt := startedAt.Add(4 * time.Hour)
	require.NoError(t, run.CloseCollection(closedAt))
	addApprovingReviews(t, run, startedAt.Add(3*time.Hour))
	require.NoError(t, run.Finalize("release-owner", "approved", finalizedAt))

	restored, err := Restore(PersistedInput{
		ID: run.ID(), Release: run.Release(), Status: run.Status(), Version: run.Version(),
		Attempts: run.Attempts(), Reviews: run.Reviews(), CreatedAt: run.CreatedAt(),
		ClosedAt: run.ClosedAt(), FinalizedAt: run.FinalizedAt(), FinalizedBy: run.FinalizedBy(),
		FinalReason: run.FinalReason(), Gate: run.Gate(),
	})
	require.NoError(t, err)
	require.True(t, restored.IsPublishEvidence())

	tampered := run.Gate()
	tampered.Passed = false
	_, err = Restore(PersistedInput{
		ID: run.ID(), Release: run.Release(), Status: run.Status(), Version: run.Version(),
		Attempts: run.Attempts(), Reviews: run.Reviews(), CreatedAt: run.CreatedAt(),
		ClosedAt: run.ClosedAt(), FinalizedAt: run.FinalizedAt(), FinalizedBy: run.FinalizedBy(),
		FinalReason: run.FinalReason(), Gate: tampered,
	})
	require.Error(t, err)
}

func TestAssertionGroupsAllowSemanticResolutionButNeverHideFailure(t *testing.T) {
	result := GateResult{}
	pendingThenPassed := generationAttempt("generation-1", 1, time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC))
	pendingThenPassed.Assertions = append(pendingThenPassed.Assertions,
		AssertionReceipt{Type: "forbidden_claims_absent", Scope: AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "deterministic-v1", Status: AssertionPendingSemantic},
		AssertionReceipt{Type: "forbidden_claims_absent", Scope: AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "semantic-v1", Status: AssertionPassed},
	)
	casePresent, casePassed := evaluateAssertionGroups(&result, pendingThenPassed)
	require.True(t, casePresent)
	require.True(t, casePassed)
	require.Empty(t, result.Reasons)

	result = GateResult{}
	failedThenPassed := pendingThenPassed
	failedThenPassed.Assertions[len(failedThenPassed.Assertions)-2].Status = AssertionFailed
	evaluateAssertionGroups(&result, failedThenPassed)
	require.Contains(t, reasonCodes(&result), "hard_assertion_not_passed")
}

func TestAttemptKeepsRepeatedAssertionTypeIndependentByOrdinal(t *testing.T) {
	attempt := generationAttempt("generation-1", 1, time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC))
	attempt.Assertions = append(attempt.Assertions,
		AssertionReceipt{Type: "forbid_dimension_group", Scope: AssertionScopeCase, Ordinal: 1, Hard: true, Evaluator: "deterministic-v1", Status: AssertionPassed},
		AssertionReceipt{Type: "forbid_dimension_group", Scope: AssertionScopeCase, Ordinal: 2, Hard: true, Evaluator: "deterministic-v1", Status: AssertionPassed},
	)
	require.NoError(t, attempt.Validate())

	duplicated := cloneAttempt(attempt)
	duplicated.Assertions = append(duplicated.Assertions,
		AssertionReceipt{Type: "forbid_dimension_group", Scope: AssertionScopeCase, Ordinal: 1, Hard: true, Evaluator: "deterministic-v1", Status: AssertionPassed},
	)
	require.Error(t, duplicated.Validate())
}

func TestFailedGenerationAttemptRequiresAuditedCancellationBeforeReview(t *testing.T) {
	startedAt := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	attempt := generationAttempt("generation-1", 1, startedAt)
	attempt.ProviderReceipt = nil
	attempt.RawOutput = nil
	attempt.NormalizedOutput = nil
	attempt.OutputFingerprint = ""
	attempt.Semantic = nil
	attempt.Failure = &AttemptFailure{
		Stage: "provider_execution", Code: "provider_timeout", SafeMessage: "Provider timed out", Retryable: true, ResultUnknown: true,
	}
	attempt.Assertions = []AssertionReceipt{{
		Type: "provider_execution", Scope: AssertionScopeDefault, Ordinal: 1, Hard: true,
		Evaluator: "runner-v1", Status: AssertionFailed, Detail: "provider_timeout",
	}}
	require.NoError(t, attempt.Validate())

	run, err := New(meta.ID(9010), validRelease(), startedAt)
	require.NoError(t, err)
	require.NoError(t, run.AddAttempt(attempt))
	for caseIndex, caseID := range run.Release().GenerationCaseIDs {
		for repetition := 1; repetition <= RequiredRepetitionsPerCase; repetition++ {
			if caseID == attempt.CaseID && repetition == attempt.Attempt {
				continue
			}
			require.NoError(t, run.AddAttempt(generationAttempt(caseID, repetition, startedAt.Add(time.Duration(caseIndex*10+repetition)*time.Minute))))
		}
	}
	require.NoError(t, run.AddAttempt(preflightAttempt(run.Release(), startedAt.Add(90*time.Minute))))
	require.NoError(t, run.CloseCollection(startedAt.Add(2*time.Hour)))
	require.Equal(t, 1, run.FailedAttemptCount())
	require.True(t, run.CanCancel())
	require.Error(t, run.AddHumanReview(HumanReview{
		CaseID: attempt.CaseID, Attempt: attempt.Attempt, Role: ReviewRoleAssessmentSemantics,
		Reviewer: "reviewer-1", Decision: ReviewDecisionReject, ReviewedAt: startedAt.Add(3 * time.Hour), Reason: "technical failure",
	}))
	require.Error(t, run.Finalize("release-owner", "must not finalize technical failure", startedAt.Add(3*time.Hour)))
	require.NoError(t, run.Cancel("release-owner", "technical failure requires a clean rerun", startedAt.Add(4*time.Hour)))
	require.Equal(t, StatusCanceled, run.Status())
	require.NotNil(t, run.ClosedAt())
	require.Len(t, run.Attempts(), RequiredGenerationAttempts+1)

	restored, err := Restore(PersistedInput{
		ID: run.ID(), Release: run.Release(), Status: run.Status(), Version: run.Version(), Attempts: run.Attempts(), Reviews: run.Reviews(),
		CreatedAt: run.CreatedAt(), ClosedAt: run.ClosedAt(), CanceledAt: run.CanceledAt(), CanceledBy: run.CanceledBy(), CancelReason: run.CancelReason(),
	})
	require.NoError(t, err)
	require.Equal(t, 1, restored.FailedAttemptCount())
	require.False(t, restored.CanCancel())
}

func TestSuccessfulAwaitingReviewRunCannotBypassReviewThroughCancellation(t *testing.T) {
	startedAt := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	run, err := New(meta.ID(9011), validRelease(), startedAt)
	require.NoError(t, err)
	addCompleteAttempts(t, run, startedAt)
	require.NoError(t, run.CloseCollection(startedAt.Add(2*time.Hour)))
	require.False(t, run.CanCancel())
	require.ErrorIs(t, run.Cancel("release-owner", "bypass review", startedAt.Add(3*time.Hour)), ErrCancellationNotAllowed)
}

func TestPromptEvaluationAttemptExecutionCheckpointGuardsDispatchAndOrder(t *testing.T) {
	startedAt := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	run, err := New(meta.ID(9012), validRelease(), startedAt)
	require.NoError(t, err)
	require.NoError(t, run.AddAttempt(preflightAttempt(run.Release(), startedAt.Add(time.Second))))

	caseID, attempt, ok := run.NextPendingGenerationAttempt()
	require.True(t, ok)
	require.Equal(t, "generation-1", caseID)
	require.Equal(t, 1, attempt)

	claimedAt := startedAt.Add(time.Minute)
	checkpoint := AttemptExecution{
		CaseID: caseID, Attempt: attempt, Owner: "event-1",
		InvocationID: "ai-prompt-eval:9012:generation-1:1",
		Phase:        AttemptExecutionPrepared, ClaimedAt: claimedAt, LeaseExpiresAt: claimedAt.Add(3 * time.Minute),
	}
	require.NoError(t, run.BeginAttemptExecution(checkpoint))
	require.Error(t, run.BeginAttemptExecution(checkpoint))
	require.Error(t, run.AddAttempt(generationAttempt(caseID, attempt, claimedAt.Add(time.Second))))
	require.Error(t, run.MarkAttemptDispatching("different-event", claimedAt.Add(time.Second)))
	require.NoError(t, run.MarkAttemptDispatching("event-1", claimedAt.Add(time.Second)))

	record := generationAttempt(caseID, attempt, claimedAt.Add(2*time.Second))
	require.Error(t, run.CompleteAttemptExecution("different-event", record))
	require.NoError(t, run.CompleteAttemptExecution("event-1", record))
	require.Nil(t, run.Execution())
	require.True(t, run.HasAttempt(caseID, attempt))

	nextCaseID, nextAttempt, ok := run.NextPendingGenerationAttempt()
	require.True(t, ok)
	require.Equal(t, "generation-1", nextCaseID)
	require.Equal(t, 2, nextAttempt)
}

func TestPromptEvaluationPreparedCheckpointCanOnlyBeReclaimedAfterLease(t *testing.T) {
	startedAt := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	run, err := New(meta.ID(9013), validRelease(), startedAt)
	require.NoError(t, err)
	require.NoError(t, run.AddAttempt(preflightAttempt(run.Release(), startedAt.Add(time.Second))))

	claimedAt := startedAt.Add(time.Minute)
	require.NoError(t, run.BeginAttemptExecution(AttemptExecution{
		CaseID: "generation-1", Attempt: 1, Owner: "event-1", InvocationID: "invocation-1",
		Phase: AttemptExecutionPrepared, ClaimedAt: claimedAt, LeaseExpiresAt: claimedAt.Add(time.Minute),
	}))
	require.Error(t, run.ReleaseExpiredPreparation(claimedAt.Add(30*time.Second)))
	require.NoError(t, run.ReleaseExpiredPreparation(claimedAt.Add(time.Minute)))
	require.Nil(t, run.Execution())

	require.NoError(t, run.BeginAttemptExecution(AttemptExecution{
		CaseID: "generation-1", Attempt: 1, Owner: "event-2", InvocationID: "invocation-1",
		Phase: AttemptExecutionPrepared, ClaimedAt: claimedAt.Add(time.Minute), LeaseExpiresAt: claimedAt.Add(2 * time.Minute),
	}))
	restored, err := Restore(PersistedInput{
		ID: run.ID(), Release: run.Release(), Status: run.Status(), Version: run.Version(),
		Attempts: run.Attempts(), Reviews: run.Reviews(), Execution: run.Execution(), CreatedAt: run.CreatedAt(),
	})
	require.NoError(t, err)
	require.Equal(t, "event-2", restored.Execution().Owner)
}

func TestPromptEvaluationRecoveryRequiresExpiredLeaseAndKeepsAudit(t *testing.T) {
	startedAt := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	run, err := NewRequested(meta.ID(9014), validRelease(), 7, "user:42", "evaluate release", startedAt)
	require.NoError(t, err)
	require.NoError(t, run.AddAttempt(preflightAttempt(run.Release(), startedAt.Add(time.Second))))
	claimedAt := startedAt.Add(time.Minute)
	require.NoError(t, run.BeginAttemptExecution(AttemptExecution{
		CaseID: "generation-1", Attempt: 1, Owner: "event-1", InvocationID: "invocation-1",
		Phase: AttemptExecutionPrepared, ClaimedAt: claimedAt, LeaseExpiresAt: claimedAt.Add(time.Minute),
	}))
	_, _, err = run.RequestRecovery("recovery-1", "user:88", "worker delivery exhausted", claimedAt.Add(30*time.Second))
	require.ErrorContains(t, err, "has not expired")
	caseID, attempt, err := run.RequestRecovery("recovery-1", "user:88", "worker delivery exhausted", claimedAt.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, "generation-1", caseID)
	require.Equal(t, 1, attempt)
	require.Len(t, run.Recoveries(), 1)
	_, _, err = run.RequestRecovery("recovery-1", "user:88", "duplicate", claimedAt.Add(2*time.Minute))
	require.ErrorContains(t, err, "duplicated")

	restored, err := Restore(PersistedInput{
		ID: run.ID(), Release: run.Release(), Status: run.Status(), Version: run.Version(), Attempts: run.Attempts(),
		Execution: run.Execution(), Recoveries: run.Recoveries(), RequestedOrgID: run.RequestedOrgID(),
		RequestedBy: run.RequestedBy(), RequestReason: run.RequestReason(), CreatedAt: run.CreatedAt(),
	})
	require.NoError(t, err)
	require.Equal(t, "user:88", restored.Recoveries()[0].Actor)
}

func TestPromptEvaluationAutomaticRecoveryRequiresExactExpiredPreparedInvocation(t *testing.T) {
	startedAt := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	newPreparedRun := func(id meta.ID) *PromptEvaluationRun {
		run, err := NewRequested(id, validRelease(), 7, "user:42", "evaluate release", startedAt)
		require.NoError(t, err)
		require.NoError(t, run.AddAttempt(preflightAttempt(run.Release(), startedAt.Add(time.Second))))
		require.NoError(t, run.BeginAttemptExecution(AttemptExecution{
			CaseID: "generation-1", Attempt: 1, Owner: "event-1", InvocationID: "invocation-1",
			Phase: AttemptExecutionPrepared, ClaimedAt: startedAt.Add(time.Minute), LeaseExpiresAt: startedAt.Add(2 * time.Minute),
		}))
		return run
	}

	prepared := newPreparedRun(meta.ID(9017))
	leaseExpiresAt := startedAt.Add(2 * time.Minute)
	_, _, err := prepared.RequestExpiredPreparationRecovery("auto-1", "different", leaseExpiresAt, "system:scanner", "expired prepared", startedAt.Add(3*time.Minute))
	require.ErrorIs(t, err, ErrRecoveryNotAllowed)
	_, _, err = prepared.RequestExpiredPreparationRecovery("auto-1", "invocation-1", leaseExpiresAt.Add(time.Second), "system:scanner", "stale checkpoint", startedAt.Add(3*time.Minute))
	require.ErrorIs(t, err, ErrRecoveryNotAllowed)
	_, _, err = prepared.RequestExpiredPreparationRecovery("auto-1", "invocation-1", leaseExpiresAt, "system:scanner", "expired prepared", startedAt.Add(90*time.Second))
	require.ErrorIs(t, err, ErrRecoveryNotAllowed)
	caseID, attempt, err := prepared.RequestExpiredPreparationRecovery("auto-1", "invocation-1", leaseExpiresAt, "system:scanner", "expired prepared", startedAt.Add(3*time.Minute))
	require.NoError(t, err)
	require.Equal(t, "generation-1", caseID)
	require.Equal(t, 1, attempt)

	dispatching := newPreparedRun(meta.ID(9018))
	require.NoError(t, dispatching.MarkAttemptDispatching("event-1", startedAt.Add(90*time.Second)))
	_, _, err = dispatching.RequestExpiredPreparationRecovery("auto-2", "invocation-1", leaseExpiresAt, "system:scanner", "must not replay", startedAt.Add(3*time.Minute))
	require.ErrorIs(t, err, ErrRecoveryNotAllowed)
	require.Empty(t, dispatching.Recoveries())
}

func TestPromptEvaluationCancelIsSafeOnlyBeforeDispatch(t *testing.T) {
	startedAt := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	newRun := func(id meta.ID) *PromptEvaluationRun {
		run, err := NewRequested(id, validRelease(), 7, "user:42", "evaluate release", startedAt)
		require.NoError(t, err)
		require.NoError(t, run.AddAttempt(preflightAttempt(run.Release(), startedAt.Add(time.Second))))
		require.NoError(t, run.BeginAttemptExecution(AttemptExecution{
			CaseID: "generation-1", Attempt: 1, Owner: "event-1", InvocationID: "invocation-1",
			Phase: AttemptExecutionPrepared, ClaimedAt: startedAt.Add(time.Minute), LeaseExpiresAt: startedAt.Add(3 * time.Minute),
		}))
		return run
	}

	prepared := newRun(meta.ID(9015))
	require.NoError(t, prepared.Cancel("user:88", "stop before incurring further cost", startedAt.Add(2*time.Minute)))
	require.Equal(t, StatusCanceled, prepared.Status())
	require.Nil(t, prepared.Execution())
	require.True(t, prepared.Status().IsTerminal())
	_, _, pending := prepared.NextPendingGenerationAttempt()
	require.False(t, pending)
	_, err := Restore(PersistedInput{
		ID: prepared.ID(), Release: prepared.Release(), Status: prepared.Status(), Version: prepared.Version(), Attempts: prepared.Attempts(),
		RequestedOrgID: prepared.RequestedOrgID(), RequestedBy: prepared.RequestedBy(), RequestReason: prepared.RequestReason(),
		CreatedAt: prepared.CreatedAt(), CanceledAt: prepared.CanceledAt(), CanceledBy: prepared.CanceledBy(), CancelReason: prepared.CancelReason(),
	})
	require.NoError(t, err)

	dispatching := newRun(meta.ID(9016))
	require.NoError(t, dispatching.MarkAttemptDispatching("event-1", startedAt.Add(2*time.Minute)))
	require.Error(t, dispatching.Cancel("user:88", "unsafe cancellation", startedAt.Add(4*time.Minute)))
	require.Equal(t, StatusCollecting, dispatching.Status())
}

func validRelease() ReleaseIdentity {
	caseIDs := make([]string, RequiredGenerationCaseCount)
	for index := range caseIDs {
		caseIDs[index] = "generation-" + string(rune('1'+index))
	}
	return ReleaseIdentity{
		Suite:        SuiteRef{ID: "cross-dimension-participant-scale-v1", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("suite")), GitBlobSHA: "suite-blob"},
		Prompt:       aiexplanation.PromptRef{TemplateID: "ai-explanation", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("prompt")), GitBlobSHA: "prompt-blob"},
		Profile:      aiexplanation.ProfileRef{ID: "participant-scale", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("profile"))},
		InputSchema:  SchemaRef{Version: aiexplanation.InputSchemaVersionV1, Fingerprint: aiexplanation.NewFingerprint([]byte("input-schema"))},
		OutputSchema: SchemaRef{Version: aiexplanation.OutputSchemaVersionV1, Fingerprint: aiexplanation.NewFingerprint([]byte("output-schema"))},
		Provider:     aiexplanation.ProviderExecutionSpec{Route: "balanced_text_v1", RouteRevision: "v1", ResolvedProvider: "provider-a", ResolvedModel: "model-a", Fingerprint: aiexplanation.NewFingerprint([]byte("provider-route"))},
		Decoding:     DecodingParameters{MaxOutputTokens: 3000}, GenerationCaseIDs: caseIDs,
		SemanticEvaluator: validSemanticEvaluatorSpec(),
		PreflightCaseID:   "preflight-ineligible", PreflightRejectionReason: "insufficient_eligible_dimensions",
		RepetitionsPerCase: RequiredRepetitionsPerCase,
	}
}

func addCompleteAttempts(t *testing.T, run *PromptEvaluationRun, startedAt time.Time) {
	t.Helper()
	release := run.Release()
	for caseIndex, caseID := range release.GenerationCaseIDs {
		for attempt := 1; attempt <= release.RepetitionsPerCase; attempt++ {
			require.NoError(t, run.AddAttempt(generationAttempt(caseID, attempt, startedAt.Add(time.Duration(caseIndex*10+attempt)*time.Minute))))
		}
	}
	require.NoError(t, run.AddAttempt(preflightAttempt(release, startedAt.Add(90*time.Minute))))
}

func generationAttempt(caseID string, attempt int, startedAt time.Time) AttemptRecord {
	normalized := []byte(`{"summary":"synthetic evaluation output"}`)
	receipt := aiexplanation.ProviderReceipt{
		InvocationID: caseID + "-attempt", RequestID: "request-id", Provider: "provider-a", Model: "model-a",
		InputTokens: 100, OutputTokens: 200, Latency: time.Second,
	}
	return AttemptRecord{
		CaseID: caseID, Attempt: attempt, Stage: AttemptStageGeneration,
		StartedAt: startedAt, FinishedAt: startedAt.Add(time.Second), ProviderCallCount: 1,
		ProviderReceipt: &receipt, RawOutput: append([]byte(nil), normalized...), NormalizedOutput: normalized,
		OutputFingerprint: aiexplanation.NewFingerprint(normalized),
		Assertions: []AssertionReceipt{
			{Type: "output_schema_valid", Scope: AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "contract-v1", Status: AssertionPassed},
			{Type: "case_goal", Scope: AssertionScopeCase, Ordinal: 1, Evaluator: "semantic-v1", Status: AssertionPassed},
		},
		Semantic: &SemanticReceipt{EvaluatorVersion: "semantic-rubric-v1", ProviderReceipt: aiexplanation.ProviderReceipt{
			InvocationID: "semantic-" + caseID + "-attempt", RequestID: "semantic-request-id", Provider: "judge-provider", Model: "judge-model", Latency: time.Second,
		}, Scores: SemanticScores{
			Faithfulness: 5, CrossDimensionQuality: 5, SuggestionActionability: 5, AudienceClarity: 5, Concision: 5,
		}, Rationale: "synthetic evaluation rationale"},
	}
}

func validSemanticEvaluatorSpec() SemanticEvaluatorSpec {
	return SemanticEvaluatorSpec{
		Version: "semantic-rubric-v1",
		Prompt: aiexplanation.PromptRef{
			TemplateID: "ai-explanation-semantic-evaluator", Version: "v1",
			Fingerprint: aiexplanation.NewFingerprint([]byte("semantic-prompt")), GitBlobSHA: "semantic-prompt-blob",
		},
		OutputSchema: SchemaRef{Version: "ai-explanation-semantic-evaluation-output/v1", Fingerprint: aiexplanation.NewFingerprint([]byte("semantic-schema"))},
		Provider: aiexplanation.ProviderExecutionSpec{
			Route: "semantic_judge_v1", RouteRevision: "v1", ResolvedProvider: "judge-provider", ResolvedModel: "judge-model",
			Fingerprint: aiexplanation.NewFingerprint([]byte("semantic-route")),
		},
		Decoding: DecodingParameters{MaxOutputTokens: 2000},
	}
}

func preflightAttempt(release ReleaseIdentity, startedAt time.Time) AttemptRecord {
	return AttemptRecord{
		CaseID: release.PreflightCaseID, Attempt: 1, Stage: AttemptStagePreflight,
		StartedAt: startedAt, FinishedAt: startedAt.Add(time.Second), ProviderCallCount: 0,
		RejectionReason: release.PreflightRejectionReason,
		Assertions: []AssertionReceipt{
			{Type: "provider_call_count", Scope: AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "preflight-v1", Status: AssertionPassed},
			{Type: "rejection_reason", Scope: AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "preflight-v1", Status: AssertionPassed},
		},
	}
}

func addApprovingReviews(t *testing.T, run *PromptEvaluationRun, reviewedAt time.Time) {
	t.Helper()
	for _, attempt := range run.sortedGenerationAttempts() {
		for _, role := range []ReviewRole{ReviewRoleAssessmentSemantics, ReviewRoleSafetyProduct} {
			require.NoError(t, run.AddHumanReview(HumanReview{
				CaseID: attempt.CaseID, Attempt: attempt.Attempt, Role: role, Reviewer: string(role) + "-reviewer",
				Decision: ReviewDecisionApprove, ReviewedAt: reviewedAt, Reason: "reviewed against the frozen output and rubric",
			}))
		}
	}
}

func reasonCodes(gate *GateResult) []string {
	if gate == nil {
		return nil
	}
	values := make([]string, 0, len(gate.Reasons))
	for _, reason := range gate.Reasons {
		values = append(values, reason.Code)
	}
	return values
}
