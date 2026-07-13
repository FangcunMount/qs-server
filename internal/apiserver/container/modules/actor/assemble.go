package actor

import (
	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	assessmentEntryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/assessmententry"
	clinicianApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	operatorApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	testeeCache "github.com/FangcunMount/qs-server/internal/apiserver/cache/adapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	assessmentEntryDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/assessmententry"
	clinicianDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	actorInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
)

// Module assembles actor application services.
type Module struct {
	TesteeRegistrationService        testeeApp.TesteeRegistrationService
	TesteeManagementService          testeeApp.TesteeManagementService
	TesteeQueryService               testeeApp.TesteeQueryService
	TesteeBackendQueryService        testeeApp.TesteeBackendQueryService
	TesteeAssessmentAttentionService testeeApp.TesteeAssessmentAttentionService

	OperatorLifecycleService      operatorApp.OperatorLifecycleService
	OperatorAuthorizationService  operatorApp.OperatorAuthorizationService
	OperatorQueryService          operatorApp.OperatorQueryService
	ClinicianLifecycleService     clinicianApp.ClinicianLifecycleService
	ClinicianQueryService         clinicianApp.ClinicianQueryService
	ClinicianRelationshipService  clinicianApp.ClinicianRelationshipService
	AssessmentEntryService        assessmentEntryApp.AssessmentEntryService
	TesteeAccessService           actorAccessApp.TesteeAccessService
	ActiveOperatorChecker         operatorApp.ActiveOperatorChecker
	OperatorRoleProjectionUpdater operatorApp.OperatorRoleProjectionUpdater
	ReadModel                     actorreadmodel.ReadModel
}

// Deps defines explicit constructor dependencies for the actor module.
type Deps struct {
	MySQLDB             *gorm.DB
	ProfileLinkService  *iam.ProfileLinkService
	IdentityService     *iam.IdentityService
	RedisClient         redis.UniversalClient
	CacheBuilder        *keyspace.Builder
	TesteePolicy        cachepolicy.CachePolicy
	OperatorAuthz       *iam.OperatorAuthzBundle
	OperationAccountSvc *iam.OperationAccountService
	Observer            *observability.ComponentObserver
	MySQLLimiter        backpressure.Acquirer
}

// New assembles the actor module.
func New(deps Deps) (*Module, error) {
	if deps.MySQLDB == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	module := &Module{}
	mysqlDB := deps.MySQLDB
	profileLinkSvc := deps.ProfileLinkService
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
	profileLinkDirectory := iam.NewProfileLinkDirectory(profileLinkSvc, identitySvc)

	txRunner := modtx.NewMySQLRunner(mysqlDB)
	mysqlOptions := mysql.BaseRepositoryOptions{Limiter: deps.MySQLLimiter}

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
	testeeValidator := testee.NewValidator(testeeRepo)
	testeeFactory := testee.NewFactory(testeeRepo, testeeValidator)
	testeeEditor := testee.NewEditor(testeeValidator)
	testeeBinder := testee.NewBinder(testeeRepo)

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
		profileLinkSvc,
	)
	module.TesteeManagementService = testeeApp.NewManagementService(
		testeeRepo,
		testeeEditor,
		testeeBinder,
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
		profileLinkDirectory,
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
		profileLinkSvc,
		resolveLogWriter,
		intakeLogWriter,
		txRunner,
		actorReadModel,
	)

	return module, nil
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
		Description: "Actor 管理模块（测评对象和工作人员）",
	}
}
