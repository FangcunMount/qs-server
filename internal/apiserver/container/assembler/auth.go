package assembler

import (
	"gorm.io/gorm"

	authApp "github.com/fangcun-mount/qs-server/internal/apiserver/application/auth"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/port"
	userInfra "github.com/fangcun-mount/qs-server/internal/apiserver/infrastructure/mysql/user"
	"github.com/fangcun-mount/qs-server/internal/pkg/code"
	"github.com/fangcun-mount/qs-server/pkg/errors"
)

// Module 认证模块
// 负责组装用户相关的所有组件
type AuthModule struct {
	// repository 层
	UserRepo port.UserRepository

	// service 层 - 使用接口类型而非具体类型
	Authenticator port.Authenticator
}

// NewModule 创建认证模块
func NewAuthModule() *AuthModule {
	return &AuthModule{}
}

// Initialize 初始化模块
func (m *AuthModule) Initialize(params ...interface{}) error {
	db := params[0].(*gorm.DB)
	if db == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	// 初始化 repository 层
	m.UserRepo = userInfra.NewRepository(db)

	// 初始化 service 层
	m.Authenticator = authApp.NewAuthenticator(m.UserRepo)

	return nil
}

// CheckHealth 检查模块健康状态
func (m *AuthModule) CheckHealth() error {
	return nil
}

// Cleanup 清理模块资源
func (m *AuthModule) Cleanup() error {
	return nil
}

// ModuleInfo 返回模块信息
func (m *AuthModule) ModuleInfo() ModuleInfo {
	return ModuleInfo{
		Name:        "auth",
		Version:     "1.0.0",
		Description: "认证模块",
	}
}
