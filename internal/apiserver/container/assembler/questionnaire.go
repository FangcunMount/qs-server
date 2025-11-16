package assembler

import (
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/FangcunMount/component-base/pkg/errors"
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/questionnaire/port"
	quesDocInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Module 问卷模块
type QuestionnaireModule struct {
	// repository 层
	QuesRepo port.QuestionnaireRepositoryMongo

	// handler 层
	QuesHandler *handler.QuestionnaireHandler

	// service 层
	QuesCreator   port.QuestionnaireCreator
	QuesEditor    port.QuestionnaireEditor
	QuesPublisher port.QuestionnairePublisher
	QuesQueryer   port.QuestionnaireQueryer
}

// NewModule 创建用户模块
func NewQuestionnaireModule() *QuestionnaireModule {
	return &QuestionnaireModule{}
}

// Initialize 初始化模块
func (m *QuestionnaireModule) Initialize(params ...interface{}) error {
	mongoDB := params[0].(*mongo.Database)
	if mongoDB == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	// 初始化 repository 层
	m.QuesRepo = quesDocInfra.NewRepository(mongoDB)

	// 初始化 service 层
	m.QuesCreator = quesApp.NewCreator(m.QuesRepo)
	m.QuesEditor = quesApp.NewEditor(m.QuesRepo)
	m.QuesPublisher = quesApp.NewPublisher(m.QuesRepo)
	m.QuesQueryer = quesApp.NewQueryer(m.QuesRepo)

	// 初始化 handler 层
	m.QuesHandler = handler.NewQuestionnaireHandler(
		m.QuesCreator,
		m.QuesEditor,
		m.QuesPublisher,
		m.QuesQueryer,
	)

	return nil
}

// Cleanup 清理模块资源
func (m *QuestionnaireModule) Cleanup() error {
	// 如果有需要清理的资源，在这里进行清理
	// 比如关闭数据库连接、释放缓存等
	return nil
}

// CheckHealth 检查模块健康状态
func (m *QuestionnaireModule) CheckHealth() error {
	return nil
}

// ModuleInfo 返回模块信息
func (m *QuestionnaireModule) ModuleInfo() ModuleInfo {
	return ModuleInfo{
		Name:        "questionnaire",
		Version:     "1.0.0",
		Description: "问卷管理模块",
	}
}
