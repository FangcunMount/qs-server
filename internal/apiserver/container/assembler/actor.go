package assembler

import (
	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	assessmentEntryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/assessmententry"
	clinicianApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	operatorApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	assessmentEntryDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/assessmententry"
	clinicianDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	testeeCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	actorInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	mysqlEventOutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventoutbox"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

// ActorModule Actor 模块（测评对象和工作人员）
type ActorModule struct {
	// testee service 层（按行为者组织）
	TesteeRegistrationService        testeeApp.TesteeRegistrationService        // 注册服务 - C端用户
	TesteeManagementService          testeeApp.TesteeManagementService          // 管理服务 - B端员工
	TesteeQueryService               testeeApp.TesteeQueryService               // 查询服务 - 通用（小程序、C端）
	TesteeBackendQueryService        testeeApp.TesteeBackendQueryService        // 后台查询服务 - B端员工（包含家长信息）
	TesteeAssessmentAttentionService testeeApp.TesteeAssessmentAttentionService // 测评后置关注同步服务 - 系统自动（事件驱动）

	// operator service 层（按行为者组织）
	OperatorLifecycleService      operatorApp.OperatorLifecycleService      // 生命周期服务 - 人事/行政
	OperatorAuthorizationService  operatorApp.OperatorAuthorizationService  // 权限管理服务 - IT管理员
	OperatorQueryService          operatorApp.OperatorQueryService          // 查询服务 - 通用
	ClinicianLifecycleService     clinicianApp.ClinicianLifecycleService    // 生命周期服务 - 人事/行政部门
	ClinicianQueryService         clinicianApp.ClinicianQueryService        // 查询服务 - 通用
	ClinicianRelationshipService  clinicianApp.ClinicianRelationshipService // 关系服务 - 建立从业者-受试者关系、查询名下受试者
	AssessmentEntryService        assessmentEntryApp.AssessmentEntryService // 创建测评入口、解析 token、扫码 intake
	TesteeAccessService           actorAccessApp.TesteeAccessService        // 解析后台访问范围：admin bypass / ClinicianTesteeRelation
	ActiveOperatorChecker         operatorApp.ActiveOperatorChecker         // REST/gRPC 认证期 active operator 检查
	OperatorRoleProjectionUpdater operatorApp.OperatorRoleProjectionUpdater // 将 IAM 角色快照投影回本地 operator
	ReadModel                     actorreadmodel.ReadModel                  // 只读投影，供跨模块工作台等读侧编排使用
}

// ActorModuleDeps 定义 Actor 模块的显式构造依赖。
type ActorModuleDeps struct {
	MySQLDB             *gorm.DB
	GuardianshipService *iam.GuardianshipService
	IdentityService     *iam.IdentityService
	RedisClient         redis.UniversalClient
	CacheBuilder        *keyspace.Builder
	TesteePolicy        cachepolicy.CachePolicy
	OperatorAuthz       *iam.OperatorAuthzBundle
	OperationAccountSvc *iam.OperationAccountService
	Observer            *observability.ComponentObserver
	TopicResolver       eventcatalog.TopicResolver
	MySQLLimiter        backpressure.Acquirer
}

// NewActorModule 创建 Actor 模块。
func NewActorModule(deps ActorModuleDeps) (*ActorModule, error) {
	if deps.MySQLDB == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	module := &ActorModule{}
	mysqlDB := deps.MySQLDB
	guardianshipSvc := deps.GuardianshipService
	identitySvc := deps.IdentityService
	operationAccountSvc := deps.OperationAccountSvc
	var authzAssign *iam.AuthzAssignmentClient
	var authzSnap *iam.AuthzSnapshotLoader
	if deps.OperatorAuthz != nil {
		authzAssign = deps.OperatorAuthz.Assignment
		authzSnap = deps.OperatorAuthz.Snapshot
	}
	authzSnapshotReader := iam.NewAuthzSnapshotReader(authzSnap)
	operatorAuthzGateway := iam.NewOperatorAuthzGateway(authzAssign, authzSnap)
	userDirectory := iam.NewUserDirectory(identitySvc)
	accountRegistrar := iam.NewOperationAccountRegistrar(operationAccountSvc)
	guardianDirectory := iam.NewGuardianDirectory(guardianshipSvc, identitySvc)

	txRunner := newMySQLTransactionRunner(mysqlDB)
	mysqlOptions := mysql.BaseRepositoryOptions{Limiter: deps.MySQLLimiter}

	// 初始化 repository 层
	baseTesteeRepo := actorInfra.NewTesteeRepository(mysqlDB, mysqlOptions)

	var testeeRepo testee.Repository
	if deps.RedisClient != nil {
		testeeRepo = testeeCache.NewCachedTesteeRepositoryWithBuilderPolicyAndObserver(baseTesteeRepo, deps.RedisClient, deps.CacheBuilder, deps.TesteePolicy, deps.Observer)
	} else {
		testeeRepo = baseTesteeRepo
	}

	operatorRepo := actorInfra.NewOperatorRepository(mysqlDB, mysqlOptions)
	clinicianRepo := actorInfra.NewClinicianRepository(mysqlDB, mysqlOptions)
	relationRepo := actorInfra.NewRelationRepository(mysqlDB, mysqlOptions)
	assessmentEntryRepo := actorInfra.NewAssessmentEntryRepository(mysqlDB, mysqlOptions)
	actorReadModel := actorInfra.NewReadModel(mysqlDB, mysqlOptions)
	module.ReadModel = actorReadModel
	statisticsRepo := statisticsInfra.NewStatisticsRepository(mysqlDB, mysqlOptions)
	resolveLogWriter := statisticsInfra.NewAssessmentEntryResolveLogger(statisticsRepo)
	intakeLogWriter := statisticsInfra.NewAssessmentEntryIntakeLogger(statisticsRepo)
	behaviorEvents := statisticsApp.NewBehaviorEventStager(mysqlEventOutbox.NewStoreWithTopicResolver(mysqlDB, deps.TopicResolver))

	// 初始化 testee domain services
	testeeValidator := testee.NewValidator(testeeRepo)
	testeeFactory := testee.NewFactory(testeeRepo, testeeValidator)
	testeeEditor := testee.NewEditor(testeeValidator)
	testeeBinder := testee.NewBinder(testeeRepo)
	testeeTagger := testee.NewTagger(testeeValidator)

	// 初始化 operator domain services
	operatorValidator := operator.NewValidator()
	operatorFactory := operator.NewFactory(operatorRepo, operatorValidator)
	operatorEditor := operator.NewEditor(operatorValidator)
	operatorRoleAllocator := operator.NewRoleAllocator(operatorValidator)
	operatorLifecycler := operator.NewLifecycler(operatorRoleAllocator)
	clinicianValidator := clinicianDomain.NewValidator()
	assessmentEntryValidator := assessmentEntryDomain.NewValidator()

	module.TesteeRegistrationService = testeeApp.NewRegistrationService(
		testeeRepo,
		testeeFactory,
		testeeValidator,
		testeeBinder,
		txRunner,
		guardianshipSvc,
	)
	module.TesteeManagementService = testeeApp.NewManagementService(
		testeeRepo,
		testeeEditor,
		testeeBinder,
		testeeTagger,
		txRunner,
	)
	module.TesteeQueryService = testeeApp.NewQueryService(actorReadModel)
	module.TesteeAssessmentAttentionService = testeeApp.NewAssessmentAttentionService(
		testeeRepo,
		testeeEditor,
		txRunner,
	)

	module.TesteeBackendQueryService = testeeApp.NewBackendQueryService(
		module.TesteeQueryService,
		guardianDirectory,
	)
	module.OperatorLifecycleService = operatorApp.NewLifecycleService(
		operatorRepo,
		operatorFactory,
		operatorValidator,
		operatorEditor,
		operatorLifecycler,
		operatorRoleAllocator,
		txRunner,
		userDirectory,
		accountRegistrar,
		operatorAuthzGateway,
	)
	module.OperatorAuthorizationService = operatorApp.NewAuthorizationService(
		operatorRepo,
		operatorValidator,
		operatorRoleAllocator,
		operatorLifecycler,
		txRunner,
		operatorAuthzGateway,
	)
	module.OperatorQueryService = operatorApp.NewQueryService(actorReadModel)
	module.ActiveOperatorChecker = operatorApp.NewActiveOperatorChecker(actorReadModel)
	module.OperatorRoleProjectionUpdater = operatorApp.NewRoleProjectionUpdater(operatorRepo)
	module.ClinicianLifecycleService = clinicianApp.NewLifecycleService(
		clinicianRepo,
		operatorRepo,
		clinicianValidator,
		txRunner,
	)
	module.ClinicianQueryService = clinicianApp.NewQueryService(actorReadModel, actorReadModel, actorReadModel)
	module.ClinicianRelationshipService = clinicianApp.NewRelationshipService(
		relationRepo,
		clinicianRepo,
		testeeRepo,
		behaviorEvents,
		txRunner,
		actorReadModel,
	)
	module.TesteeAccessService = actorAccessApp.NewTesteeAccessService(
		actorReadModel,
		actorReadModel,
		actorReadModel,
		actorReadModel,
		authzSnapshotReader,
	)
	module.AssessmentEntryService = assessmentEntryApp.NewService(
		assessmentEntryRepo,
		clinicianRepo,
		relationRepo,
		testeeRepo,
		testeeFactory,
		assessmentEntryValidator,
		guardianshipSvc,
		resolveLogWriter,
		intakeLogWriter,
		behaviorEvents,
		txRunner,
		actorReadModel,
	)

	return module, nil
}

// Cleanup 清理模块资源
func (m *ActorModule) Cleanup() error {
	// 如果有需要清理的资源，在这里进行清理
	return nil
}

// CheckHealth 检查模块健康状态
func (m *ActorModule) CheckHealth() error {
	return nil
}

// ModuleInfo 返回模块信息
func (m *ActorModule) ModuleInfo() ModuleInfo {
	return ModuleInfo{
		Name:        "actor",
		Version:     "1.0.0",
		Description: "Actor 管理模块（测评对象和工作人员）",
	}
}
