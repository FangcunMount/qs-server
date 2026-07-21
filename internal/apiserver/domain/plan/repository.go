package plan

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

// AssessmentPlanRepository 测评计划仓储接口
type AssessmentPlanRepository interface {
	// FindByID 根据 ID 查询计划
	FindByID(ctx context.Context, id AssessmentPlanID) (*AssessmentPlan, error)

	// Save 保存计划
	Save(ctx context.Context, plan *AssessmentPlan) error
}

// AssessmentTaskRepository 测评任务仓储接口
type AssessmentTaskRepository interface {
	// FindByID 根据 ID 查询任务
	FindByID(ctx context.Context, id AssessmentTaskID) (*AssessmentTask, error)

	// FindByPlanID 查询某个计划的所有任务
	FindByPlanID(ctx context.Context, planID AssessmentPlanID) ([]*AssessmentTask, error)

	// FindByPlanIDAndTesteeIDs 查询某个计划下指定受试者集合的任务。
	FindByPlanIDAndTesteeIDs(ctx context.Context, planID AssessmentPlanID, testeeIDs []testee.ID) ([]*AssessmentTask, error)

	// FindByTesteeID 查询某个受试者的所有任务
	FindByTesteeID(ctx context.Context, testeeID testee.ID) ([]*AssessmentTask, error)

	// FindByTesteeIDAndPlanID 查询某个受试者在某个计划下的所有任务
	FindByTesteeIDAndPlanID(ctx context.Context, testeeID testee.ID, planID AssessmentPlanID) ([]*AssessmentTask, error)

	// FindPendingTasks 查询待推送的任务（计划时间 <= before）
	FindPendingTasks(ctx context.Context, orgID int64, before time.Time) ([]*AssessmentTask, error)

	// FindExpiredTasks 查询已过期的任务（状态为 opened，截止时间 <= now）
	FindExpiredTasks(ctx context.Context) ([]*AssessmentTask, error)

	// Save 保存任务
	Save(ctx context.Context, task *AssessmentTask) error

	// SaveBatch 批量保存任务
	SaveBatch(ctx context.Context, tasks []*AssessmentTask) error
}

type EnrollmentTaskRepository interface {
	FindByEnrollmentID(ctx context.Context, enrollmentID PlanEnrollmentID) ([]*AssessmentTask, error)
}

// EnrollmentRepository 持久化患者参与 Plan 的轮次事实。
type EnrollmentRepository interface {
	FindByID(ctx context.Context, id PlanEnrollmentID) (*Enrollment, error)
	FindActive(ctx context.Context, orgID int64, planID AssessmentPlanID, testeeID testee.ID) (*Enrollment, error)
	FindLatest(ctx context.Context, orgID int64, planID AssessmentPlanID, testeeID testee.ID) (*Enrollment, error)
	Save(ctx context.Context, enrollment *Enrollment) error
	CloseIfAllTasksTerminal(ctx context.Context, id PlanEnrollmentID, closedAt time.Time) (bool, error)
}

// PlanEnrollmentLifecycleRepository performs set-based Enrollment transitions
// required by a Plan lifecycle transaction.
type PlanEnrollmentLifecycleRepository interface {
	TerminateActiveByPlan(ctx context.Context, orgID int64, planID AssessmentPlanID, reason string, at time.Time) (int64, error)
	CloseActiveByPlanIfAllTasksTerminal(ctx context.Context, orgID int64, planID AssessmentPlanID, at time.Time) (int64, error)
}
