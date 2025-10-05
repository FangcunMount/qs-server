package assembler

import (
	"gorm.io/gorm"

	userApp "github.com/fangcun-mount/qs-server/internal/apiserver/application/user"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/port"
	userInfra "github.com/fangcun-mount/qs-server/internal/apiserver/infra/mysql/user"
	"github.com/fangcun-mount/qs-server/internal/apiserver/interface/restful/handler"
	"github.com/fangcun-mount/qs-server/internal/pkg/code"
	"github.com/fangcun-mount/qs-server/pkg/errors"
)

// Module 用户模块
// 负责组装用户相关的所有组件
type UserModule struct {
	// repository 层
	UserRepo port.UserRepository

	// handler 层
	UserHandler *handler.UserHandler

	// service 层
	UserCreator         port.UserCreator
	UserQueryer         port.UserQueryer
	UserEditor          port.UserEditor
	UserActivator       port.UserActivator
	UserPasswordChanger port.PasswordChanger
}

// NewModule 创建用户模块
func NewUserModule() *UserModule {
	return &UserModule{}
}

// Initialize 初始化模块
func (m *UserModule) Initialize(params ...interface{}) error {
	db := params[0].(*gorm.DB)
	if db == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	// 初始化 repository 层
	m.UserRepo = userInfra.NewRepository(db)

	// 初始化 service 层
	m.UserCreator = userApp.NewUserCreator(m.UserRepo)
	m.UserQueryer = userApp.NewUserQueryer(m.UserRepo)
	m.UserEditor = userApp.NewUserEditor(m.UserRepo)
	m.UserActivator = userApp.NewUserActivator(m.UserRepo)
	m.UserPasswordChanger = userApp.NewPasswordChanger(m.UserRepo)

	// 初始化 handler 层
	m.UserHandler = handler.NewUserHandler(
		m.UserCreator,
		m.UserQueryer,
		m.UserEditor,
		m.UserActivator,
		m.UserPasswordChanger,
	)

	return nil
}

// Cleanup 清理模块资源
func (m *UserModule) Cleanup() error {
	// 如果有需要清理的资源，在这里进行清理
	// 比如关闭数据库连接、释放缓存等
	return nil
}

// CheckHealth 检查模块健康状态
func (m *UserModule) CheckHealth() error {
	return nil
}

// ModuleInfo 返回模块信息
func (m *UserModule) ModuleInfo() ModuleInfo {
	return ModuleInfo{
		Name:        "user",
		Version:     "1.0.0",
		Description: "用户管理模块",
	}
}
