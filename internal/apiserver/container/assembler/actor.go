package assembler

import (
	"gorm.io/gorm"

	"github.com/FangcunMount/component-base/pkg/errors"
	staffApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/staff"
	testeeManagement "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee/management"
	testeeRegistration "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee/registration"
	testeeShared "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee/shared"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/staff"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
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

	// testee service 层
	TesteeRegistrationService testeeShared.TesteeRegistrationApplicationService
	TesteeProfileService      testeeShared.TesteeProfileApplicationService
	TesteeTagService          testeeShared.TesteeTagApplicationService
	TesteeQueryService        testeeShared.TesteeQueryApplicationService
	TesteeService             testeeShared.Service // 聚合服务，用于 gRPC

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

	// 初始化 testee service 层
	m.TesteeRegistrationService = testeeRegistration.NewRegistrationService(
		m.TesteeRepo,
		testeeFactory,
		testeeValidator,
		testeeBinder,
		uow,
	)
	m.TesteeProfileService = testeeManagement.NewProfileService(
		m.TesteeRepo,
		testeeValidator,
		testeeEditor,
		testeeBinder,
		uow,
	)
	m.TesteeTagService = testeeManagement.NewTagService(
		m.TesteeRepo,
		testeeTagger,
		testeeEditor,
		uow,
	)
	m.TesteeQueryService = testeeManagement.NewQueryService(m.TesteeRepo)

	// 初始化 staff service 层（按行为者组织）
	// 生命周期服务 - 服务于人事/行政部门
	m.StaffLifecycleService = staffApp.NewLifecycleService(
		m.StaffRepo,
		staffFactory,
		staffValidator,
		staffEditor,
		staffRoleAllocator,
		staffBinder,
		uow,
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

	// 初始化聚合服务（为 Handler 提供统一接口）
	testeeCompositeService := testeeShared.NewCompositeService(
		m.TesteeRegistrationService,
		m.TesteeProfileService,
		m.TesteeTagService,
		m.TesteeQueryService,
	)
	m.TesteeService = testeeCompositeService // 保存聚合服务供 gRPC 使用

	// 初始化 handler 层 - 直接使用按行为者划分的服务,不需要 CompositeService
	m.ActorHandler = handler.NewActorHandler(
		testeeCompositeService,
		m.StaffLifecycleService,
		m.StaffAuthorizationService,
		m.StaffQueryService,
	)

	return nil
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
