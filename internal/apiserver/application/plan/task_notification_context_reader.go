package plan

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
)

type repositoryTaskNotificationContextReader struct {
	taskRepo domainplan.AssessmentTaskRepository
	planRepo domainplan.AssessmentPlanRepository
}

func NewTaskNotificationContextReader(
	taskRepo domainplan.AssessmentTaskRepository,
	planRepo domainplan.AssessmentPlanRepository,
) TaskNotificationContextReader {
	if taskRepo == nil {
		return nil
	}
	return &repositoryTaskNotificationContextReader{
		taskRepo: taskRepo,
		planRepo: planRepo,
	}
}

func (r *repositoryTaskNotificationContextReader) GetTaskNotificationContext(
	ctx context.Context,
	taskIDRaw string,
) (*TaskNotificationContext, error) {
	if r == nil || r.taskRepo == nil || taskIDRaw == "" {
		return nil, nil
	}

	taskID, err := domainplan.ParseAssessmentTaskID(taskIDRaw)
	if err != nil {
		return nil, err
	}
	task, err := r.taskRepo.FindByID(ctx, taskID)
	if err != nil || task == nil {
		return nil, err
	}

	result := &TaskNotificationContext{
		TaskID:    task.GetID().String(),
		PlanID:    task.GetPlanID().String(),
		ScaleCode: task.GetScaleCode(),
		PlannedAt: task.GetPlannedAt(),
		Seq:       task.GetSeq(),
	}

	if r.planRepo != nil {
		parentPlan, err := r.planRepo.FindByID(ctx, task.GetPlanID())
		if err != nil {
			logger.L(ctx).Warnw("failed to load plan for mini program notification",
				"action", "resolve_task_opened_template_data",
				"task_id", task.GetID().String(),
				"plan_id", task.GetPlanID().String(),
				"error", err.Error(),
			)
		} else if parentPlan != nil {
			result.TotalTimes = parentPlan.GetTotalTimes()
		}
	}

	tasks, err := r.taskRepo.FindByTesteeID(ctx, task.GetTesteeID())
	if err != nil {
		logger.L(ctx).Warnw("failed to count unfinished tasks for mini program notification",
			"action", "resolve_task_opened_template_data",
			"task_id", task.GetID().String(),
			"testee_id", task.GetTesteeID().String(),
			"error", err.Error(),
		)
		return result, nil
	}

	result.UnfinishedSameDayTaskCount = countUnfinishedSameDayTasks(tasks, task.GetPlannedAt())
	return result, nil
}

func countUnfinishedSameDayTasks(tasks []*domainplan.AssessmentTask, plannedAt time.Time) int {
	count := 0
	for _, item := range tasks {
		if item == nil || item.GetStatus().IsTerminal() {
			continue
		}
		if sameLocalDate(item.GetPlannedAt(), plannedAt) {
			count++
		}
	}
	return count
}

func sameLocalDate(left, right time.Time) bool {
	if left.IsZero() || right.IsZero() {
		return false
	}
	left = left.Local()
	right = right.Local()
	return left.Year() == right.Year() && left.Month() == right.Month() && left.Day() == right.Day()
}
