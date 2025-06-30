package user

import (
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	quesDocInfra "github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/driven/mongo/questionnaire"
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
	QuesRepo port.QuestionnaireRepository
	QuesDoc  port.QuestionnaireDocument

	// handler 层
	QuesHandler *quesAdapter.Handler

	// service 层
	QuesCreator   port.QuestionnaireCreator
	QuesEditor    port.QuestionnaireEditor
	QuesPublisher port.QuestionnairePublisher
	QuesQueryer   port.QuestionnaireQueryer
}

// NewModule 创建用户模块
func NewModule() *Module {
	return &Module{}
}

// Initialize 初始化模块
func (m *Module) Initialize(params ...interface{}) error {
	mysqlDB := params[0].(*gorm.DB)
	mongoDB := params[1].(*mongo.Database)
	if mysqlDB == nil || mongoDB == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	// 初始化 repository 层
	m.QuesRepo = quesInfra.NewRepository(mysqlDB)
	m.QuesDoc = quesDocInfra.NewRepository(mongoDB).(port.QuestionnaireDocument)

	// 初始化 service 层
	m.QuesCreator = quesApp.NewCreator(m.QuesRepo, m.QuesDoc)
	m.QuesEditor = quesApp.NewEditor(m.QuesRepo, m.QuesDoc)
	m.QuesPublisher = quesApp.NewPublisher(m.QuesRepo, m.QuesDoc)
	m.QuesQueryer = quesApp.NewQueryer(m.QuesRepo, m.QuesDoc)

	// 初始化 handler 层
	m.QuesHandler = quesAdapter.NewHandler(
		m.QuesCreator,
		m.QuesEditor,
		m.QuesPublisher,
		m.QuesQueryer,
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
