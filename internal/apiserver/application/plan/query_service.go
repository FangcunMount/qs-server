package plan

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// queryService 计划查询服务实现
// 行为者：所有用户
type queryService struct {
	planRepo domainPlan.AssessmentPlanRepository
	taskRepo domainPlan.AssessmentTaskRepository
}

// NewQueryService 创建计划查询服务
func NewQueryService(
	planRepo domainPlan.AssessmentPlanRepository,
	taskRepo domainPlan.AssessmentTaskRepository,
) PlanQueryService {
	return &queryService{
		planRepo: planRepo,
		taskRepo: taskRepo,
	}
}

// GetPlan 根据ID获取计划
func (s *queryService) GetPlan(ctx context.Context, planID string) (*PlanResult, error) {
	// 1. 转换参数
	id, err := toPlanID(planID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID: %v", err)
	}

	// 2. 查询计划
	p, err := s.planRepo.FindByID(ctx, id)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrPageNotFound, "计划不存在")
	}

	return toPlanResult(p), nil
}

// ListPlans 查询计划列表
func (s *queryService) ListPlans(ctx context.Context, dto ListPlansDTO) (*PlanListResult, error) {
	// 1. 验证分页参数
	if dto.Page <= 0 {
		dto.Page = 1
	}
	if dto.PageSize <= 0 {
		dto.PageSize = 10
	}
	if dto.PageSize > 100 {
		dto.PageSize = 100 // 限制最大每页数量
	}

	// 2. 查询计划列表
	plans, total, err := s.planRepo.FindList(ctx, dto.OrgID, dto.ScaleCode, dto.Status, dto.Page, dto.PageSize)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询计划列表失败")
	}

	// 3. 转换为结果
	items := make([]*PlanResult, 0, len(plans))
	for _, plan := range plans {
		items = append(items, toPlanResult(plan))
	}

	return &PlanListResult{
		Items:    items,
		Total:    total,
		Page:     dto.Page,
		PageSize: dto.PageSize,
	}, nil
}

// GetTask 根据ID获取任务
func (s *queryService) GetTask(ctx context.Context, taskID string) (*TaskResult, error) {
	// 1. 转换参数
	id, err := toTaskID(taskID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的任务ID: %v", err)
	}

	// 2. 查询任务
	task, err := s.taskRepo.FindByID(ctx, id)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrPageNotFound, "任务不存在")
	}

	return toTaskResult(task), nil
}

// ListTasks 查询任务列表
func (s *queryService) ListTasks(ctx context.Context, dto ListTasksDTO) (*TaskListResult, error) {
	// 1. 验证分页参数
	if dto.Page <= 0 {
		dto.Page = 1
	}
	if dto.PageSize <= 0 {
		dto.PageSize = 10
	}
	if dto.PageSize > 100 {
		dto.PageSize = 100 // 限制最大每页数量
	}

	// 2. 转换查询条件
	var planID *domainPlan.AssessmentPlanID
	if dto.PlanID != "" {
		id, err := toPlanID(dto.PlanID)
		if err != nil {
			return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID: %v", err)
		}
		planID = &id
	}

	var testeeID *testee.ID
	if dto.TesteeID != "" {
		id, err := toTesteeID(dto.TesteeID)
		if err != nil {
			return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的受试者ID: %v", err)
		}
		testeeID = &id
	}

	var status *domainPlan.TaskStatus
	if dto.Status != "" {
		statusVal := domainPlan.TaskStatus(dto.Status)
		if !statusVal.IsValid() {
			return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的任务状态: %s", dto.Status)
		}
		status = &statusVal
	}

	// 3. 查询任务列表
	tasks, total, err := s.taskRepo.FindList(ctx, planID, testeeID, status, dto.Page, dto.PageSize)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询任务列表失败")
	}

	// 4. 转换为结果
	items := make([]*TaskResult, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, toTaskResult(task))
	}

	return &TaskListResult{
		Items:    items,
		Total:    total,
		Page:     dto.Page,
		PageSize: dto.PageSize,
	}, nil
}

// ListTasksByPlan 查询计划下的所有任务
func (s *queryService) ListTasksByPlan(ctx context.Context, planID string) ([]*TaskResult, error) {
	// 1. 转换参数
	id, err := toPlanID(planID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID: %v", err)
	}

	// 2. 查询任务
	tasks, err := s.taskRepo.FindByPlanID(ctx, id)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询任务失败")
	}

	return toTaskResults(tasks), nil
}

// ListTasksByTestee 查询受试者的所有任务
func (s *queryService) ListTasksByTestee(ctx context.Context, testeeID string) ([]*TaskResult, error) {
	// 1. 转换参数
	testeeIDDomain, err := toTesteeID(testeeID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的受试者ID: %v", err)
	}

	// 2. 查询任务
	tasks, err := s.taskRepo.FindByTesteeID(ctx, testeeIDDomain)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询任务失败")
	}

	return toTaskResults(tasks), nil
}

// ListPlansByTestee 查询受试者参与的所有计划
func (s *queryService) ListPlansByTestee(ctx context.Context, testeeID string) ([]*PlanResult, error) {
	// 1. 转换参数
	testeeIDDomain, err := toTesteeID(testeeID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的受试者ID: %v", err)
	}

	// 2. 查询计划
	plans, err := s.planRepo.FindByTesteeID(ctx, testeeIDDomain)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询计划失败")
	}

	// 3. 转换为结果
	results := make([]*PlanResult, 0, len(plans))
	for _, plan := range plans {
		results = append(results, toPlanResult(plan))
	}

	return results, nil
}

// ListTasksByTesteeAndPlan 查询受试者在某个计划下的所有任务
func (s *queryService) ListTasksByTesteeAndPlan(ctx context.Context, testeeID string, planID string) ([]*TaskResult, error) {
	// 1. 转换参数
	testeeIDDomain, err := toTesteeID(testeeID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的受试者ID: %v", err)
	}

	planIDDomain, err := toPlanID(planID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID: %v", err)
	}

	// 2. 查询任务
	tasks, err := s.taskRepo.FindByTesteeIDAndPlanID(ctx, testeeIDDomain, planIDDomain)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询任务失败")
	}

	return toTaskResults(tasks), nil
}
