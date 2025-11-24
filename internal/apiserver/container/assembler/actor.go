package assembler

import (
	"gorm.io/gorm"

	"github.com/FangcunMount/component-base/pkg/errors"
	staffApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/staff_management"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee_management"
	testeeReg "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee_registration"
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
	TesteeRegistrationService testeeReg.TesteeRegistrationApplicationService
	TesteeProfileService      testeeApp.TesteeProfileApplicationService
	TesteeTagService          testeeApp.TesteeTagApplicationService
	TesteeQueryService        testeeApp.TesteeQueryApplicationService
	TesteeService             testeeApp.Service // 聚合服务，用于 gRPC

	// staff service 层
	StaffService        staffApp.StaffApplicationService
	StaffProfileService staffApp.StaffProfileApplicationService
	StaffRoleService    staffApp.StaffRoleApplicationService
	StaffQueryService   staffApp.StaffQueryApplicationService
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
	testeeValidator := testee.NewValidator()
	testeeFactory := testee.NewFactory(m.TesteeRepo, testeeValidator)
	testeeEditor := testee.NewEditor(testeeValidator)
	testeeBinder := testee.NewBinder(m.TesteeRepo)

	// 初始化 staff domain services
	staffValidator := staff.NewValidator()
	staffFactory := staff.NewFactory(m.StaffRepo, staffValidator)
	staffEditor := staff.NewEditor(staffValidator)
	staffRoleManager := staff.NewRoleManager(staffValidator)
	staffIAMSync := staff.NewIAMSynchronizer(m.StaffRepo, staffValidator)

	// 初始化 testee service 层
	m.TesteeRegistrationService = testeeReg.NewRegistrationService(
		m.TesteeRepo,
		testeeFactory,
		testeeValidator,
		testeeBinder,
		uow,
	)
	m.TesteeProfileService = testeeApp.NewProfileService(
		m.TesteeRepo,
		testeeValidator,
		testeeEditor,
		testeeBinder,
		uow,
	)
	m.TesteeTagService = testeeApp.NewTagService(
		m.TesteeRepo,
		testeeEditor,
		uow,
	)
	m.TesteeQueryService = testeeApp.NewQueryService(m.TesteeRepo)

	// 初始化 staff service 层
	m.StaffService = staffApp.NewStaffService(
		m.StaffRepo,
		staffFactory,
		staffValidator,
		staffRoleManager,
		uow,
	)
	m.StaffProfileService = staffApp.NewProfileService(
		m.StaffRepo,
		staffEditor,
		staffIAMSync,
		uow,
	)
	m.StaffRoleService = staffApp.NewRoleService(
		m.StaffRepo,
		staffValidator,
		staffRoleManager,
		staffEditor,
		uow,
	)
	m.StaffQueryService = staffApp.NewQueryService(m.StaffRepo)

	// 初始化聚合服务（为 Handler 提供统一接口）
	testeeCompositeService := testeeApp.NewCompositeService(
		m.TesteeRegistrationService,
		m.TesteeProfileService,
		m.TesteeTagService,
		m.TesteeQueryService,
	)
	m.TesteeService = testeeCompositeService // 保存聚合服务供 gRPC 使用

	staffCompositeService := staffApp.NewCompositeService(
		m.StaffService,
		m.StaffProfileService,
		m.StaffRoleService,
		m.StaffQueryService,
	)

	// 初始化 handler 层
	m.ActorHandler = handler.NewActorHandler(
		testeeCompositeService,
		staffCompositeService,
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
