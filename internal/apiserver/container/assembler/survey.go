package assembler

import (
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/FangcunMount/component-base/pkg/errors"
	asApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/validation"
	asMongoInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	quesMongoInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// SurveyModule Survey 模块（问卷&答卷）
// 按照 DDD 限界上下文组织，Survey 是一个完整的子域
type SurveyModule struct {
	// Questionnaire 子模块
	Questionnaire *QuestionnaireSubModule

	// AnswerSheet 子模块
	AnswerSheet *AnswerSheetSubModule
}

// QuestionnaireSubModule 问卷子模块
type QuestionnaireSubModule struct {
	// repository 层
	Repo questionnaire.Repository

	// handler 层
	Handler *handler.QuestionnaireHandler

	// service 层 - 按行为者组织
	LifecycleService quesApp.QuestionnaireLifecycleService
	ContentService   quesApp.QuestionnaireContentService
	QueryService     quesApp.QuestionnaireQueryService
}

// AnswerSheetSubModule 答卷子模块
type AnswerSheetSubModule struct {
	// repository 层
	Repo answersheet.Repository

	// handler 层
	Handler *handler.AnswerSheetHandler

	// service 层 - 按行为者组织
	SubmissionService asApp.AnswerSheetSubmissionService
	ManagementService asApp.AnswerSheetManagementService
}

// NewSurveyModule 创建 Survey 模块
func NewSurveyModule() *SurveyModule {
	return &SurveyModule{
		Questionnaire: &QuestionnaireSubModule{},
		AnswerSheet:   &AnswerSheetSubModule{},
	}
}

// Initialize 初始化 Survey 模块
func (m *SurveyModule) Initialize(params ...interface{}) error {
	mongoDB := params[0].(*mongo.Database)
	if mongoDB == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	// 初始化问卷子模块
	if err := m.initQuestionnaireSubModule(mongoDB); err != nil {
		return err
	}

	// 初始化答卷子模块
	if err := m.initAnswerSheetSubModule(mongoDB); err != nil {
		return err
	}

	return nil
}

// initQuestionnaireSubModule 初始化问卷子模块
func (m *SurveyModule) initQuestionnaireSubModule(mongoDB *mongo.Database) error {
	sub := m.Questionnaire

	// 初始化 repository 层
	sub.Repo = quesMongoInfra.NewRepository(mongoDB)

	// 初始化领域服务
	validator := questionnaire.Validator{}
	lifecycle := questionnaire.NewLifecycle()
	questionMgr := questionnaire.QuestionManager{}

	// 初始化事件发布器（当前使用空实现，后续可注入实际实现）
	eventPublisher := event.NewNopEventPublisher()

	// 初始化 service 层 - 按行为者组织的服务
	sub.LifecycleService = quesApp.NewLifecycleService(sub.Repo, validator, lifecycle, eventPublisher)
	sub.ContentService = quesApp.NewContentService(sub.Repo, questionMgr)
	sub.QueryService = quesApp.NewQueryService(sub.Repo)

	// 初始化 handler 层
	sub.Handler = handler.NewQuestionnaireHandler(
		sub.LifecycleService,
		sub.ContentService,
		sub.QueryService,
	)

	return nil
}

// initAnswerSheetSubModule 初始化答卷子模块
func (m *SurveyModule) initAnswerSheetSubModule(mongoDB *mongo.Database) error {
	sub := m.AnswerSheet

	// 初始化 repository 层
	sub.Repo = asMongoInfra.NewRepository(mongoDB)

	// 获取问卷仓储（答卷服务需要依赖问卷仓储进行验证）
	quesRepo := m.Questionnaire.Repo

	// 创建验证器
	validator := validation.NewDefaultValidator()

	// 初始化 service 层 - 按行为者组织的服务
	sub.SubmissionService = asApp.NewSubmissionService(sub.Repo, quesRepo, validator)
	sub.ManagementService = asApp.NewManagementService(sub.Repo)

	// 初始化 handler 层
	sub.Handler = handler.NewAnswerSheetHandler(
		sub.ManagementService,
	)

	return nil
}

// Cleanup 清理模块资源
func (m *SurveyModule) Cleanup() error {
	// 如果有需要清理的资源，在这里进行清理
	return nil
}

// CheckHealth 检查模块健康状态
func (m *SurveyModule) CheckHealth() error {
	return nil
}

// ModuleInfo 返回模块信息
func (m *SurveyModule) ModuleInfo() ModuleInfo {
	return ModuleInfo{
		Name:        "survey",
		Version:     "1.0.0",
		Description: "问卷量表模块（包含问卷和答卷子模块）",
	}
}
