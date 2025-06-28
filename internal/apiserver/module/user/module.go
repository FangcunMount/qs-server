package user

import (
	"gorm.io/gorm"

	userInfra "github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/driven/mysql/user"
	userAdapter "github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/driving/restful/user"
	userApp "github.com/yshujie/questionnaire-scale/internal/apiserver/application/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
)

// Module 用户模块
// 负责组装用户相关的所有组件
type Module struct {
	// 基础设施层
	userRepository port.UserRepository

	// 应用层
	userCreator         port.UserCreator
	userQueryer         port.UserQueryer
	userEditor          port.UserEditor
	userActivator       port.UserActivator
	userPasswordChanger port.PasswordChanger
	userAuthService     *userApp.AuthService

	// 适配器层
	userHandler *userAdapter.Handler
}

// NewModule 创建用户模块
func NewModule(db *gorm.DB) *Module {
	// 构造基础设施层 - Repository
	userRepository := userInfra.NewRepository(db)

	// 构造应用层 - Service
	userCreator := userApp.NewUserCreator(userRepository)
	userQueryer := userApp.NewUserQueryer(userRepository)
	userEditor := userApp.NewUserEditor(userRepository)
	userActivator := userApp.NewUserActivator(userRepository)
	userPasswordChanger := userApp.NewPasswordChanger(userRepository)
	userAuthService := userApp.NewAuthService(userRepository, userPasswordChanger, userQueryer, userActivator)

	// 构造适配器层 - Handler
	userHandler := userAdapter.NewHandler(userCreator, userQueryer, userEditor, userActivator, userPasswordChanger)

	return &Module{
		userRepository:      userRepository,
		userCreator:         userCreator,
		userQueryer:         userQueryer,
		userEditor:          userEditor,
		userActivator:       userActivator,
		userPasswordChanger: userPasswordChanger,
		userAuthService:     userAuthService,
		userHandler:         userHandler,
	}
}

// GetRepository 获取用户存储库
func (m *Module) GetRepository() port.UserRepository {
	return m.userRepository
}

// GetServices 获取用户服务
func (m *Module) GetServices() []interface{} {
	return []interface{}{
		m.userCreator,
		m.userQueryer,
		m.userEditor,
		m.userActivator,
		m.userPasswordChanger,
	}
}

// GetHandler 获取用户处理器
func (m *Module) GetHandler() *userAdapter.Handler {
	return m.userHandler
}

// GetAuthService 获取认证服务
func (m *Module) GetAuthService() *userApp.AuthService {
	return m.userAuthService
}

// RegisterRoutes 注册用户路由
// 这个方法可以被路由器调用来注册用户相关的路由
func (m *Module) RegisterRoutes(group interface{}) {
	// 这里可以根据不同的路由器实现来注册路由
	// 由于我们现在使用gin，所以期望传入*gin.RouterGroup
	// 但为了保持模块的独立性，这里使用interface{}

	// 实际的路由注册可能在外部进行，这里只是提供一个示例接口
}

// Cleanup 清理模块资源
func (m *Module) Cleanup() error {
	// 如果有需要清理的资源，在这里进行清理
	// 比如关闭数据库连接、释放缓存等
	return nil
}

// ModuleInfo 返回模块信息
func (m *Module) ModuleInfo() map[string]interface{} {
	return map[string]interface{}{
		"name":        "user",
		"version":     "1.0.0",
		"description": "用户管理模块",
		"components": map[string]string{
			"repository": "user.Repository",
			"service":    "user.Service",
			"handler":    "user.Handler",
		},
	}
}
