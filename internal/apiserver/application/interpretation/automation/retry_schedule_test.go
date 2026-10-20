package automation

import (
	"context"
	"errors"
	"testing"
	"time"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	run "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	events "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	storeport "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

type reportAuthorizationRootKey struct{}
type governedGenerations struct {
	generation.Repository
	value *generation.ReportGeneration
}

func (g *governedGenerations) FindByID(context.Context, meta.ID) (*generation.ReportGeneration, error) {
	return g.value, nil
}

type governedRuns struct {
	run.Repository
	value  *run.InterpretationRun
	writes int
}

func cloneGovernedRun(r *run.InterpretationRun) *run.InterpretationRun {
	copy, err := run.Restore(run.RestoreInput{ID: r.ID(), GenerationID: r.GenerationID(), Attempt: r.Attempt(), Status: r.Status(), Failure: r.Failure(), TraceID: r.TraceID(), StartedAt: r.StartedAt(), LeaseExpiresAt: r.LeaseExpiresAt(), FinishedAt: r.FinishedAt(), Origin: r.Origin(), RetryDecision: r.RetryDecision(), RetryEventEvidence: r.RetryEventEvidence(), FailedTransactionEvidence: r.FailedTransactionEvidence(), ClaimHistory: r.ClaimHistory(), RecoveryCount: r.RecoveryCount(), LastReclaimedAt: r.LastReclaimedAt()})
	if err != nil {
		panic(err)
	}
	return copy
}
func (r *governedRuns) FindByID(context.Context, run.ID) (*run.InterpretationRun, error) {
	return cloneGovernedRun(r.value), nil
}
func (r *governedRuns) AuthorizeRetry(ctx context.Context, q run.RetryAuthorizationRequest) (*run.InterpretationRun, error) {
	if ctx.Value(reportAuthorizationRootKey{}) != true {
		return nil, transaction.ErrTransactionRequired
	}
	r.writes++
	if err := r.value.AuthorizeOneRetryWithEvidence(q.Origin, q.RequestID, q.AuthorizedAt, q.EventEvidence); err != nil {
		return nil, err
	}
	return cloneGovernedRun(r.value), nil
}

type reportAuthorizationStore struct {
	storeport.Store
	record *model.Record
	saves  int
}

func (s *reportAuthorizationStore) Find(ctx context.Context, candidate model.Frozen) (*model.Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ctx.Value(reportAuthorizationRootKey{}) == true {
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
func (s *reportAuthorizationStore) SaveFrozen(ctx context.Context, candidate model.Frozen) (model.Frozen, error) {
	if ctx.Value(reportAuthorizationRootKey{}) != true {
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

type reportAuthorizationGuard struct {
	calls int
	fail  bool
}

func (g *reportAuthorizationGuard) GuardAuthorization(ctx context.Context, _ model.Frozen) error {
	if ctx.Value(reportAuthorizationRootKey{}) != true {
		return transaction.ErrTransactionRequired
	}
	g.calls++
	if g.fail {
		return transaction.ErrStaleExecution
	}
	return nil
}

type reportRetryRoutes struct{}

func (reportRetryRoutes) GetTopicForEvent(string) (string, bool) { return model.LifecycleTopic, true }

func TestGovernedReportSchedulePreservesFirstEvidenceAndOriginalAttempt(t *testing.T) {
	for _, origin := range []retrygovernance.AttemptOrigin{retrygovernance.AttemptOriginManual, retrygovernance.AttemptOriginForce} {
		t.Run(string(origin), func(t *testing.T) {
			at := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
			attempt, retryable := 3, true
			if origin == retrygovernance.AttemptOriginForce {
				attempt, retryable = 1, false
			}
			g, err := generation.New(meta.FromUint64(51), generation.Key{OutcomeID: meta.FromUint64(71), ReportType: policy.ReportTypeStandard, TemplateVersion: "v1"}, at.Add(-time.Minute))
			require.NoError(t, err)
			original, err := run.NewPending(meta.FromUint64(61), g.ID(), attempt)
			require.NoError(t, err)
			require.NoError(t, original.StartWithLease(at.Add(-time.Second), "original", at.Add(time.Minute)))
			require.NoError(t, g.Begin(original.ID(), *original.StartedAt()))
			require.NoError(t, original.Fail(at, run.Failure{Kind: run.FailureKindBuild, Code: "controlled", SafeMessage: "fixture", Retryable: retryable}))
			require.NoError(t, g.Fail(original.ID(), at))
			runs := &governedRuns{value: original}
			store := &reportAuthorizationStore{}
			guard := &reportAuthorizationGuard{}
			roots := 0
			tx := apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
				roots++
				if err := fn(context.WithValue(ctx, reportAuthorizationRootKey{}, true)); err != nil {
					return err
				}
				return errors.New("controlled lost root commit reply")
			})
			facts := &outcomeRepoStub{record: evaluationfact.NewRecord(evaluationfact.NewRecordInput{ID: meta.FromUint64(71), OrgID: 7, AssessmentID: meta.FromUint64(41), TesteeID: 9})}
			service := NewGovernedRetryService(&governedGenerations{value: g}, runs, facts, tx, nil, WithRetrySchedules(store, model.Builder{Resolver: reportRetryRoutes{}}, guard)).(*governedRetryService)
			service.now = func() time.Time { at = at.Add(time.Second); return at }
			command := GovernedRetryCommand{OrgID: 7, GenerationID: g.ID(), ExpectedAttempt: attempt, Origin: origin, RequestID: "operation-1", Reason: "explicit original authorization"}
			accepted, err := service.Authorize(t.Context(), command)
			require.NoError(t, err)
			require.Equal(t, attempt, accepted.Attempt())
			first := store.record.Plan.Bytes()
			firstProof := accepted.RetryEventEvidence()
			require.NotNil(t, firstProof)
			again, err := service.Authorize(t.Context(), command)
			require.NoError(t, err)
			require.Equal(t, firstProof, again.RetryEventEvidence())
			require.Equal(t, 1, roots)
			require.Equal(t, 1, runs.writes)
			require.Equal(t, 1, guard.calls)
			require.Equal(t, first, store.record.Plan.Bytes())
			command.Reason = "different input"
			_, err = service.Authorize(t.Context(), command)
			require.ErrorIs(t, err, model.ErrConflict)
			require.Equal(t, 1, roots)
		})
	}
}

func TestGovernedReportScheduleRollsBackWhenGenerationOwnershipChanged(t *testing.T) {
	at := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	g, err := generation.New(meta.FromUint64(51), generation.Key{OutcomeID: meta.FromUint64(71), ReportType: policy.ReportTypeStandard, TemplateVersion: "v1"}, at.Add(-time.Minute))
	require.NoError(t, err)
	original, err := run.NewPending(meta.FromUint64(61), g.ID(), 3)
	require.NoError(t, err)
	require.NoError(t, original.StartWithLease(at.Add(-time.Second), "original", at.Add(time.Minute)))
	require.NoError(t, g.Begin(original.ID(), *original.StartedAt()))
	require.NoError(t, original.Fail(at, run.Failure{Kind: run.FailureKindBuild, Code: "controlled", SafeMessage: "fixture", Retryable: true}))
	require.NoError(t, g.Fail(original.ID(), at))
	runs := &governedRuns{value: original}
	store := &reportAuthorizationStore{}
	guard := &reportAuthorizationGuard{fail: true}
	tx := apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		err := fn(context.WithValue(ctx, reportAuthorizationRootKey{}, true))
		if err != nil {
			store.record = nil
		}
		return err
	})
	facts := &outcomeRepoStub{record: evaluationfact.NewRecord(evaluationfact.NewRecordInput{ID: meta.FromUint64(71), OrgID: 7, AssessmentID: meta.FromUint64(41), TesteeID: 9})}
	service := NewGovernedRetryService(&governedGenerations{value: g}, runs, facts, tx, nil, WithRetrySchedules(store, model.Builder{Resolver: reportRetryRoutes{}}, guard)).(*governedRetryService)
	service.now = func() time.Time { return at }
	_, err = service.Authorize(t.Context(), GovernedRetryCommand{OrgID: 7, GenerationID: g.ID(), ExpectedAttempt: 3, Origin: retrygovernance.AttemptOriginManual, RequestID: "operation-1", Reason: "controlled"})
	require.ErrorIs(t, err, events.ErrUnresolved)
	require.ErrorIs(t, err, transaction.ErrStaleExecution)
	require.Zero(t, runs.writes)
	require.Nil(t, store.record)
}
