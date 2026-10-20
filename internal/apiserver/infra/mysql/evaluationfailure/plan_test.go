package evaluationfailure

import (
	"encoding/json"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	repository "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationtransaction"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/stretchr/testify/require"
)

type routes struct{}

func (routes) GetTopicForEvent(string) (string, bool) { return "qs.evaluation.lifecycle", true }
func failureRequest(t *testing.T, failedAt time.Time, retryable ...bool) port.FailureRequest {
	t.Helper()
	a, err := assessment.NewAssessment(7, testee.NewID(8), assessment.NewQuestionnaireRefByCode(meta.NewCode("Q"), "1"), assessment.NewAnswerSheetRef(meta.FromUint64(9)), assessment.NewAdhocOrigin(), assessment.WithID(meta.FromUint64(42)), assessment.WithCreatedAt(time.Unix(50, 0)), assessment.WithEvaluationModel(assessment.NewScaleEvaluationModelRef(meta.ZeroID, meta.NewCode("SCALE"), "1", "fixture")))
	require.NoError(t, err)
	require.NoError(t, a.SubmitAt(time.Unix(100, 0)))
	a.ClearEvents()
	run := evalrun.NewEvaluationRunWithAttempt(42, 1)
	require.NoError(t, run.Claim(evalrun.ClaimInput{Token: "original-claim", ClaimedAt: time.Unix(110, 0), LeaseExpiresAt: time.Unix(170, 0)}))
	failed, err := a.PrepareFailure("model calculation failed", failedAt)
	require.NoError(t, err)
	canRetry := true
	if len(retryable) > 0 {
		canRetry = retryable[0]
	}
	require.NoError(t, run.Fail(failedAt, evalrun.Failure{Kind: evalrun.FailureKindCalculation, Message: "calculation failed", Retryable: canRetry}))
	if !canRetry {
		return port.FailureRequest{Assessment: failed, Run: run, Immediate: failed.Events()}
	}
	decision := run.RetryDecision()
	require.NotNil(t, decision.NextAttemptAt)
	future := assessment.NewEvaluationRetryRequestedEvent(failed, run.Attempt().Number, retrygovernance.AttemptOriginAutomatic, "", *decision.NextAttemptAt)
	require.NoError(t, run.AttachRetryEvent(future.EventID()))
	return port.FailureRequest{Assessment: failed, Run: run, Immediate: failed.Events(), FutureRetry: future, RetryAt: *decision.NextAttemptAt}
}
func TestFailurePlanFreezesFutureScheduleWithoutPreparingItAsHalf(t *testing.T) {
	q := failureRequest(t, time.Unix(120, 0))
	req, err := freeze(q, routes{})
	require.NoError(t, err)
	require.Len(t, req.Events, 1)
	require.Equal(t, assessment.EventTypeFailed, req.Events[0].EventType())
	f, err := read(req.Artifact)
	require.NoError(t, err)
	require.True(t, q.RetryAt.Equal(f.Due))
	require.Equal(t, q.FutureRetry.EventID(), f.Future.ID)
	requireSameRetryDecision(t, q.Run.RetryDecision(), f.Run.RetryDecision)
	require.Equal(t, q.Run.ClaimToken(), f.Run.ClaimToken)
	require.True(t, f.Due.After(*f.Assessment.FailedAt))
	raw := append([]byte(nil), f.Future.Payload...)
	encoded, err := freeze(q, routes{})
	require.NoError(t, err)
	again, err := read(encoded.Artifact)
	require.NoError(t, err)
	require.Equal(t, raw, again.Future.Payload)
	// Reconstructing the frozen root never authorizes another attempt.
	restored := evalrun.Reconstruct(f.Run)
	require.Equal(t, q.Run.ID(), restored.ID())
	require.Equal(t, q.Run.Attempt(), restored.Attempt())
	requireSameRetryDecision(t, q.Run.RetryDecision(), restored.RetryDecision())
}

func requireSameRetryDecision(t *testing.T, want, got *retrygovernance.Decision) {
	t.Helper()
	require.NotNil(t, want)
	require.NotNil(t, got)
	require.NotNil(t, want.NextAttemptAt)
	require.NotNil(t, got.NextAttemptAt)
	require.True(t, want.NextAttemptAt.Equal(*got.NextAttemptAt))
	first, second := *want, *got
	first.NextAttemptAt, second.NextAttemptAt = nil, nil
	require.Equal(t, first, second)
}

func TestFailurePlanRetainsExactInstantsAcrossTimezoneEncoding(t *testing.T) {
	for _, location := range []*time.Location{time.UTC, time.FixedZone("east", 8*60*60), time.FixedZone("west", -5*60*60)} {
		t.Run(location.String(), func(t *testing.T) {
			q := failureRequest(t, time.Unix(120, 123000000).In(location))
			request, err := freeze(q, routes{})
			require.NoError(t, err)
			f, err := read(request.Artifact)
			require.NoError(t, err)
			require.True(t, q.Assessment.FailedAt().Equal(*f.Assessment.FailedAt))
			require.True(t, q.Run.FinishedAt().Equal(*f.Run.FinishedAt))
			require.True(t, q.RetryAt.Equal(f.Due))
			requireSameRetryDecision(t, q.Run.RetryDecision(), f.Run.RetryDecision)
		})
	}
}
func TestFailureInputExcludesGeneratedClockButBindsBusinessDecision(t *testing.T) {
	first, err := freeze(failureRequest(t, time.Unix(120, 0)), routes{})
	require.NoError(t, err)
	second, err := freeze(failureRequest(t, time.Unix(125, 0)), routes{})
	require.NoError(t, err)
	require.Equal(t, first.Key, second.Key)
	require.Equal(t, first.InputHash, second.InputHash)
	require.NotEqual(t, first.Artifact, second.Artifact)
	f, err := read(first.Artifact)
	require.NoError(t, err)
	f.Run.Failure.Message = "different business failure"
	other, err := f.inputHash()
	require.NoError(t, err)
	require.NotEqual(t, first.InputHash, other)
	f, err = read(first.Artifact)
	require.NoError(t, err)
	f.Run.RetryDecision.PolicyVersion = "new-explicit-business-policy"
	other, err = f.inputHash()
	require.NoError(t, err)
	require.NotEqual(t, first.InputHash, other)
}
func TestFailurePlanRejectsLostOrEarlyFutureResponsibility(t *testing.T) {
	for _, kind := range []string{"missing-wake", "wrong-due", "wrong-attempt", "wrong-owner-scope", "mixed-business"} {
		t.Run(kind, func(t *testing.T) {
			request, err := freeze(failureRequest(t, time.Unix(120, 0)), routes{})
			require.NoError(t, err)
			f, err := read(request.Artifact)
			require.NoError(t, err)
			switch kind {
			case "missing-wake":
				f.Future = nil
			case "wrong-due":
				f.Due = f.Due.Add(-time.Second)
			case "wrong-attempt":
				f.Run.Attempt.Number++
			case "wrong-owner-scope":
				f.Future.Scope = "org:8"
			case "mixed-business":
				po := *f.Assessment
				po.OrgID++
				f.Assessment = &po
			}
			require.Error(t, f.validate())
		})
	}
}
func TestFailureFreezeDoesNotMutateOriginalRetryAuthorization(t *testing.T) {
	q := failureRequest(t, time.Unix(120, 0))
	decision := q.Run.RetryDecision()
	_, err := freeze(q, routes{})
	require.NoError(t, err)
	require.Equal(t, decision, q.Run.RetryDecision())
	po := repository.NewAssessmentMapper().ToPO(q.Assessment)
	require.Equal(t, *q.Assessment.FailedAt(), *po.FailedAt)
}

func TestFailureFutureOwnershipVersionRecoversLegacyWinnerWithoutReplacingInput(t *testing.T) {
	q := failureRequest(t, time.Unix(120, 123499999))
	request, err := freeze(q, routes{})
	require.NoError(t, err)
	current, err := read(request.Artifact)
	require.NoError(t, err)
	require.Equal(t, 2, current.Version)
	plan, err := current.retryPlan()
	require.NoError(t, err)
	require.Equal(t, current.Future.Payload, plan.Message().Input().Payload)
	require.NoError(t, plan.MatchDecision(current.Run.RetryDecision, current.Due))
	legacy := current
	legacy.Version = 1
	raw, err := json.Marshal(legacy)
	require.NoError(t, err)
	old, err := read(raw)
	require.NoError(t, err)
	require.Equal(t, 1, old.Version)
	before, err := old.inputHash()
	require.NoError(t, err)
	after, err := current.inputHash()
	require.NoError(t, err)
	require.Equal(t, before, after)
	_, err = old.retryPlan()
	require.ErrorIs(t, err, transaction.ErrInvalidPlan, "a legacy artifact cannot silently acquire new schedule ownership")
	require.Equal(t, current.Future.Payload, old.Future.Payload)
}
