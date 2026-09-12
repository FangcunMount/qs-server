package plan

import (
	"context"
	"errors"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"testing"
	"time"
)

type operatorPlanScopeStub struct{ all bool }

func (s operatorPlanScopeStub) ResolveStoreRange(context.Context, int64, int64, string, string) (appauthz.StoreRange, error) {
	return appauthz.StoreRange{AllStores: s.all, StoreIDs: []uint64{3}}, nil
}
func (operatorPlanScopeStub) ValidateTesteeStoreAccess(context.Context, int64, int64, uint64, string, string) error {
	return nil
}

type operatorPlanCommandStub struct {
	PlanCommandService
	creates int
}

func (s *operatorPlanCommandStub) CreatePlan(context.Context, CreatePlanDTO) (*PlanResult, error) {
	s.creates++
	return &PlanResult{}, nil
}

func TestOperatorPlanCommandsNeverInferInternalCallerFromMissingIdentity(t *testing.T) {
	// Every delegate method except CreatePlan would panic if reached.
	service := NewOperatorCommandService(&operatorPlanCommandStub{}, operatorPlanScopeStub{all: true}, nil)
	ctx := context.Background()
	calls := []func() error{
		func() error { _, err := service.CreatePlan(ctx, CreatePlanDTO{OrgID: 1}); return err },
		func() error { _, err := service.PausePlan(ctx, 1, "11"); return err },
		func() error { _, err := service.ResumePlan(ctx, 1, "11", nil); return err },
		func() error { _, err := service.FinishPlan(ctx, 1, "11"); return err },
		func() error { _, err := service.CancelPlan(ctx, 1, "11"); return err },
		func() error {
			_, err := service.EnrollTestee(ctx, EnrollTesteeDTO{OrgID: 1, TesteeID: "3"})
			return err
		},
		func() error { _, err := service.TerminateEnrollment(ctx, 1, "11", "3"); return err },
		func() error { _, err := service.SchedulePendingTasks(ctx, 1, ""); return err },
		func() error { _, err := service.OpenTask(ctx, 1, "11"); return err },
		func() error { _, err := service.CompleteTask(ctx, 1, "11", "22"); return err },
		func() error { _, err := service.ExpireTask(ctx, 1, "11"); return err },
		func() error { _, err := service.CancelTask(ctx, 1, "11"); return err },
	}
	for i, call := range calls {
		if err := call(); err == nil {
			t.Fatalf("method %d accepted missing identity", i)
		}
	}
}
func TestSharedPlanCreationRequiresCompanyWideActionRange(t *testing.T) {
	for _, all := range []bool{false, true} {
		command := &operatorPlanCommandStub{}
		service := NewOperatorCommandService(command, operatorPlanScopeStub{all: all}, nil)
		ctx := appauthz.WithSnapshot(enrollmentContext(), &appauthz.Snapshot{Permissions: []appauthz.Permission{{Resource: appauthz.EvaluationPlanResource, Action: "create", Mode: appauthz.AuthorizationModeUnconditional}}})
		result, err := service.CreatePlan(ctx, CreatePlanDTO{OrgID: 1})
		if all {
			if err != nil || result == nil || command.creates != 1 {
				t.Fatalf("allowed: %v", err)
			}
		} else if err == nil || result != nil || command.creates != 0 {
			t.Fatal("store-only scope modified shared plan")
		}
	}
}

type ownershipTxKey struct{}
type ownershipLockStub struct {
	storeID uint64
	locked  bool
}

func (s *ownershipLockStub) LockTesteeStore(ctx context.Context, orgID int64, id uint64) (*uint64, error) {
	if ctx.Value(ownershipTxKey{}) != true || orgID != 1 || id != 3 {
		return nil, errors.New("missing ownership transaction")
	}
	s.locked = true
	return &s.storeID, nil
}
func TestEnrollmentMutationUsesLockedOwnershipAndSameTransaction(t *testing.T) {
	for _, storeID := range []uint64{3, 4} {
		lock := &ownershipLockStub{storeID: storeID}
		runner := transaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(context.WithValue(ctx, ownershipTxKey{}, true))
		})
		service := NewOperatorCommandService(&operatorPlanCommandStub{}, operatorPlanScopeStub{}, nil, OperatorMutationDependencies{Transaction: runner, Ownership: lock}).(*operatorCommands)
		ctx := appauthz.WithSnapshot(enrollmentContext(), &appauthz.Snapshot{Permissions: []appauthz.Permission{{Resource: appauthz.EvaluationPlanResource, Action: "enroll", Mode: appauthz.AuthorizationModeUnconditional}}})
		writes := 0
		err := service.withTesteeMutation(ctx, 1, "3", "enroll", func(tx context.Context) error {
			if !lock.locked || tx.Value(ownershipTxKey{}) != true {
				t.Fatal("write escaped ownership transaction")
			}
			writes++
			return nil
		})
		if storeID == 3 {
			if err != nil || writes != 1 {
				t.Fatalf("valid ownership: %v", err)
			}
		} else if err == nil || writes != 0 {
			t.Fatal("transferred testee was mutated by previous store")
		}
	}
}

type ownershipTaskRepo struct {
	domainplan.AssessmentTaskRepository
}

func (ownershipTaskRepo) FindByID(context.Context, domainplan.AssessmentTaskID) (*domainplan.AssessmentTask, error) {
	return domainplan.NewAssessmentTask(11, 1, 1, testee.ID(3), "S1", time.Now()), nil
}

type lockedTaskCommand struct {
	PlanCommandService
	calls int
}

func (s *lockedTaskCommand) record(ctx context.Context) error {
	if ctx.Value(ownershipTxKey{}) != true {
		return errors.New("write escaped transaction")
	}
	s.calls++
	return nil
}
func (s *lockedTaskCommand) OpenTask(ctx context.Context, _ int64, _ string) (*TaskResult, error) {
	return &TaskResult{}, s.record(ctx)
}
func (s *lockedTaskCommand) CompleteTask(ctx context.Context, _ int64, _, _ string) (*TaskResult, error) {
	return &TaskResult{}, s.record(ctx)
}
func (s *lockedTaskCommand) ExpireTask(ctx context.Context, _ int64, _ string) (*TaskResult, error) {
	return &TaskResult{}, s.record(ctx)
}
func (s *lockedTaskCommand) CancelTask(ctx context.Context, _ int64, _ string) (*TaskMutationResult, error) {
	return &TaskMutationResult{}, s.record(ctx)
}
func TestEveryTaskMutationLocksCurrentOwnership(t *testing.T) {
	for _, action := range []string{"open", "complete", "expire", "cancel"} {
		for _, storeID := range []uint64{3, 4} {
			lock := &ownershipLockStub{storeID: storeID}
			runner := transaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
				return fn(context.WithValue(ctx, ownershipTxKey{}, true))
			})
			command := &lockedTaskCommand{}
			service := NewOperatorCommandService(command, operatorPlanScopeStub{}, ownershipTaskRepo{}, OperatorMutationDependencies{Transaction: runner, Ownership: lock})
			ctx := appauthz.WithSnapshot(enrollmentContext(), &appauthz.Snapshot{Permissions: []appauthz.Permission{{Resource: appauthz.EvaluationPlanTaskResource, Action: action, Mode: appauthz.AuthorizationModeUnconditional}}})
			var err error
			switch action {
			case "open":
				_, err = service.OpenTask(ctx, 1, "22")
			case "complete":
				_, err = service.CompleteTask(ctx, 1, "22", "33")
			case "expire":
				_, err = service.ExpireTask(ctx, 1, "22")
			case "cancel":
				_, err = service.CancelTask(ctx, 1, "22")
			}
			if !lock.locked {
				t.Fatalf("%s did not lock ownership", action)
			}
			if storeID == 3 {
				if err != nil || command.calls != 1 {
					t.Fatalf("%s allowed: %v", action, err)
				}
			} else if err == nil || command.calls != 0 {
				t.Fatalf("%s mutated transferred testee", action)
			}
		}
	}
}
