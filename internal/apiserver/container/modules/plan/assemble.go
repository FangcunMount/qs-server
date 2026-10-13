package plan

import (
	"context"
	"strings"

	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	plancache "github.com/FangcunMount/qs-server/internal/apiserver/cache/plan"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	planDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	planInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	planEntryInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/plan"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/planreadmodel"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
)

// Module assembles plan application services.
type Module struct {
	OperatorCommandService        planApp.PlanCommandService
	CommandService                planApp.PlanCommandService
	QueryService                  planApp.PlanQueryService
	EnrollmentQueryService        planApp.EnrollmentQueryService
	TaskAssessmentResolver        planApp.TaskAssessmentResolver
	TaskEntryResolver             planApp.TaskEntryResolver
	TaskNotificationContextReader planApp.TaskNotificationContextReader
	TaskReminderStateReader       planApp.TaskReminderStateReader
	FollowUpQueueReader           planreadmodel.FollowUpQueueReader

	eventPublisher event.EventPublisher
}

// Deps defines explicit constructor dependencies for the plan module.
type Deps struct {
	MySQLDB                   *gorm.DB
	EventPublisher            event.EventPublisher
	PublishedModels           modelcatalogport.PublishedModelLister
	RedisClient               redis.UniversalClient
	CacheBuilder              *keyspace.Builder
	CachePolicies             sharedcache.PolicyProvider
	EntryBaseURL              string
	Observer                  *observability.ComponentObserver
	MySQLLimiter              backpressure.Acquirer
	TesteeAccess              actorAccessApp.TesteeAccessService
	OpenedOutbox              appEventing.ProfileBinding
	TaskOpenedReminderEnabled bool
}

// New assembles the plan module.
func New(deps Deps) (*Module, error) {
	normalized, err := normalizeDeps(deps)
	if err != nil {
		return nil, err
	}

	module := &Module{}
	module.eventPublisher = planEntryInfra.NewCommitPublisher(normalized.EventPublisher)

	mysqlOptions := mysql.BaseRepositoryOptions{Limiter: normalized.MySQLLimiter}
	basePlanRepo := planInfra.NewPlanRepository(normalized.MySQLDB, mysqlOptions)

	planRepo := basePlanRepo
	if normalized.RedisClient != nil {
		planRepo = plancache.NewCachedPlanRepositoryWithBuilderProviderAndObserver(basePlanRepo, normalized.RedisClient, normalized.CacheBuilder, normalized.CachePolicies, normalized.Observer)
	}

	taskRepo := planInfra.NewTaskRepository(normalized.MySQLDB, mysqlOptions)
	enrollmentRepo := planInfra.NewEnrollmentRepository(normalized.MySQLDB, mysqlOptions)
	txRunner := modtx.NewMySQLRunner(normalized.MySQLDB)
	entryGenerator := planEntryInfra.NewEntryGenerator(normalized.EntryBaseURL)
	scaleCatalog := planApp.NewPublishedScaleCatalog(normalized.PublishedModels)
	planReadModel := planInfra.NewReadModel(normalized.MySQLDB)
	module.FollowUpQueueReader = planReadModel

	lifecycleEnrollments, ok := enrollmentRepo.(planDomain.PlanEnrollmentLifecycleRepository)
	if !ok {
		return nil, missingModuleDependency("enrollment lifecycle transitions")
	}
	lifecycleService, err := planApp.NewLifecycleService(planRepo, taskRepo, scaleCatalog, lifecycleEnrollments, txRunner, module.eventPublisher)
	if err != nil {
		return nil, err
	}
	enrollmentService := planApp.NewEnrollmentService(planRepo, taskRepo, enrollmentRepo, txRunner, module.eventPublisher)
	taskSchedulerService, err := planApp.NewTaskSchedulerService(taskRepo, planRepo, enrollmentRepo, txRunner, entryGenerator, module.eventPublisher, planApp.TaskOpeningOptions{ReminderEnabled: normalized.TaskOpenedReminderEnabled, ReminderOutbox: normalized.OpenedOutbox})
	if err != nil {
		return nil, err
	}
	taskManagementService, err := planApp.NewTaskManagementService(taskRepo, planRepo, enrollmentRepo, txRunner, entryGenerator, module.eventPublisher, planApp.TaskOpeningOptions{ReminderEnabled: normalized.TaskOpenedReminderEnabled, ReminderOutbox: normalized.OpenedOutbox})
	if err != nil {
		return nil, err
	}
	module.CommandService = planApp.NewCommandService(
		lifecycleService,
		enrollmentService,
		taskSchedulerService,
		taskManagementService,
		planRepo,
		taskRepo,
	)
	operatorAccess, _ := normalized.TesteeAccess.(planApp.OperatorCommandScope)
	module.OperatorCommandService = planApp.NewOperatorCommandService(module.CommandService, operatorAccess, taskRepo, planApp.OperatorMutationDependencies{Transaction: txRunner, Ownership: planInfra.OwnershipLocker{}})
	enrollmentAccess, _ := normalized.TesteeAccess.(planApp.EnrollmentScopeChecker)
	module.QueryService = planApp.NewQueryService(planReadModel, planReadModel, scaleCatalog, enrollmentAccess)
	module.EnrollmentQueryService = planApp.NewEnrollmentQueryService(planInfra.NewEnrollmentReadStore(normalized.MySQLDB, normalized.MySQLLimiter), scaleCatalog, enrollmentAccess)
	module.TaskAssessmentResolver = planApp.NewTaskAssessmentResolver(taskRepo)
	module.TaskEntryResolver, err = planApp.NewTaskEntryResolver(taskRepo, scaleCatalog, enrollmentRepo)
	if err != nil {
		return nil, err
	}
	module.TaskNotificationContextReader = planApp.NewTaskNotificationContextReader(taskRepo, planRepo)
	module.TaskReminderStateReader = planApp.NewTaskReminderStateReader(taskRepo)

	return module, nil
}

func normalizeDeps(deps Deps) (Deps, error) {
	if deps.MySQLDB == nil {
		return Deps{}, missingModuleDependency("MySQL database")
	}
	if deps.PublishedModels == nil {
		return Deps{}, missingModuleDependency("published model catalog")
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

func missingModuleDependency(dependency string) error {
	logger.L(context.Background()).Errorw("required Plan module dependency is missing", "dependency", dependency)
	return errors.WithCode(code.ErrModuleInitializationFailed, "plan module: %s is required", dependency)
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
