package operator

import (
	"context"
	"errors"
	"testing"
	"time"

	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	events "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	storeport "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

type scheduleRootKey struct{}
type scheduledRetryRuns struct {
	evaluationrun.Repository
	value          evalrun.EvaluationRun
	writes         int
	policyConflict bool
}

func cloneScheduledRun(r evalrun.EvaluationRun) evalrun.EvaluationRun {
	return evalrun.Reconstruct(evalrun.ReconstructInput{RunID: r.ID(), AssessmentID: r.AssessmentID(), Attempt: r.Attempt(), Failure: r.Failure(), TraceID: r.TraceID(), InputSnapshotRef: r.InputSnapshotRef(), ClaimToken: r.ClaimToken(), LeaseExpiresAt: r.LeaseExpiresAt(), StartedAt: r.StartedAt(), FinishedAt: r.FinishedAt(), Origin: r.Origin(), ActionRequestID: r.ActionRequestID(), RetryDecision: r.RetryDecision()})
}
func (r *scheduledRetryRuns) FindLatestByAssessmentID(context.Context, uint64) (*evalrun.EvaluationRun, error) {
	v := cloneScheduledRun(r.value)
	return &v, nil
}
func (r *scheduledRetryRuns) AuthorizeRetry(ctx context.Context, q evaluationrun.RetryAuthorizationRequest) (*evalrun.EvaluationRun, error) {
	if ctx.Value(scheduleRootKey{}) != true {
		return nil, transaction.ErrTransactionRequired
	}
	r.writes++
	if err := r.value.AuthorizeOneRetry(q.Origin, q.RequestID, q.EventID, q.AuthorizedAt); err != nil {
		return nil, err
	}
	result := cloneScheduledRun(r.value)
	if r.policyConflict {
		in := result.RetryDecision()
		in.PolicyVersion = "concurrent-policy"
		result = evalrun.Reconstruct(evalrun.ReconstructInput{RunID: result.ID(), AssessmentID: result.AssessmentID(), Attempt: result.Attempt(), Failure: result.Failure(), StartedAt: result.StartedAt(), RetryDecision: in})
	}
	return &result, nil
}

type authorizationStore struct {
	storeport.Store
	record *model.Record
	saves  int
}

func (s *authorizationStore) Find(ctx context.Context, candidate model.Frozen) (*model.Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ctx.Value(scheduleRootKey{}) == true {
		return nil, transaction.ErrTransactionRequired
	}
	if s.record == nil {
		return nil, transaction.ErrNotFound
	}
	if err := s.record.Plan.MatchInput(candidate); err != nil {
		return nil, err
	}
	r := *s.record
	return &r, nil
}
func (s *authorizationStore) SaveFrozen(ctx context.Context, candidate model.Frozen) (model.Frozen, error) {
	if ctx.Value(scheduleRootKey{}) != true {
		return model.Frozen{}, transaction.ErrTransactionRequired
	}
	s.saves++
	if s.record == nil {
		s.record = &model.Record{Plan: candidate, State: model.Ready, Version: 1}
	}
	if err := s.record.Plan.MatchInput(candidate); err != nil {
		return model.Frozen{}, err
	}
	return s.record.Plan, nil
}

type governedRoutes struct{}

func (governedRoutes) GetTopicForEvent(string) (string, bool) { return model.LifecycleTopic, true }

func TestScheduledGovernanceBothOriginsReuseFirstAuthorizationWithoutOutbox(t *testing.T) {
	for _, origin := range []retrygovernance.AttemptOrigin{retrygovernance.AttemptOriginManual, retrygovernance.AttemptOriginForce} {
		t.Run(string(origin), func(t *testing.T) {
			a := scheduledFailedAssessment(t)
			attempt, retryable, action := 3, true, "retry"
			if origin == retrygovernance.AttemptOriginForce {
				attempt, retryable, action = 1, false, "force_retry"
			}
			at := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
			initial := evalrun.NewEvaluationRunWithAttempt(1, attempt)
			require.NoError(t, initial.Claim(evalrun.ClaimInput{Token: "original", ClaimedAt: at.Add(-time.Minute), LeaseExpiresAt: at.Add(time.Minute)}))
			require.NoError(t, initial.Fail(at.Add(-time.Second), evalrun.Failure{Kind: evalrun.FailureKindCalculation, Retryable: retryable}))
			runs := &scheduledRetryRuns{value: initial}
			store := &authorizationStore{}
			old := &eventStagerStub{}
			roots := 0
			runner := apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
				roots++
				err := fn(context.WithValue(ctx, scheduleRootKey{}, true))
				if err != nil {
					return err
				}
				return errors.New("controlled lost commit reply")
			})
			service := NewGovernedRetryService(&assessmentRepoStub{items: map[uint64]*assessment.Assessment{1: a}}, runs, runner, old, &accessCheckerStub{}, WithRetrySchedules(store, model.Builder{Resolver: governedRoutes{}})).(*governedRetryService)
			service.now = func() time.Time { at = at.Add(time.Second); return at }
			ctx := appauthz.WithSnapshot(t.Context(), &appauthz.Snapshot{Permissions: []appauthz.Permission{{Resource: appauthz.AssessmentResource, Action: action, Mode: appauthz.AuthorizationModeUnconditional}}})
			command := GovernedRetryCommand{AssessmentID: 1, ExpectedAttempt: attempt, Origin: origin, RequestID: "operation-1", Reason: "explicit recovery", AuthorizationSubject: "user:9", AuthorizationAction: action}
			accepted, err := service.Authorize(ctx, Actor{OrgID: 1, OperatorUserID: 9}, command)
			require.NoError(t, err)
			require.Equal(t, attempt, accepted.Attempt().Number)
			first := store.record.Plan.Bytes()
			firstWire := store.record.Plan.Message().Input().Payload
			repeated, err := service.Authorize(ctx, Actor{OrgID: 1, OperatorUserID: 9}, command)
			require.NoError(t, err)
			require.Equal(t, accepted.ID(), repeated.ID())
			require.Equal(t, 1, roots)
			require.Equal(t, 1, runs.writes)
			require.Zero(t, old.calls)
			require.Equal(t, first, store.record.Plan.Bytes())
			require.Equal(t, firstWire, store.record.Plan.Message().Input().Payload)
			require.Contains(t, string(store.record.Plan.Authority().Authorization), "explicit recovery")
			command.Reason = "changed input"
			_, err = service.Authorize(ctx, Actor{OrgID: 1, OperatorUserID: 9}, command)
			require.ErrorIs(t, err, model.ErrConflict)
			require.Equal(t, 1, roots)
		})
	}
}

func TestScheduledGovernanceConflictingActualDecisionRollsBackTheOriginalRoot(t *testing.T) {
	a := scheduledFailedAssessment(t)
	at := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	initial := evalrun.NewEvaluationRunWithAttempt(1, 3)
	require.NoError(t, initial.Claim(evalrun.ClaimInput{Token: "original", ClaimedAt: at.Add(-time.Minute), LeaseExpiresAt: at.Add(time.Minute)}))
	require.NoError(t, initial.Fail(at.Add(-time.Second), evalrun.Failure{Kind: evalrun.FailureKindCalculation, Retryable: true}))
	runs := &scheduledRetryRuns{value: initial, policyConflict: true}
	store := &authorizationStore{}
	old := &eventStagerStub{}
	runner := apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		before := cloneScheduledRun(runs.value)
		err := fn(context.WithValue(ctx, scheduleRootKey{}, true))
		if err != nil {
			runs.value = before
			store.record = nil
		}
		return err
	})
	service := NewGovernedRetryService(&assessmentRepoStub{items: map[uint64]*assessment.Assessment{1: a}}, runs, runner, old, &accessCheckerStub{}, WithRetrySchedules(store, model.Builder{Resolver: governedRoutes{}})).(*governedRetryService)
	service.now = func() time.Time { return at }
	ctx := appauthz.WithSnapshot(t.Context(), &appauthz.Snapshot{Permissions: []appauthz.Permission{{Resource: appauthz.AssessmentResource, Action: "retry", Mode: appauthz.AuthorizationModeUnconditional}}})
	_, err := service.Authorize(ctx, Actor{OrgID: 1, OperatorUserID: 9}, GovernedRetryCommand{AssessmentID: 1, ExpectedAttempt: 3, Origin: retrygovernance.AttemptOriginManual, RequestID: "operation-1", Reason: "controlled", AuthorizationSubject: "user:9", AuthorizationAction: "retry"})
	require.ErrorIs(t, err, events.ErrUnresolved)
	require.ErrorIs(t, err, model.ErrConflict)
	require.Nil(t, store.record)
	require.Equal(t, retrygovernance.DispositionManualRequired, runs.value.RetryDecision().Disposition)
	require.Zero(t, old.calls)
}

func scheduledFailedAssessment(t *testing.T) *assessment.Assessment {
	t.Helper()
	a, err := assessment.NewAssessment(1, testee.NewID(101), assessment.NewQuestionnaireRefByCode(meta.NewCode("Q"), "1"), assessment.NewAnswerSheetRef(meta.FromUint64(201)), assessment.NewAdhocOrigin(), assessment.WithID(meta.FromUint64(1)), assessment.WithEvaluationModel(assessment.NewScaleEvaluationModelRef(meta.ZeroID, meta.NewCode("S"), "1", "fixture")))
	require.NoError(t, err)
	require.NoError(t, a.Submit())
	require.NoError(t, a.MarkAsFailed("controlled"))
	return a
}
