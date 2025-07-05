package assembler

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet/port"
	"github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"

	qnMongoInfra "github.com/yshujie/questionnaire-scale/internal/apiserver/infrastructure/mongo/questionnaire"

	asApp "github.com/yshujie/questionnaire-scale/internal/apiserver/application/answersheet"
	asMongoInfra "github.com/yshujie/questionnaire-scale/internal/apiserver/infrastructure/mongo/answersheet"
	asHandler "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/handler"
)

// AnswersheetModule 答卷模块
type AnswersheetModule struct {
	// repository 层
	AnswersheetRepo port.AnswerSheetRepositoryMongo

	// handler 层
	AnswersheetHandler *asHandler.AnswerSheetHandler

	// service 层
	AnswersheetSaver   port.AnswerSheetSaver
	AnswersheetQueryer port.AnswerSheetQueryer
}

// NewAnswersheetModule 创建答卷模块
func NewAnswersheetModule() *AnswersheetModule {
	return &AnswersheetModule{}
}

// Initialize 初始化模块
func (m *AnswersheetModule) Initialize(params ...interface{}) error {
	mongoDB := params[0].(*mongo.Database)
	if mongoDB == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	// 初始化 repository 层
	m.AnswersheetRepo = asMongoInfra.NewRepository(mongoDB)

	// 初始化 service 层
	m.AnswersheetSaver = asApp.NewSaver(m.AnswersheetRepo)
	m.AnswersheetQueryer = asApp.NewQueryer(m.AnswersheetRepo, qnMongoInfra.NewRepository(mongoDB))

	// 初始化 handler 层
	m.AnswersheetHandler = asHandler.NewAnswerSheetHandler(m.AnswersheetSaver, m.AnswersheetQueryer)

	return nil
}

// Cleanup 清理模块资源
func (m *AnswersheetModule) Cleanup() error {
	// 如果有需要清理的资源，在这里进行清理
	// 比如关闭数据库连接、释放缓存等
	return nil
}

// CheckHealth 检查模块健康状态
func (m *AnswersheetModule) CheckHealth() error {
	return nil
}

// ModuleInfo 返回模块信息
func (m *AnswersheetModule) ModuleInfo() ModuleInfo {
	return ModuleInfo{
		Name:        "answersheet",
		Version:     "1.0.0",
		Description: "答卷管理模块",
	}
}
