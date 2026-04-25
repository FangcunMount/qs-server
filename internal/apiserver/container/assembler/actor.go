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
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	assessmentEntryDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/assessmententry"
	clinicianDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	relationDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	testeeCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	actorInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	mysqlEventOutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventoutbox"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
)

// ActorModule Actor 模块（测评对象和工作人员）
type ActorModule struct {
	// repository 层
	TesteeRepo          testee.Repository
	OperatorRepo        operator.Repository
	ClinicianRepo       clinicianDomain.Repository
	RelationRepo        relationDomain.Repository
	AssessmentEntryRepo assessmentEntryDomain.Repository

	// handler 层
	TesteeHandler            *handler.TesteeHandler
	OperatorClinicianHandler *handler.OperatorClinicianHandler
	AssessmentEntryHandler   *handler.AssessmentEntryHandler

	// testee service 层（按行为者组织）
	TesteeRegistrationService testeeApp.TesteeRegistrationService // 注册服务 - C端用户
	TesteeManagementService   testeeApp.TesteeManagementService   // 管理服务 - B端员工
	TesteeQueryService        testeeApp.TesteeQueryService        // 查询服务 - 通用（小程序、C端）
	TesteeBackendQueryService testeeApp.TesteeBackendQueryService // 后台查询服务 - B端员工（包含家长信息）
	TesteeTaggingService      testeeApp.TesteeTaggingService      // 标签服务 - 系统自动（事件驱动）

	// operator service 层（按行为者组织）
	OperatorLifecycleService      operatorApp.OperatorLifecycleService      // 生命周期服务 - 人事/行政
	OperatorAuthorizationService  operatorApp.OperatorAuthorizationService  // 权限管理服务 - IT管理员
	OperatorQueryService          operatorApp.OperatorQueryService          // 查询服务 - 通用
	ClinicianLifecycleService     clinicianApp.ClinicianLifecycleService    // 生命周期服务 - 人事/行政部门
	ClinicianQueryService         clinicianApp.ClinicianQueryService        // 查询服务 - 通用
	ClinicianRelationshipService  clinicianApp.ClinicianRelationshipService // 关系服务 - 建立从业者-受试者关系、查询名下受试者
	AssessmentEntryService        assessmentEntryApp.AssessmentEntryService // 创建测评入口、解析 token、扫码 intake
	TesteeAccessService           actorAccessApp.TesteeAccessService        // 解析后台访问范围：admin bypass / ClinicianTesteeRelation
	OperatorRoleProjectionUpdater operatorApp.OperatorRoleProjectionUpdater // 将 IAM 角色快照投影回本地 operator
}

// ActorModuleDeps 定义 Actor 模块的显式构造依赖。
type ActorModuleDeps struct {
	MySQLDB             *gorm.DB
	GuardianshipService *iam.GuardianshipService
	IdentityService     *iam.IdentityService
	RedisClient         redis.UniversalClient
	CacheBuilder        *rediskey.Builder
	TesteePolicy        cachepolicy.CachePolicy
	OperatorAuthz       *iam.OperatorAuthzBundle
	OperationAccountSvc *iam.OperationAccountService
	Observer            *cacheobservability.ComponentObserver
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

	// 初始化 UnitOfWork
	uow := mysql.NewUnitOfWork(mysqlDB)
	mysqlOptions := mysql.BaseRepositoryOptions{Limiter: deps.MySQLLimiter}

	// 初始化 repository 层
	baseTesteeRepo := actorInfra.NewTesteeRepository(mysqlDB, mysqlOptions)

	if deps.RedisClient != nil {
		module.TesteeRepo = testeeCache.NewCachedTesteeRepositoryWithBuilderPolicyAndObserver(baseTesteeRepo, deps.RedisClient, deps.CacheBuilder, deps.TesteePolicy, deps.Observer)
	} else {
		module.TesteeRepo = baseTesteeRepo
	}

	module.OperatorRepo = actorInfra.NewOperatorRepository(mysqlDB, mysqlOptions)
	module.ClinicianRepo = actorInfra.NewClinicianRepository(mysqlDB, mysqlOptions)
	module.RelationRepo = actorInfra.NewRelationRepository(mysqlDB, mysqlOptions)
	module.AssessmentEntryRepo = actorInfra.NewAssessmentEntryRepository(mysqlDB, mysqlOptions)
	statisticsRepo := statisticsInfra.NewStatisticsRepository(mysqlDB, mysqlOptions)
	resolveLogWriter := statisticsInfra.NewAssessmentEntryResolveLogger(statisticsRepo)
	intakeLogWriter := statisticsInfra.NewAssessmentEntryIntakeLogger(statisticsRepo)
	behaviorEvents := statisticsApp.NewBehaviorEventStager(mysqlEventOutbox.NewStoreWithTopicResolver(mysqlDB, deps.TopicResolver))

	// 初始化 testee domain services
	testeeValidator := testee.NewValidator(module.TesteeRepo)
	testeeFactory := testee.NewFactory(module.TesteeRepo, testeeValidator)
	testeeEditor := testee.NewEditor(testeeValidator)
	testeeBinder := testee.NewBinder(module.TesteeRepo)
	testeeTagger := testee.NewTagger(testeeValidator)

	// 初始化 operator domain services
	operatorValidator := operator.NewValidator()
	operatorFactory := operator.NewFactory(module.OperatorRepo, operatorValidator)
	operatorEditor := operator.NewEditor(operatorValidator)
	operatorBinder := operator.NewBinder(module.OperatorRepo, operatorValidator)
	operatorRoleAllocator := operator.NewRoleAllocator(operatorValidator)
	operatorLifecycler := operator.NewLifecycler(operatorRoleAllocator)
	clinicianValidator := clinicianDomain.NewValidator()
	assessmentEntryValidator := assessmentEntryDomain.NewValidator()

	module.TesteeRegistrationService = testeeApp.NewRegistrationService(
		module.TesteeRepo,
		testeeFactory,
		testeeValidator,
		testeeBinder,
		uow,
		guardianshipSvc,
	)
	module.TesteeManagementService = testeeApp.NewManagementService(
		module.TesteeRepo,
		testeeEditor,
		testeeBinder,
		testeeTagger,
		uow,
	)
	module.TesteeQueryService = testeeApp.NewQueryService(module.TesteeRepo)
	module.TesteeTaggingService = testeeApp.NewTaggingService(
		module.TesteeRepo,
		module.TesteeManagementService,
		module.TesteeQueryService,
		uow,
	)

	module.TesteeBackendQueryService = testeeApp.NewBackendQueryService(
		module.TesteeQueryService,
		guardianshipSvc,
		identitySvc,
	)
	module.OperatorLifecycleService = operatorApp.NewLifecycleService(
		module.OperatorRepo,
		operatorFactory,
		operatorValidator,
		operatorEditor,
		operatorLifecycler,
		operatorRoleAllocator,
		operatorBinder,
		uow,
		identitySvc,
		operationAccountSvc,
		authzAssign,
		authzSnap,
	)
	module.OperatorAuthorizationService = operatorApp.NewAuthorizationService(
		module.OperatorRepo,
		operatorValidator,
		operatorRoleAllocator,
		operatorLifecycler,
		uow,
		authzAssign,
		authzSnap,
	)
	module.OperatorQueryService = operatorApp.NewQueryService(module.OperatorRepo)
	module.OperatorRoleProjectionUpdater = operatorApp.NewRoleProjectionUpdater(module.OperatorRepo)
	module.ClinicianLifecycleService = clinicianApp.NewLifecycleService(
		module.ClinicianRepo,
		module.OperatorRepo,
		clinicianValidator,
		uow,
	)
	module.ClinicianQueryService = clinicianApp.NewQueryService(module.ClinicianRepo, module.RelationRepo, module.AssessmentEntryRepo)
	module.ClinicianRelationshipService = clinicianApp.NewRelationshipService(
		module.RelationRepo,
		module.ClinicianRepo,
		module.TesteeRepo,
		behaviorEvents,
		uow,
	)
	module.TesteeAccessService = actorAccessApp.NewTesteeAccessService(
		module.OperatorRepo,
		module.ClinicianRepo,
		module.RelationRepo,
		module.TesteeRepo,
		authzSnap,
	)
	module.AssessmentEntryService = assessmentEntryApp.NewService(
		module.AssessmentEntryRepo,
		module.ClinicianRepo,
		module.RelationRepo,
		module.TesteeRepo,
		testeeFactory,
		assessmentEntryValidator,
		guardianshipSvc,
		resolveLogWriter,
		intakeLogWriter,
		behaviorEvents,
		uow,
	)

	module.TesteeHandler = handler.NewTesteeHandler(
		module.TesteeManagementService,
		module.TesteeQueryService,
		module.TesteeBackendQueryService,
		module.ClinicianQueryService,
		module.ClinicianRelationshipService,
		module.TesteeAccessService,
		nil,
		nil,
	)
	module.OperatorClinicianHandler = handler.NewOperatorClinicianHandler(
		module.OperatorLifecycleService,
		module.OperatorAuthorizationService,
		module.OperatorQueryService,
		module.ClinicianLifecycleService,
		module.ClinicianQueryService,
		module.ClinicianRelationshipService,
		module.TesteeQueryService,
		module.TesteeAccessService,
	)
	module.AssessmentEntryHandler = handler.NewAssessmentEntryHandler(
		module.OperatorQueryService,
		module.ClinicianQueryService,
		module.AssessmentEntryService,
		nil,
	)

	return module, nil
}

// SetEvaluationServices 设置评估服务（用于延迟注入）
func (m *ActorModule) SetEvaluationServices(
	assessmentManagementService assessmentApp.AssessmentManagementService,
	scoreQueryService assessmentApp.ScoreQueryService,
) {
	if m.TesteeHandler != nil {
		m.TesteeHandler.SetEvaluationServices(assessmentManagementService, scoreQueryService)
	}
}

// SetQRCodeService 设置二维码服务（用于测评入口二维码生成）。
func (m *ActorModule) SetQRCodeService(qrCodeService qrcodeApp.QRCodeService) {
	if m.AssessmentEntryHandler != nil {
		m.AssessmentEntryHandler.SetQRCodeService(qrCodeService)
	}
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
