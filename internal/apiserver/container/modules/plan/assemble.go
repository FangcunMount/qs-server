package plan

import (
	"strings"

	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	planCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	planInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	planEntryInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/plan"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/planreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// Module assembles plan application services.
type Module struct {
	CommandService                planApp.PlanCommandService
	QueryService                  planApp.PlanQueryService
	TaskAssessmentResolver        planApp.TaskAssessmentResolver
	TaskNotificationContextReader planApp.TaskNotificationContextReader
	FollowUpQueueReader           planreadmodel.FollowUpQueueReader

	eventPublisher      event.EventPublisher
	testeeAccessService actorAccessApp.TesteeAccessService
}

// Deps defines explicit constructor dependencies for the plan module.
type Deps struct {
	MySQLDB             *gorm.DB
	EventPublisher      event.EventPublisher
	AssessmentModelRepo modelcatalogport.ModelRepository
	RedisClient         redis.UniversalClient
	CacheBuilder        *keyspace.Builder
	PlanPolicy          cachepolicy.CachePolicy
	EntryBaseURL        string
	Observer            *observability.ComponentObserver
	MySQLLimiter        backpressure.Acquirer
	TesteeAccess        actorAccessApp.TesteeAccessService
}

// New assembles the plan module.
func New(deps Deps) (*Module, error) {
	normalized, err := normalizeDeps(deps)
	if err != nil {
		return nil, err
	}

	module := &Module{}
	module.eventPublisher = normalized.EventPublisher
	module.testeeAccessService = normalized.TesteeAccess

	mysqlOptions := mysql.BaseRepositoryOptions{Limiter: normalized.MySQLLimiter}
	basePlanRepo := planInfra.NewPlanRepository(normalized.MySQLDB, mysqlOptions)

	planRepo := basePlanRepo
	if normalized.RedisClient != nil {
		planRepo = planCache.NewCachedPlanRepositoryWithBuilderPolicyAndObserver(basePlanRepo, normalized.RedisClient, normalized.CacheBuilder, normalized.PlanPolicy, normalized.Observer)
	}

	taskRepo := planInfra.NewTaskRepository(normalized.MySQLDB, mysqlOptions)
	entryGenerator := planEntryInfra.NewEntryGenerator(normalized.EntryBaseURL)
	scaleCatalog := planApp.NewAssessmentModelScaleCatalog(normalized.AssessmentModelRepo)
	planReadModel := planInfra.NewReadModel(normalized.MySQLDB)
	module.FollowUpQueueReader = planReadModel

	lifecycleService := planApp.NewLifecycleServiceWithScaleCatalog(planRepo, taskRepo, scaleCatalog, module.eventPublisher)
	enrollmentService := planApp.NewEnrollmentService(planRepo, taskRepo, module.eventPublisher)
	taskSchedulerService := planApp.NewTaskSchedulerService(taskRepo, planRepo, entryGenerator, module.eventPublisher)
	taskManagementService := planApp.NewTaskManagementService(taskRepo, entryGenerator, module.eventPublisher)
	module.CommandService = planApp.NewCommandService(
		lifecycleService,
		enrollmentService,
		taskSchedulerService,
		taskManagementService,
		planRepo,
		taskRepo,
	)
	module.QueryService = planApp.NewQueryService(planReadModel, planReadModel, scaleCatalog)
	module.TaskAssessmentResolver = planApp.NewTaskAssessmentResolver(taskRepo)
	module.TaskNotificationContextReader = planApp.NewTaskNotificationContextReader(taskRepo, planRepo)

	return module, nil
}

func normalizeDeps(deps Deps) (Deps, error) {
	if deps.MySQLDB == nil {
		return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
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

// Cleanup releases module resources.
func (m *Module) Cleanup() error {
	return nil
}

// CheckHealth verifies module health.
func (m *Module) CheckHealth() error {
	return nil
}

// ModuleInfo returns module metadata.
func (m *Module) ModuleInfo() modules.ModuleInfo {
	return modules.ModuleInfo{
		Name:        string(Name),
		Version:     "1.0.0",
		Description: "测评计划管理模块",
	}
}
