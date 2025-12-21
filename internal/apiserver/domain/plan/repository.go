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

	// FindByScaleCode 查询某个量表的所有计划
	FindByScaleCode(ctx context.Context, scaleCode string) ([]*AssessmentPlan, error)

	// FindActivePlans 查询所有活跃的计划
	FindActivePlans(ctx context.Context) ([]*AssessmentPlan, error)

	// FindByTesteeID 查询某个受试者参与的所有计划
	// 实现方式：通过 Task 反查 Plan，返回去重后的 Plan 列表
	FindByTesteeID(ctx context.Context, testeeID testee.ID) ([]*AssessmentPlan, error)

	// FindList 分页查询计划列表（支持条件筛选）
	FindList(ctx context.Context, orgID int64, scaleCode string, status string, page, pageSize int) ([]*AssessmentPlan, int64, error)

	// Save 保存计划
	Save(ctx context.Context, plan *AssessmentPlan) error
}

// AssessmentTaskRepository 测评任务仓储接口
type AssessmentTaskRepository interface {
	// FindByID 根据 ID 查询任务
	FindByID(ctx context.Context, id AssessmentTaskID) (*AssessmentTask, error)

	// FindByPlanID 查询某个计划的所有任务
	FindByPlanID(ctx context.Context, planID AssessmentPlanID) ([]*AssessmentTask, error)

	// FindByTesteeID 查询某个受试者的所有任务
	FindByTesteeID(ctx context.Context, testeeID testee.ID) ([]*AssessmentTask, error)

	// FindByTesteeIDAndPlanID 查询某个受试者在某个计划下的所有任务
	FindByTesteeIDAndPlanID(ctx context.Context, testeeID testee.ID, planID AssessmentPlanID) ([]*AssessmentTask, error)

	// FindPendingTasks 查询待推送的任务（计划时间 <= before）
	FindPendingTasks(ctx context.Context, before time.Time) ([]*AssessmentTask, error)

	// FindExpiredTasks 查询已过期的任务（状态为 opened，截止时间 <= now）
	FindExpiredTasks(ctx context.Context) ([]*AssessmentTask, error)

	// FindList 分页查询任务列表（支持条件筛选）
	FindList(ctx context.Context, planID *AssessmentPlanID, testeeID *testee.ID, status *TaskStatus, page, pageSize int) ([]*AssessmentTask, int64, error)

	// Save 保存任务
	Save(ctx context.Context, task *AssessmentTask) error

	// SaveBatch 批量保存任务
	SaveBatch(ctx context.Context, tasks []*AssessmentTask) error
}
