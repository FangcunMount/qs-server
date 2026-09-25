package plan

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type repositoryTaskReminderStateReader struct {
	tasks domainplan.AssessmentTaskRepository
}

func NewTaskReminderStateReader(tasks domainplan.AssessmentTaskRepository) TaskReminderStateReader {
	if tasks == nil {
		return nil
	}
	return &repositoryTaskReminderStateReader{tasks: tasks}
}

func (r *repositoryTaskReminderStateReader) GetTaskReminderState(
	ctx context.Context, orgID int64, taskIDRaw string,
) (*TaskReminderState, error) {
	if r == nil || r.tasks == nil {
		return nil, fmt.Errorf("task reminder state reader is not configured")
	}
	if orgID <= 0 || strings.TrimSpace(taskIDRaw) == "" {
		return nil, fmt.Errorf("task reminder state requires organization and task ID")
	}
	taskID, err := domainplan.ParseAssessmentTaskID(taskIDRaw)
	if err != nil {
		return nil, err
	}
	task, err := r.tasks.FindByID(ctx, taskID)
	if err != nil || task == nil {
		return nil, err
	}
	if task.GetOrgID() != orgID {
		return nil, errors.WithCode(code.ErrPageNotFound, "task not found")
	}
	return &TaskReminderState{
		TaskID:           task.GetID().String(),
		PlanID:           task.GetPlanID().String(),
		OrgID:            task.GetOrgID(),
		TesteeID:         task.GetTesteeID().String(),
		Status:           task.GetStatus(),
		ScheduleRevision: task.GetScheduleRevision(),
		OpenAt:           copyReminderTime(task.GetOpenAt()),
		ExpireAt:         copyReminderTime(task.GetExpireAt()),
		EntryURL:         task.GetEntryURL(),
	}, nil
}

func copyReminderTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}
