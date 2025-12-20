package plan

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// queryService 计划查询服务实现
// 行为者：所有用户
type queryService struct {
	planRepo plan.AssessmentPlanRepository
	taskRepo plan.AssessmentTaskRepository
}

// NewQueryService 创建计划查询服务
func NewQueryService(
	planRepo plan.AssessmentPlanRepository,
	taskRepo plan.AssessmentTaskRepository,
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
	// TODO: 实现分页查询逻辑
	// 目前先返回空列表，等待仓储层实现分页查询方法
	return &PlanListResult{
		Items:    []*PlanResult{},
		Total:    0,
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
	// TODO: 实现分页查询逻辑
	// 目前先返回空列表，等待仓储层实现分页查询方法
	return &TaskListResult{
		Items:    []*TaskResult{},
		Total:    0,
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
