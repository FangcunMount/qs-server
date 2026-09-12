package plan

import (
	"context"
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"math"
)

// OperatorCommandScope is implemented by Actor; the plan application owns the
// action selection while Actor checks active membership and current ownership.
type OperatorCommandScope interface {
	EnrollmentScopeChecker
	ResolveStoreRange(context.Context, int64, int64, string, string) (appauthz.StoreRange, error)
}

type OperatorOwnershipLocker interface {
	LockTesteeStore(context.Context, int64, uint64) (*uint64, error)
}
type OperatorMutationDependencies struct {
	Transaction transaction.Runner
	Ownership   OperatorOwnershipLocker
}

type operatorCommands struct {
	mutation OperatorMutationDependencies
	commands PlanCommandService
	access   OperatorCommandScope
	tasks    domainplan.AssessmentTaskRepository
}

// NewOperatorCommandService constructs the backstage entry explicitly. Internal
// scheduling and assessment intake retain their separately composed commands.
func NewOperatorCommandService(commands PlanCommandService, access OperatorCommandScope, tasks domainplan.AssessmentTaskRepository, dependencies ...OperatorMutationDependencies) PlanCommandService {
	var mutation OperatorMutationDependencies
	if len(dependencies) > 0 {
		mutation = dependencies[0]
	}
	return &operatorCommands{commands: commands, access: access, tasks: tasks, mutation: mutation}
}

func (s *operatorCommands) authorize(ctx context.Context, orgID int64, resource, action string) (int64, error) {
	user := actorctx.GrantingUserID(ctx)
	if s.commands == nil || s.access == nil || orgID <= 0 || actorctx.OperatorOrgID(ctx) != orgID || user == 0 || user > math.MaxInt64 {
		return 0, errors.WithCode(code.ErrPermissionDenied, "trusted operator command scope required")
	}
	if err := appauthz.RequirePermission(ctx, resource, action); err != nil {
		return 0, err
	}
	return int64(user), nil
}
func (s *operatorCommands) headquarters(ctx context.Context, orgID int64, resource, action string) error {
	user, err := s.authorize(ctx, orgID, resource, action)
	if err != nil {
		return err
	}
	scope, err := s.access.ResolveStoreRange(ctx, orgID, user, resource, action)
	if err != nil {
		return err
	}
	if !scope.AllStores {
		return errors.WithCode(code.ErrPermissionDenied, "company-wide permission required for shared plan mutation")
	}
	return nil
}
func (s *operatorCommands) CreatePlan(ctx context.Context, dto CreatePlanDTO) (*PlanResult, error) {
	if err := s.headquarters(ctx, dto.OrgID, appauthz.EvaluationPlanResource, "create"); err != nil {
		return nil, err
	}
	return s.commands.CreatePlan(ctx, dto)
}

func (s *operatorCommands) PausePlan(ctx context.Context, orgID int64, planID string) (*PlanResult, error) {
	if err := s.headquarters(ctx, orgID, appauthz.EvaluationPlanResource, "pause"); err != nil {
		return nil, err
	}
	return s.commands.PausePlan(ctx, orgID, planID)
}

func (s *operatorCommands) ResumePlan(ctx context.Context, orgID int64, planID string, testeeStartDates map[string]string) (*PlanResult, error) {
	if err := s.headquarters(ctx, orgID, appauthz.EvaluationPlanResource, "resume"); err != nil {
		return nil, err
	}
	return s.commands.ResumePlan(ctx, orgID, planID, testeeStartDates)
}

func (s *operatorCommands) FinishPlan(ctx context.Context, orgID int64, planID string) (*PlanResult, error) {
	if err := s.headquarters(ctx, orgID, appauthz.EvaluationPlanResource, "update"); err != nil {
		return nil, err
	}
	return s.commands.FinishPlan(ctx, orgID, planID)
}

func (s *operatorCommands) CancelPlan(ctx context.Context, orgID int64, planID string) (*PlanMutationResult, error) {
	if err := s.headquarters(ctx, orgID, appauthz.EvaluationPlanResource, "cancel"); err != nil {
		return nil, err
	}
	return s.commands.CancelPlan(ctx, orgID, planID)
}

func (s *operatorCommands) EnrollTestee(ctx context.Context, dto EnrollTesteeDTO) (*EnrollmentResult, error) {
	var result *EnrollmentResult
	err := s.withTesteeMutation(ctx, dto.OrgID, dto.TesteeID, "enroll", func(tx context.Context) error {
		var err error
		result, err = s.commands.EnrollTestee(tx, dto)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (s *operatorCommands) TerminateEnrollment(ctx context.Context, orgID int64, planID, testeeID string) (*EnrollmentTerminationResult, error) {
	var result *EnrollmentTerminationResult
	err := s.withTesteeMutation(ctx, orgID, testeeID, "terminate", func(tx context.Context) error {
		var err error
		result, err = s.commands.TerminateEnrollment(tx, orgID, planID, testeeID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (s *operatorCommands) withTesteeMutation(ctx context.Context, orgID int64, rawID, action string, write func(context.Context) error) error {
	return s.withOwnershipMutation(ctx, orgID, rawID, appauthz.EvaluationPlanResource, action, write)
}
func (s *operatorCommands) withOwnershipMutation(ctx context.Context, orgID int64, rawID, resource, action string, write func(context.Context) error) error {
	user, err := s.authorize(ctx, orgID, resource, action)
	if err != nil {
		return err
	}
	if s.mutation.Transaction == nil || s.mutation.Ownership == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "ownership transaction required")
	}
	id, err := toTesteeID(rawID)
	if err != nil {
		return errors.WithCode(code.ErrInvalidArgument, "invalid testee ID")
	}
	stores, err := s.access.ResolveStoreRange(ctx, orgID, user, resource, action)
	if err != nil {
		return err
	}
	return s.mutation.Transaction.WithinTransaction(ctx, func(tx context.Context) error {
		storeID, err := s.mutation.Ownership.LockTesteeStore(tx, orgID, id.Uint64())
		if err != nil {
			return err
		}
		if !stores.Contains(storeID) {
			return errors.WithCode(code.ErrPermissionDenied, "testee outside current store range")
		}
		return write(tx)
	})
}

func (s *operatorCommands) SchedulePendingTasks(ctx context.Context, orgID int64, before string) (*TaskScheduleResult, error) {
	if err := s.headquarters(ctx, orgID, appauthz.EvaluationPlanTaskResource, "schedule"); err != nil {
		return nil, err
	}
	return s.commands.SchedulePendingTasks(ctx, orgID, before)
}

func (s *operatorCommands) OpenTask(ctx context.Context, orgID int64, taskID string) (*TaskResult, error) {
	var result *TaskResult
	err := s.withTaskMutation(ctx, orgID, taskID, "open", func(ctx context.Context) error {
		var err error
		result, err = s.commands.OpenTask(ctx, orgID, taskID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *operatorCommands) CompleteTask(ctx context.Context, orgID int64, taskID string, assessmentID string) (*TaskResult, error) {
	var result *TaskResult
	err := s.withTaskMutation(ctx, orgID, taskID, "complete", func(ctx context.Context) error {
		var err error
		result, err = s.commands.CompleteTask(ctx, orgID, taskID, assessmentID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *operatorCommands) ExpireTask(ctx context.Context, orgID int64, taskID string) (*TaskResult, error) {
	var result *TaskResult
	err := s.withTaskMutation(ctx, orgID, taskID, "expire", func(ctx context.Context) error {
		var err error
		result, err = s.commands.ExpireTask(ctx, orgID, taskID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *operatorCommands) CancelTask(ctx context.Context, orgID int64, taskID string) (*TaskMutationResult, error) {
	var result *TaskMutationResult
	err := s.withTaskMutation(ctx, orgID, taskID, "cancel", func(ctx context.Context) error {
		var err error
		result, err = s.commands.CancelTask(ctx, orgID, taskID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *operatorCommands) withTaskMutation(ctx context.Context, orgID int64, taskID, action string, write func(context.Context) error) error {
	if _, err := s.authorize(ctx, orgID, appauthz.EvaluationPlanTaskResource, action); err != nil {
		return err
	}
	if s.tasks == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "task repository required")
	}
	// Task TesteeID is immutable. Resolve it before beginning the ownership
	// transaction so its first authorization read is the current locking read.
	task, err := loadTaskInOrg(ctx, s.tasks, orgID, taskID, action)
	if err != nil {
		return err
	}
	return s.withOwnershipMutation(ctx, orgID, task.GetTesteeID().String(), appauthz.EvaluationPlanTaskResource, action, write)
}
