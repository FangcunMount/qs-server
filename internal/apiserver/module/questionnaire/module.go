package user

import (
	"gorm.io/gorm"

	quesInfra "github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/driven/mysql/questionnaire"
	quesAdapter "github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/driving/restful/questionnaire"
	quesApp "github.com/yshujie/questionnaire-scale/internal/apiserver/application/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/module"
	"github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// Module 问卷模块
type Module struct {
	// repository 层
	QuestionnaireRepo port.QuestionnaireRepository

	// handler 层
	QuestionnaireHandler *quesAdapter.Handler

	// service 层
	QuestionnaireCreator   port.QuestionnaireCreator
	QuestionnaireEditor    port.QuestionnaireEditor
	QuestionnairePublisher port.QuestionnairePublisher
	QuestionnaireQueryer   port.QuestionnaireQueryer
}

// NewModule 创建用户模块
func NewModule() *Module {
	return &Module{}
}

// Initialize 初始化模块
func (m *Module) Initialize(params ...interface{}) error {
	db := params[0].(*gorm.DB)
	if db == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	// 初始化 repository 层
	m.QuestionnaireRepo = quesInfra.NewRepository(db)

	// 初始化 service 层
	m.QuestionnaireCreator = quesApp.NewCreator(m.QuestionnaireRepo)
	m.QuestionnaireEditor = quesApp.NewEditor(m.QuestionnaireRepo)
	m.QuestionnairePublisher = quesApp.NewPublisher(m.QuestionnaireRepo)
	m.QuestionnaireQueryer = quesApp.NewQueryer(m.QuestionnaireRepo)

	// 初始化 handler 层
	m.QuestionnaireHandler = quesAdapter.NewHandler(
		m.QuestionnaireCreator,
		m.QuestionnaireEditor,
		m.QuestionnairePublisher,
		m.QuestionnaireQueryer,
	)

	return nil
}

// Cleanup 清理模块资源
func (m *Module) Cleanup() error {
	// 如果有需要清理的资源，在这里进行清理
	// 比如关闭数据库连接、释放缓存等
	return nil
}

// CheckHealth 检查模块健康状态
func (m *Module) CheckHealth() error {
	return nil
}

// ModuleInfo 返回模块信息
func (m *Module) ModuleInfo() module.ModuleInfo {
	return module.ModuleInfo{
		Name:        "questionnaire",
		Version:     "1.0.0",
		Description: "问卷管理模块",
	}
}
