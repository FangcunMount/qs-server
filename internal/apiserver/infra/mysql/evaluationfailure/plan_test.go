package evaluationfailure

import (
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
	require.Equal(t, q.RetryAt, f.Due)
	require.Equal(t, q.FutureRetry.EventID(), f.Future.ID)
	require.Equal(t, q.Run.RetryDecision(), f.Run.RetryDecision)
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
	require.Equal(t, q.Run.RetryDecision(), restored.RetryDecision())
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
