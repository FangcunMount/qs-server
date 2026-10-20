package execute

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationtransaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/stretchr/testify/require"
)

type failureBoundaryStub struct {
	request       port.FailureRequest
	err           error
	pending       bool
	recoveryCalls int
}

func (s *failureBoundaryStub) CommitFailure(_ context.Context, q port.FailureRequest) (*assessment.Assessment, *evalrun.EvaluationRun, error) {
	s.request = q
	if s.err != nil {
		return nil, nil, s.err
	}
	return q.Assessment, &q.Run, nil
}
func (s *failureBoundaryStub) RecoverPending(context.Context, *assessment.Assessment) (bool, error) {
	s.recoveryCalls++
	if s.pending {
		return true, eventtransaction.ErrUnresolved
	}
	return false, nil
}
func TestFailureTransactionFinalizerKeepsFutureDecisionAndCallerIsolation(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "committed", true: "unknown"}[unknown], func(t *testing.T) {
			a := splitPhaseAssessment(t)
			a.ClearEvents()
			run := evalrun.NewEvaluationRunWithAttempt(a.ID().Uint64(), 1)
			require.NoError(t, run.Start(time.Unix(100, 0)))
			stub := &failureBoundaryStub{}
			if unknown {
				stub.err = eventtransaction.ErrUnresolved
			}
			finalizer := evaluationFailureFinalizer{failureCommitter: stub}
			err := finalizer.Finalize(t.Context(), a, &run, "model calculation failed", evalrun.Failure{Kind: evalrun.FailureKindCalculation, Message: "failed", Retryable: true})
			if unknown {
				require.ErrorIs(t, err, eventtransaction.ErrUnresolved)
				require.True(t, a.Status().IsSubmitted())
				require.Equal(t, evalrun.StatusRunning, run.Attempt().Status)
			} else {
				require.NoError(t, err)
				require.True(t, a.Status().IsFailed())
				require.Equal(t, evalrun.StatusFailed, run.Attempt().Status)
			}
			require.Len(t, stub.request.Immediate, 1)
			require.Equal(t, assessment.EventTypeFailed, stub.request.Immediate[0].EventType())
			require.NotNil(t, stub.request.FutureRetry)
			require.Equal(t, *stub.request.Run.RetryDecision().NextAttemptAt, stub.request.RetryAt)
			require.Equal(t, stub.request.Run.RetryDecision().RetryEventID, stub.request.FutureRetry.EventID())
			require.Equal(t, 1, stub.request.Run.Attempt().Number)
		})
	}
}
func TestPendingFailureBlocksEngineBeforeAnotherCalculation(t *testing.T) {
	a := splitPhaseAssessment(t)
	a.ClearEvents()
	evaluator := &countingEvaluator{key: evaluation.ExecutionIdentityScaleDefault, outcome: executionForAssessment(a, "ok")}
	runs := &stubRunRepo{}
	boundary := &failureBoundaryStub{pending: true}
	svc := NewEngine(&fakeAssessmentRepo{assessment: a}, stubInputResolver{}, withTestEvaluator(evaluator), WithRunRepository(runs), WithFailureTransactionCommitter(boundary))
	err := svc.Evaluate(t.Context(), a.ID().Uint64())
	require.True(t, errors.Is(err, eventtransaction.ErrUnresolved))
	require.Zero(t, evaluator.calls)
	require.Nil(t, runs.latest)
	require.Equal(t, 1, boundary.recoveryCalls)
}
