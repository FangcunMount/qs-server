package plan

import "context"

// ============= 按行为者组织的应用服务接口（Driving Ports）=============
//
// 设计原则：单一职责原则 (SRP)
// 每个服务只对一个行为者负责，避免不同行为者的需求变更影响同一个类
//
// 行为者识别：
// 1. 计划管理员 (Plan Admin) - 负责计划创建、生命周期管理
// 2. 受试者管理服务 (Enrollment Service) - 负责受试者加入/终止计划
// 3. 任务调度服务 (Task Scheduler) - 负责定时扫描待推送任务
// 4. 任务管理服务 (Task Management) - 负责任务状态管理
// 5. 查询服务 (Query Service) - 为所有行为者提供只读查询

// PlanLifecycleService 计划生命周期服务
// 行为者：计划管理员 (Plan Admin)
// 职责：计划创建、暂停、恢复、取消
// 变更来源：管理员的业务流程需求变化
type PlanLifecycleService interface {
	// CreatePlan 创建测评计划模板
	// 场景：管理员创建新的测评计划模板
	CreatePlan(ctx context.Context, dto CreatePlanDTO) (*PlanResult, error)

	// PausePlan 暂停计划
	// 场景：管理员暂停计划，取消所有未执行的任务
	PausePlan(ctx context.Context, planID string) (*PlanResult, error)

	// ResumePlan 恢复计划
	// 场景：管理员恢复计划，重新生成未完成的任务
	ResumePlan(ctx context.Context, planID string, testeeStartDates map[string]string) (*PlanResult, error)

	// CancelPlan 取消计划
	// 场景：管理员取消计划
	CancelPlan(ctx context.Context, planID string) error
}

// PlanEnrollmentService 受试者加入计划服务
// 行为者：受试者管理服务 (Enrollment Service)
// 职责：受试者加入计划、终止计划
// 变更来源：受试者管理的业务需求变化
type PlanEnrollmentService interface {
	// EnrollTestee 将受试者加入计划
	// 场景：受试者加入测评计划，生成所有任务
	EnrollTestee(ctx context.Context, dto EnrollTesteeDTO) (*EnrollmentResult, error)

	// TerminateEnrollment 终止受试者的计划参与
	// 场景：受试者退出计划，取消所有待处理任务
	TerminateEnrollment(ctx context.Context, planID string, testeeID string) error
}

// TaskSchedulerService 任务调度服务
// 行为者：任务调度服务 (Task Scheduler)
// 职责：定时扫描待推送任务，生成入口并开放
// 变更来源：调度系统的需求变化
type TaskSchedulerService interface {
	// SchedulePendingTasks 调度待推送的任务
	// 场景：定时任务扫描待推送任务，生成入口并开放
	SchedulePendingTasks(ctx context.Context, before string) ([]*TaskResult, error)
}

// TaskManagementService 任务管理服务
// 行为者：任务管理服务 (Task Management)
// 职责：任务状态管理（开放、完成、过期、取消）
// 变更来源：任务管理的业务需求变化
type TaskManagementService interface {
	// OpenTask 开放任务
	// 场景：手动开放任务，生成入口
	OpenTask(ctx context.Context, taskID string, dto OpenTaskDTO) (*TaskResult, error)

	// CompleteTask 完成任务
	// 场景：用户完成测评后，更新任务状态
	CompleteTask(ctx context.Context, taskID string, assessmentID string) (*TaskResult, error)

	// ExpireTask 过期任务
	// 场景：定时任务扫描已过期的任务
	ExpireTask(ctx context.Context, taskID string) (*TaskResult, error)

	// CancelTask 取消任务
	// 场景：手动取消任务
	CancelTask(ctx context.Context, taskID string) error
}

// PlanQueryService 计划查询服务
// 行为者：所有用户（管理员、受试者、调度服务）
// 职责：提供只读查询功能
// 变更来源：查询需求变化
type PlanQueryService interface {
	// GetPlan 根据ID获取计划
	// 场景：查询指定计划的完整信息
	GetPlan(ctx context.Context, planID string) (*PlanResult, error)

	// ListPlans 查询计划列表
	// 场景：分页查询计划列表，支持条件筛选
	ListPlans(ctx context.Context, dto ListPlansDTO) (*PlanListResult, error)

	// GetTask 根据ID获取任务
	// 场景：查询指定任务的完整信息
	GetTask(ctx context.Context, taskID string) (*TaskResult, error)

	// ListTasks 查询任务列表
	// 场景：分页查询任务列表，支持条件筛选
	ListTasks(ctx context.Context, dto ListTasksDTO) (*TaskListResult, error)

	// ListTasksByPlan 查询计划下的所有任务
	// 场景：查看某个计划的所有任务
	ListTasksByPlan(ctx context.Context, planID string) ([]*TaskResult, error)

	// ListTasksByTestee 查询受试者的所有任务
	// 场景：查看某个受试者的所有任务
	ListTasksByTestee(ctx context.Context, testeeID string) ([]*TaskResult, error)

	// ListPlansByTestee 查询受试者参与的所有计划
	// 场景：查看某个受试者参与的所有计划
	ListPlansByTestee(ctx context.Context, testeeID string) ([]*PlanResult, error)

	// ListTasksByTesteeAndPlan 查询受试者在某个计划下的所有任务
	// 场景：查看某个受试者在某个计划下的所有任务
	ListTasksByTesteeAndPlan(ctx context.Context, testeeID string, planID string) ([]*TaskResult, error)
}
