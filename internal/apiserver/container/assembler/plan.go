package assembler

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	planDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	planInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	planEntryInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
	"gorm.io/gorm"
)

// PlanModule Plan 模块（测评计划子域）
// 按照 DDD 限界上下文组织
type PlanModule struct {
	// repository 层
	PlanRepo planDomain.AssessmentPlanRepository
	TaskRepo planDomain.AssessmentTaskRepository

	// handler 层
	Handler *handler.PlanHandler

	// service 层 - 按行为者组织
	LifecycleService      planApp.PlanLifecycleService
	EnrollmentService     planApp.PlanEnrollmentService
	TaskSchedulerService  planApp.TaskSchedulerService
	TaskManagementService planApp.TaskManagementService
	QueryService          planApp.PlanQueryService

	// 事件发布器（由容器统一注入）
	eventPublisher event.EventPublisher
}

// NewPlanModule 创建 Plan 模块
func NewPlanModule() *PlanModule {
	return &PlanModule{}
}

// Initialize 初始化 Plan 模块
// params[0]: *gorm.DB
// params[1]: event.EventPublisher (可选，默认使用 NopEventPublisher)
func (m *PlanModule) Initialize(params ...interface{}) error {
	if len(params) < 1 {
		return errors.WithCode(code.ErrModuleInitializationFailed, "database connection is required")
	}

	mysqlDB, ok := params[0].(*gorm.DB)
	if !ok || mysqlDB == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	// 获取事件发布器（可选参数）
	if len(params) > 1 {
		if ep, ok := params[1].(event.EventPublisher); ok && ep != nil {
			m.eventPublisher = ep
		}
	}
	if m.eventPublisher == nil {
		m.eventPublisher = event.NewNopEventPublisher()
	}

	// 初始化 repository 层
	m.PlanRepo = planInfra.NewPlanRepository(mysqlDB)
	m.TaskRepo = planInfra.NewTaskRepository(mysqlDB)

	// 初始化基础设施层（入口生成器）
	// TODO: 从配置中读取 baseURL，默认使用占位符
	entryGenerator := planEntryInfra.NewEntryGenerator("https://collect.yangshujie.com/entry")

	// 初始化 service 层（依赖 repository，使用模块统一的事件发布器）
	m.LifecycleService = planApp.NewLifecycleService(m.PlanRepo, m.TaskRepo, m.eventPublisher)
	m.EnrollmentService = planApp.NewEnrollmentService(m.PlanRepo, m.TaskRepo, m.eventPublisher)
	m.TaskSchedulerService = planApp.NewTaskSchedulerService(m.TaskRepo, entryGenerator, m.eventPublisher)
	m.TaskManagementService = planApp.NewTaskManagementService(m.TaskRepo, m.eventPublisher)
	m.QueryService = planApp.NewQueryService(m.PlanRepo, m.TaskRepo)

	// 初始化 handler 层
	m.Handler = handler.NewPlanHandler(
		m.LifecycleService,
		m.EnrollmentService,
		m.TaskSchedulerService,
		m.TaskManagementService,
		m.QueryService,
	)

	return nil
}

// Cleanup 清理模块资源
func (m *PlanModule) Cleanup() error {
	return nil
}

// CheckHealth 检查模块健康状态
func (m *PlanModule) CheckHealth() error {
	return nil
}

// ModuleInfo 返回模块信息
func (m *PlanModule) ModuleInfo() ModuleInfo {
	return ModuleInfo{
		Name:        "plan",
		Version:     "1.0.0",
		Description: "测评计划管理模块",
	}
}
