package assembler

import (
	"strings"

	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	planCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	planInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	planEntryInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/plan"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// PlanModule Plan 模块（测评计划子域）
// 按照 DDD 限界上下文组织
type PlanModule struct {
	// service 层
	CommandService                planApp.PlanCommandService
	QueryService                  planApp.PlanQueryService
	TaskAssessmentResolver        planApp.TaskAssessmentResolver
	TaskNotificationContextReader planApp.TaskNotificationContextReader

	// 事件发布器（由容器统一注入）
	eventPublisher      event.EventPublisher
	testeeAccessService actorAccessApp.TesteeAccessService
}

// PlanModuleDeps 定义 Plan 模块的显式构造依赖。
type PlanModuleDeps struct {
	MySQLDB        *gorm.DB
	EventPublisher event.EventPublisher
	ScaleRepo      scale.Repository
	RedisClient    redis.UniversalClient
	CacheBuilder   *keyspace.Builder
	PlanPolicy     cachepolicy.CachePolicy
	EntryBaseURL   string
	Observer       *observability.ComponentObserver
	MySQLLimiter   backpressure.Acquirer
	TesteeAccess   actorAccessApp.TesteeAccessService
}

// NewPlanModule 创建 Plan 模块。
func NewPlanModule(deps PlanModuleDeps) (*PlanModule, error) {
	normalized, err := normalizePlanModuleDeps(deps)
	if err != nil {
		return nil, err
	}

	module := &PlanModule{}
	module.eventPublisher = normalized.EventPublisher
	module.testeeAccessService = normalized.TesteeAccess

	// 初始化 repository 层
	// 初始化基础 Repository
	mysqlOptions := mysql.BaseRepositoryOptions{Limiter: normalized.MySQLLimiter}
	basePlanRepo := planInfra.NewPlanRepository(normalized.MySQLDB, mysqlOptions)

	// 如果提供了 Redis 客户端，使用缓存装饰器
	planRepo := basePlanRepo
	if normalized.RedisClient != nil {
		planRepo = planCache.NewCachedPlanRepositoryWithBuilderPolicyAndObserver(basePlanRepo, normalized.RedisClient, normalized.CacheBuilder, normalized.PlanPolicy, normalized.Observer)
	}

	taskRepo := planInfra.NewTaskRepository(normalized.MySQLDB, mysqlOptions)

	// 初始化基础设施层（入口生成器）
	entryGenerator := planEntryInfra.NewEntryGenerator(normalized.EntryBaseURL)
	scaleCatalog := planApp.NewRepositoryScaleCatalog(normalized.ScaleRepo)
	planReadModel := planInfra.NewReadModel(normalized.MySQLDB)

	// 初始化 service 层（依赖 repository，使用模块统一的事件发布器）
	lifecycleService := planApp.NewLifecycleServiceWithScaleCatalog(planRepo, taskRepo, scaleCatalog, module.eventPublisher)
	enrollmentService := planApp.NewEnrollmentService(planRepo, taskRepo, module.eventPublisher)
	taskSchedulerService := planApp.NewTaskSchedulerService(taskRepo, planRepo, entryGenerator, module.eventPublisher)
	taskManagementService := planApp.NewTaskManagementService(taskRepo, module.eventPublisher)
	module.CommandService = planApp.NewCommandService(
		lifecycleService,
		enrollmentService,
		taskSchedulerService,
		taskManagementService,
		planRepo,
		taskRepo,
	)
	module.QueryService = planApp.NewQueryServiceWithReadModel(planReadModel, planReadModel, scaleCatalog)
	module.TaskAssessmentResolver = planApp.NewTaskAssessmentResolver(taskRepo)
	module.TaskNotificationContextReader = planApp.NewTaskNotificationContextReader(taskRepo, planRepo)

	return module, nil
}

func normalizePlanModuleDeps(deps PlanModuleDeps) (PlanModuleDeps, error) {
	if deps.MySQLDB == nil {
		return PlanModuleDeps{}, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	if deps.EventPublisher == nil {
		deps.EventPublisher = event.NewNopEventPublisher()
	}
	if strings.TrimSpace(deps.EntryBaseURL) == "" {
		deps.EntryBaseURL = apiserveroptions.DefaultPlanEntryBaseURL
	} else {
		deps.EntryBaseURL = strings.TrimSpace(deps.EntryBaseURL)
	}
	return deps, nil
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
