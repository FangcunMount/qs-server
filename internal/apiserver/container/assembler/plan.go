package assembler

import (
	"strings"

	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	planDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	planCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	planInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	planEntryInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// PlanModule Plan 模块（测评计划子域）
// 按照 DDD 限界上下文组织
type PlanModule struct {
	// repository 层
	PlanRepo planDomain.AssessmentPlanRepository
	TaskRepo planDomain.AssessmentTaskRepository

	// handler 层
	Handler *handler.PlanHandler

	// service 层
	CommandService planApp.PlanCommandService
	QueryService   planApp.PlanQueryService

	// 事件发布器（由容器统一注入）
	eventPublisher      event.EventPublisher
	testeeAccessService actorAccessApp.TesteeAccessService
}

type planModuleDeps struct {
	mysqlDB        *gorm.DB
	eventPublisher event.EventPublisher
	scaleRepo      scale.Repository
	redisClient    redis.UniversalClient
	cacheBuilder   *rediskey.Builder
	planPolicy     cachepolicy.CachePolicy
	entryBaseURL   string
	observer       *planCache.Observer
}

// NewPlanModule 创建 Plan 模块
func NewPlanModule() *PlanModule {
	return &PlanModule{}
}

// Initialize 初始化 Plan 模块
// params[0]: *gorm.DB
// params[1]: event.EventPublisher (可选，默认使用 NopEventPublisher)
// params[2]: scale.Repository (可选，用于通过 code 查找 scale)
// params[3]: redis.UniversalClient (可选，用于缓存装饰器)
// params[4]: *rediskey.Builder (可选，用于对象缓存 key builder)
// params[5]: cachepolicy.CachePolicy (可选，用于计划详情缓存策略)
// params[6]: string (可选，用于入口基础地址)
func (m *PlanModule) Initialize(params ...interface{}) error {
	deps, err := parsePlanModuleDeps(params)
	if err != nil {
		return err
	}
	m.eventPublisher = deps.eventPublisher

	// 初始化 repository 层
	// 初始化基础 Repository
	basePlanRepo := planInfra.NewPlanRepository(deps.mysqlDB)

	// 如果提供了 Redis 客户端，使用缓存装饰器
	if deps.redisClient != nil {
		m.PlanRepo = planCache.NewCachedPlanRepositoryWithBuilderPolicyAndObserver(basePlanRepo, deps.redisClient, deps.cacheBuilder, deps.planPolicy, deps.observer)
	} else {
		m.PlanRepo = basePlanRepo
	}

	m.TaskRepo = planInfra.NewTaskRepository(deps.mysqlDB)

	// 初始化基础设施层（入口生成器）
	entryGenerator := planEntryInfra.NewEntryGenerator(deps.entryBaseURL)

	// 初始化 service 层（依赖 repository，使用模块统一的事件发布器）
	lifecycleService := planApp.NewLifecycleService(m.PlanRepo, m.TaskRepo, deps.scaleRepo, m.eventPublisher)
	enrollmentService := planApp.NewEnrollmentService(m.PlanRepo, m.TaskRepo, m.eventPublisher)
	taskSchedulerService := planApp.NewTaskSchedulerService(m.TaskRepo, m.PlanRepo, entryGenerator, m.eventPublisher)
	taskManagementService := planApp.NewTaskManagementService(m.TaskRepo, m.eventPublisher)
	m.CommandService = planApp.NewCommandService(
		lifecycleService,
		enrollmentService,
		taskSchedulerService,
		taskManagementService,
		m.PlanRepo,
		m.TaskRepo,
	)
	m.QueryService = planApp.NewQueryService(m.PlanRepo, m.TaskRepo, deps.scaleRepo)

	// 初始化 handler 层
	m.Handler = handler.NewPlanHandler(
		m.CommandService,
		m.QueryService,
	)
	if m.testeeAccessService != nil {
		m.Handler.SetTesteeAccessService(m.testeeAccessService)
	}

	return nil
}

func parsePlanModuleDeps(params []interface{}) (*planModuleDeps, error) {
	if len(params) < 1 {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is required")
	}

	mysqlDB, ok := params[0].(*gorm.DB)
	if !ok || mysqlDB == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	deps := &planModuleDeps{
		mysqlDB:        mysqlDB,
		eventPublisher: event.NewNopEventPublisher(),
		entryBaseURL:   apiserveroptions.DefaultPlanEntryBaseURL,
	}
	applyOptionalParam(params, 1, func(publisher event.EventPublisher) {
		if publisher != nil {
			deps.eventPublisher = publisher
		}
	})
	applyOptionalParam(params, 2, func(repo scale.Repository) {
		if repo != nil {
			deps.scaleRepo = repo
		}
	})
	applyOptionalParam(params, 3, func(client redis.UniversalClient) {
		if client != nil {
			deps.redisClient = client
		}
	})
	applyOptionalParam(params, 4, func(builder *rediskey.Builder) {
		deps.cacheBuilder = builder
	})
	applyOptionalParam(params, 5, func(policy cachepolicy.CachePolicy) {
		deps.planPolicy = policy
	})
	applyOptionalParam(params, 6, func(baseURL string) {
		if strings.TrimSpace(baseURL) != "" {
			deps.entryBaseURL = strings.TrimSpace(baseURL)
		}
	})
	applyOptionalParam(params, 7, func(observer *planCache.Observer) {
		deps.observer = observer
	})
	return deps, nil
}

// SetTesteeAccessService 设置 testee 访问控制服务。
func (m *PlanModule) SetTesteeAccessService(testeeAccessService actorAccessApp.TesteeAccessService) {
	m.testeeAccessService = testeeAccessService
	if m.Handler != nil {
		m.Handler.SetTesteeAccessService(testeeAccessService)
	}
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
