package assembler

import (
	"gorm.io/gorm"

	"github.com/FangcunMount/component-base/pkg/errors"
	staffApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/staff"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/staff"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	actorInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
)

// ActorModule Actor 模块（测评对象和工作人员）
type ActorModule struct {
	// repository 层
	TesteeRepo testee.Repository
	StaffRepo  staff.Repository

	// handler 层
	ActorHandler *handler.ActorHandler

	// testee service 层（按行为者组织）
	TesteeRegistrationService testeeApp.TesteeRegistrationService // 注册服务 - C端用户
	TesteeManagementService   testeeApp.TesteeManagementService   // 管理服务 - B端员工
	TesteeQueryService        testeeApp.TesteeQueryService        // 查询服务 - 通用（小程序、C端）
	TesteeBackendQueryService testeeApp.TesteeBackendQueryService // 后台查询服务 - B端员工（包含家长信息）

	// staff service 层（按行为者组织）
	StaffLifecycleService     staffApp.StaffLifecycleService     // 生命周期服务 - 人事/行政
	StaffAuthorizationService staffApp.StaffAuthorizationService // 权限管理服务 - IT管理员
	StaffQueryService         staffApp.StaffQueryService         // 查询服务 - 通用
}

// NewActorModule 创建 Actor 模块
func NewActorModule() *ActorModule {
	return &ActorModule{}
}

// Initialize 初始化模块
func (m *ActorModule) Initialize(params ...interface{}) error {
	mysqlDB := params[0].(*gorm.DB)
	if mysqlDB == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	// 提取可选的 guardianshipSvc 参数
	var guardianshipSvc *iam.GuardianshipService
	if len(params) > 1 {
		if svc, ok := params[1].(*iam.GuardianshipService); ok {
			guardianshipSvc = svc
		}
	}

	// 初始化 UnitOfWork
	uow := mysql.NewUnitOfWork(mysqlDB)

	// 初始化 repository 层
	m.TesteeRepo = actorInfra.NewTesteeRepository(mysqlDB)
	m.StaffRepo = actorInfra.NewStaffRepository(mysqlDB)

	// 初始化 testee domain services
	testeeValidator := testee.NewValidator(m.TesteeRepo)
	testeeFactory := testee.NewFactory(m.TesteeRepo, testeeValidator)
	testeeEditor := testee.NewEditor(testeeValidator)
	testeeBinder := testee.NewBinder(m.TesteeRepo)
	testeeTagger := testee.NewTagger(testeeValidator)

	// 初始化 staff domain services
	staffValidator := staff.NewValidator()
	staffFactory := staff.NewFactory(m.StaffRepo, staffValidator)
	staffEditor := staff.NewEditor(staffValidator)
	staffBinder := staff.NewBinder(m.StaffRepo, staffValidator)
	staffRoleAllocator := staff.NewRoleAllocator(staffValidator)
	staffLifecycler := staff.NewLifecycler(staffRoleAllocator)

	// 初始化 testee service 层（按行为者组织）
	// 注册服务 - 服务于C端用户（患者/家长）
	m.TesteeRegistrationService = testeeApp.NewRegistrationService(
		m.TesteeRepo,
		testeeFactory,
		testeeValidator,
		testeeBinder,
		uow,
		guardianshipSvc,
	)
	// 管理服务 - 服务于B端员工（Staff）
	m.TesteeManagementService = testeeApp.NewManagementService(
		m.TesteeRepo,
		testeeEditor,
		testeeBinder,
		testeeTagger,
		uow,
	)
	// 查询服务 - 服务于所有需要查询的用户（小程序、C端）
	m.TesteeQueryService = testeeApp.NewQueryService(m.TesteeRepo)

	// 初始化 staff service 层（按行为者组织）
	// 生命周期服务 - 服务于人事/行政部门
	// 初始化 Staff Lifecycle Service，注入 IAM IdentityService 如果可用
	var identitySvc *iam.IdentityService
	// 尝试从外部容器的 IAMModule 提取（在 Initialize 时 container 会传入该参数为 params[2]）
	if len(params) > 2 {
		if svc, ok := params[2].(*iam.IdentityService); ok {
			identitySvc = svc
		}
	}

	// 后台查询服务 - 服务于B端员工（包含家长信息）
	// 需要 IdentityService 来查询用户详细信息（当 ListGuardians 只返回 guardianship 时）
	m.TesteeBackendQueryService = testeeApp.NewBackendQueryService(
		m.TesteeQueryService,
		guardianshipSvc,
		identitySvc,
	)
	m.StaffLifecycleService = staffApp.NewLifecycleService(
		m.StaffRepo,
		staffFactory,
		staffValidator,
		staffEditor,
		staffRoleAllocator,
		staffBinder,
		uow,
		identitySvc,
	)
	// 权限管理服务 - 服务于IT管理员
	m.StaffAuthorizationService = staffApp.NewAuthorizationService(
		m.StaffRepo,
		staffValidator,
		staffRoleAllocator,
		staffLifecycler,
		uow,
	)
	// 查询服务 - 服务于所有需要查询的用户
	m.StaffQueryService = staffApp.NewQueryService(m.StaffRepo)

	// 初始化 handler 层 - 先不注入评估服务（评估模块还未初始化）
	// 评估服务将在容器初始化完成后通过 SetEvaluationServices 方法注入
	m.ActorHandler = handler.NewActorHandler(
		m.TesteeRegistrationService,
		m.TesteeManagementService,
		m.TesteeQueryService,
		m.TesteeBackendQueryService,
		m.StaffLifecycleService,
		m.StaffAuthorizationService,
		m.StaffQueryService,
		guardianshipSvc,
		nil, // assessmentManagementService - 稍后注入
		nil, // scoreQueryService - 稍后注入
	)

	return nil
}

// SetEvaluationServices 设置评估服务（用于延迟注入）
func (m *ActorModule) SetEvaluationServices(
	assessmentManagementService assessmentApp.AssessmentManagementService,
	scoreQueryService assessmentApp.ScoreQueryService,
) {
	if m.ActorHandler != nil {
		m.ActorHandler.SetEvaluationServices(assessmentManagementService, scoreQueryService)
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
