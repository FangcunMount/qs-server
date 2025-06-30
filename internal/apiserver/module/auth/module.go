package auth

import (
	"gorm.io/gorm"

	userInfra "github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/driven/mysql/user"
	authApp "github.com/yshujie/questionnaire-scale/internal/apiserver/application/auth"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/module"
	"github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// Module 用户模块
// 负责组装用户相关的所有组件
type Module struct {
	// 应用层
	Authenticator port.Authenticator
}

// NewModule 创建模块
func NewModule() *Module {
	return &Module{}
}

// Initialize 初始化模块
func (m *Module) Initialize(params ...interface{}) error {
	db := params[0].(*gorm.DB)
	if db == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	// 构造应用层 - Service
	m.Authenticator = authApp.NewAuthenticator(
		userInfra.NewRepository(db),
	)

	return nil
}

// CheckHealth 检查模块健康状态
func (m *Module) CheckHealth() error {
	return nil
}

// Cleanup 清理模块资源
func (m *Module) Cleanup() error {
	return nil
}

// ModuleInfo 返回模块信息
func (m *Module) ModuleInfo() module.ModuleInfo {
	return module.ModuleInfo{
		Name:        "auth",
		Version:     "1.0.0",
		Description: "认证模块",
	}
}
