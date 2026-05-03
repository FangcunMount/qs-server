package plan

import (
	"context"
	"strings"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

type repositoryTaskAssessmentResolver struct {
	taskRepo domainplan.AssessmentTaskRepository
}

func NewTaskAssessmentResolver(taskRepo domainplan.AssessmentTaskRepository) TaskAssessmentResolver {
	if taskRepo == nil {
		return nil
	}
	return &repositoryTaskAssessmentResolver{taskRepo: taskRepo}
}

func (r *repositoryTaskAssessmentResolver) ResolveTaskByIDForAssessment(
	ctx context.Context,
	input TaskAssessmentResolveInput,
) *TaskAssessmentContext {
	if r == nil || r.taskRepo == nil || strings.TrimSpace(input.TaskID) == "" {
		return nil
	}

	taskID, err := domainplan.ParseAssessmentTaskID(input.TaskID)
	if err != nil {
		logger.L(ctx).Warnw("计划任务ID格式非法，跳过显式任务识别",
			"task_id", input.TaskID,
			"error", err.Error(),
		)
		return nil
	}

	task, err := r.taskRepo.FindByID(ctx, taskID)
	if err != nil || task == nil {
		logger.L(ctx).Warnw("查询计划任务失败，跳过显式任务识别",
			"task_id", input.TaskID,
			"error", err,
		)
		return nil
	}

	requestOrgID, convErr := safeconv.Uint64ToInt64(input.OrgID)
	if convErr != nil {
		logger.L(ctx).Warnw("请求机构ID超出 int64 范围，跳过显式任务识别",
			"org_id", input.OrgID,
			"error", convErr.Error(),
		)
		return nil
	}
	if task.GetOrgID() != requestOrgID {
		logger.L(ctx).Warnw("计划任务机构不匹配，跳过显式任务识别",
			"task_id", input.TaskID,
			"request_org_id", input.OrgID,
			"task_org_id", task.GetOrgID(),
		)
		return nil
	}
	if task.GetTesteeID().Uint64() != input.TesteeID {
		logger.L(ctx).Warnw("计划任务受试者不匹配，跳过显式任务识别",
			"task_id", input.TaskID,
			"request_testee_id", input.TesteeID,
			"task_testee_id", task.GetTesteeID().Uint64(),
		)
		return nil
	}
	if !task.IsOpened() {
		logger.L(ctx).Warnw("计划任务未处于 opened 状态，跳过显式任务识别",
			"task_id", input.TaskID,
			"task_status", task.GetStatus().String(),
		)
		return nil
	}
	if strings.TrimSpace(input.ScaleCode) == "" {
		logger.L(ctx).Warnw("计划任务已传入，但问卷未关联量表，无法建立计划测评关系",
			"task_id", input.TaskID,
			"questionnaire_code", input.QuestionnaireCode,
		)
		return nil
	}
	if task.GetScaleCode() != input.ScaleCode {
		logger.L(ctx).Warnw("计划任务量表不匹配，跳过显式任务识别",
			"task_id", input.TaskID,
			"task_scale_code", task.GetScaleCode(),
			"request_scale_code", input.ScaleCode,
		)
		return nil
	}

	return taskAssessmentContextFromDomain(task)
}

func (r *repositoryTaskAssessmentResolver) ResolveOpenedTaskForAssessment(
	ctx context.Context,
	input OpenedTaskResolveInput,
) *TaskAssessmentContext {
	if r == nil || r.taskRepo == nil || strings.TrimSpace(input.ScaleCode) == "" || input.TesteeID == 0 {
		return nil
	}

	tasks, err := r.taskRepo.FindByTesteeID(ctx, testee.ID(meta.FromUint64(input.TesteeID)))
	if err != nil {
		logger.L(ctx).Warnw("查询受试者计划任务失败",
			"testee_id", input.TesteeID,
			"scale_code", input.ScaleCode,
			"error", err.Error(),
		)
		return nil
	}

	targetOrgID, convErr := safeconv.Uint64ToInt64(input.OrgID)
	if convErr != nil {
		logger.L(ctx).Warnw("机构ID超出 int64 范围，跳过自动 plan 识别",
			"org_id", input.OrgID,
			"error", convErr.Error(),
		)
		return nil
	}

	var matched *domainplan.AssessmentTask
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if task.GetOrgID() != targetOrgID || task.GetScaleCode() != input.ScaleCode || !task.IsOpened() {
			continue
		}
		if matched != nil {
			logger.L(ctx).Warnw("存在多个候选 opened task，跳过自动 plan 识别",
				"testee_id", input.TesteeID,
				"org_id", input.OrgID,
				"scale_code", input.ScaleCode,
				"first_task_id", matched.GetID().String(),
				"second_task_id", task.GetID().String(),
			)
			return nil
		}
		matched = task
	}

	return taskAssessmentContextFromDomain(matched)
}

func taskAssessmentContextFromDomain(task *domainplan.AssessmentTask) *TaskAssessmentContext {
	if task == nil {
		return nil
	}
	return &TaskAssessmentContext{
		TaskID:    task.GetID().String(),
		PlanID:    task.GetPlanID().String(),
		Completed: task.IsCompleted(),
	}
}
