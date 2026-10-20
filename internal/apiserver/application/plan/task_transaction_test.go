package plan

import (
	"context"
	"errors"
	"testing"
	"time"

	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	appeventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	eventport "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/tasktransaction"
	"github.com/stretchr/testify/require"
)

type transactionOpeningStub struct {
	calls       int
	err         error
	guard       port.LocalGuard
	recovered   *domain.AssessmentTask
	handled     bool
	recoveryErr error
}

func (o *transactionOpeningStub) RecoverOpened(context.Context, *domain.AssessmentTask) (*domain.AssessmentTask, bool, error) {
	return o.recovered, o.handled, o.recoveryErr
}
func (o *transactionOpeningStub) CommitOpened(_ context.Context, q port.OpenRequest) (*domain.AssessmentTask, error) {
	o.calls++
	o.guard = q.Guard
	return q.Candidate, o.err
}

func transactionOpeningTask(t *testing.T) *domain.AssessmentTask {
	t.Helper()
	at := time.Now()
	task := domain.NewAssessmentTaskAt(domain.NewAssessmentPlanID(), 1, 7, testee.NewID(9), "scale", at.Add(-time.Minute), at.Add(-time.Hour))
	require.NoError(t, domain.NewTaskLifecycle().OpenAt(t.Context(), task, "https://example.test/first", at))
	return task
}

func TestTransactionOpeningUnknownNeverFallsBackToLegacyTaskSaveOrOutbox(t *testing.T) {
	tasks := &schedulerTaskRepoStub{}
	legacy := &openingStagerSpy{}
	opener := &transactionOpeningStub{err: eventport.ErrUnresolved}
	p := taskPersistence{tasks: tasks, enrollments: &enrollmentRepoStub{}, tx: directPlanTxRunner{}, openingOptions: TaskOpeningOptions{ReminderEnabled: true, Transaction: opener, ReminderOutbox: appeventing.ProfileBinding{Stager: legacy}}}
	task := transactionOpeningTask(t)
	staged, err := p.saveOpened(t.Context(), task)
	require.ErrorIs(t, err, eventport.ErrUnresolved)
	require.False(t, staged)
	require.Equal(t, 1, opener.calls)
	require.Zero(t, legacy.calls)
	require.Empty(t, tasks.savedTasks)
	require.Len(t, task.Events(), 1, "failed admission cannot consume original event")
}

func TestTransactionOpeningExplicitSelectionRetainsLocalGuardAndClearsCommittedEvents(t *testing.T) {
	opener := &transactionOpeningStub{}
	p := taskPersistence{openingOptions: TaskOpeningOptions{ReminderEnabled: true, Transaction: opener}}
	task := transactionOpeningTask(t)
	guard := func(context.Context, *domain.AssessmentTask) error { return nil }
	staged, err := p.saveOpenedWithGuard(t.Context(), task, guard)
	require.NoError(t, err)
	require.True(t, staged)
	require.Equal(t, 1, opener.calls)
	require.NotNil(t, opener.guard)
	require.Empty(t, task.Events())
}

// Returning an existing execution must happen before entry generation or a
// second local mutation, including when the original commit is still unknown.
type openingLookupRepo struct {
	domain.AssessmentTaskRepository
	task *domain.AssessmentTask
}

func (r openingLookupRepo) FindByID(context.Context, domain.AssessmentTaskID) (*domain.AssessmentTask, error) {
	return r.task, nil
}

func TestTransactionOpeningRecoveryNeverRegeneratesEntry(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "committed", true: "unknown"}[unknown], func(t *testing.T) {
			task := transactionOpeningTask(t)
			opener := &transactionOpeningStub{recovered: task, handled: true}
			if unknown {
				opener.recovered = nil
				opener.recoveryErr = eventport.ErrUnresolved
			}
			entry := &entryGeneratorStub{}
			service := &taskManagementService{taskRepo: openingLookupRepo{task: task}, entryGenerator: entry, persistence: taskPersistence{openingOptions: TaskOpeningOptions{ReminderEnabled: true, Transaction: opener}}}
			result, err := service.OpenTask(t.Context(), task.GetOrgID(), task.GetID().String())
			if unknown {
				require.ErrorIs(t, err, eventport.ErrUnresolved)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}
			require.Zero(t, entry.calls)
			require.Zero(t, opener.calls)
		})
	}
}

type guardedOpeningCommand struct {
	PlanCommandService
	task     *domain.AssessmentTask
	lock     *ownershipLockStub
	root     transaction.Runner
	halfErr  error
	transfer bool
	writes   int
}

func (*guardedOpeningCommand) TransactionalTaskOpening() bool { return true }
func (s *guardedOpeningCommand) OpenTaskWithGuard(ctx context.Context, _ int64, _ string, guard port.LocalGuard) (*TaskResult, error) {
	if ctx.Value(ownershipTxKey{}) == true {
		return nil, errors.New("half preparation inside ownership root")
	}
	if s.halfErr != nil {
		return nil, s.halfErr
	}
	if s.transfer {
		s.lock.storeID = 4
	}
	err := s.root.WithinTransaction(ctx, func(root context.Context) error {
		if err := guard(root, s.task); err != nil {
			return err
		}
		s.writes++
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &TaskResult{}, nil
}

func TestTransactionOpeningRepeatsOwnershipGuardOnlyAfterHalfPreparation(t *testing.T) {
	for _, mode := range []string{"commit", "ownership_transferred", "half_unknown"} {
		t.Run(mode, func(t *testing.T) {
			task := domain.NewAssessmentTask(11, 1, 1, testee.ID(3), "S1", time.Now())
			lock := &ownershipLockStub{storeID: 3}
			roots := 0
			runner := transaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
				roots++
				return fn(context.WithValue(ctx, ownershipTxKey{}, true))
			})
			command := &guardedOpeningCommand{task: task, lock: lock, root: runner, transfer: mode == "ownership_transferred"}
			if mode == "half_unknown" {
				command.halfErr = eventport.ErrUnresolved
			}
			service := NewOperatorCommandService(command, operatorPlanScopeStub{}, openingLookupRepo{task: task}, OperatorMutationDependencies{Transaction: runner, Ownership: lock})
			ctx := appauthz.WithSnapshot(enrollmentContext(), &appauthz.Snapshot{Permissions: []appauthz.Permission{{Resource: appauthz.EvaluationPlanTaskResource, Action: "open", Mode: appauthz.AuthorizationModeUnconditional}}})
			result, err := service.OpenTask(ctx, 1, task.GetID().String())
			if mode == "commit" {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, 1, command.writes)
			} else {
				require.Error(t, err)
				require.Nil(t, result)
				require.Zero(t, command.writes)
			}
			if mode == "half_unknown" {
				require.Equal(t, 1, roots)
			} else {
				require.Equal(t, 2, roots)
			}
		})
	}
}
