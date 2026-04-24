package assembler

import (
	"go.mongodb.org/mongo-driver/mongo"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	asApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/validation"
	questionnaireCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	asMongoInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	quesMongoInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// SurveyModule Survey 模块（问卷&答卷）
// 按照 DDD 限界上下文组织，Survey 是一个完整的子域
type SurveyModule struct {
	// Questionnaire 子模块
	Questionnaire *QuestionnaireSubModule

	// AnswerSheet 子模块
	AnswerSheet *AnswerSheetSubModule

	// 事件发布器（由容器统一注入）
	eventPublisher event.EventPublisher
}

// SurveyModuleDeps 定义 Survey 模块的显式构造依赖。
type SurveyModuleDeps struct {
	MongoDB             *mongo.Database
	EventPublisher      event.EventPublisher
	RedisClient         redis.UniversalClient
	CacheBuilder        *rediskey.Builder
	IdentityService     *iam.IdentityService
	QuestionnairePolicy cachepolicy.CachePolicy
	HotsetRecorder      cachetarget.HotsetRecorder
	Observer            *cacheobservability.ComponentObserver
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
	SubmissionService   asApp.AnswerSheetSubmissionService
	ManagementService   asApp.AnswerSheetManagementService
	ScoringService      asApp.AnswerSheetScoringService // 新增：计分服务
	SubmittedEventRelay asApp.SubmittedEventRelay
}

// NewSurveyModule 创建 Survey 模块。
func NewSurveyModule(deps SurveyModuleDeps) (*SurveyModule, error) {
	normalized, err := normalizeSurveyModuleDeps(deps)
	if err != nil {
		return nil, err
	}

	module := &SurveyModule{
		Questionnaire: &QuestionnaireSubModule{},
		AnswerSheet:   &AnswerSheetSubModule{},
	}

	module.eventPublisher = normalized.EventPublisher

	// 初始化问卷子模块
	if err := module.initQuestionnaireSubModule(
		normalized.MongoDB,
		normalized.RedisClient,
		normalized.CacheBuilder,
		normalized.IdentityService,
		normalized.QuestionnairePolicy,
		normalized.HotsetRecorder,
		normalized.Observer,
	); err != nil {
		return nil, err
	}

	// 初始化答卷子模块
	if err := module.initAnswerSheetSubModule(normalized.MongoDB); err != nil {
		return nil, err
	}

	return module, nil
}

func normalizeSurveyModuleDeps(deps SurveyModuleDeps) (SurveyModuleDeps, error) {
	if deps.MongoDB == nil {
		return SurveyModuleDeps{}, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}
	if deps.EventPublisher == nil {
		deps.EventPublisher = event.NewNopEventPublisher()
	}
	return deps, nil
}

// initQuestionnaireSubModule 初始化问卷子模块
func (m *SurveyModule) initQuestionnaireSubModule(mongoDB *mongo.Database, redisClient redis.UniversalClient, cacheBuilder *rediskey.Builder, identitySvc *iam.IdentityService, policy cachepolicy.CachePolicy, hotset cachetarget.HotsetRecorder, observer *cacheobservability.ComponentObserver) error {
	sub := m.Questionnaire

	// 初始化 repository 层（基础实现）
	baseRepo := quesMongoInfra.NewRepository(mongoDB)
	// 如果提供了 Redis 客户端，使用缓存装饰器
	if redisClient != nil {
		sub.Repo = questionnaireCache.NewCachedQuestionnaireRepositoryWithBuilderPolicyAndObserver(baseRepo, redisClient, cacheBuilder, policy, observer)
	} else {
		sub.Repo = baseRepo
	}

	// 初始化领域服务
	validator := questionnaire.Validator{}
	lifecycle := questionnaire.NewLifecycle()
	questionMgr := questionnaire.QuestionManager{}

	// 初始化 service 层 - 按行为者组织的服务（使用模块统一的事件发布器）
	sub.LifecycleService = quesApp.NewLifecycleService(sub.Repo, nil, validator, lifecycle, m.eventPublisher)
	sub.ContentService = quesApp.NewContentService(sub.Repo, questionMgr)
	sub.QueryService = quesApp.NewQueryService(sub.Repo, identitySvc, hotset)

	// 初始化 handler 层
	// 注意：QRCodeService 在容器初始化后才创建，需要通过 SetQRCodeService 方法单独设置
	sub.Handler = handler.NewQuestionnaireHandler(
		sub.LifecycleService,
		sub.ContentService,
		sub.QueryService,
		nil, // QRCodeService 稍后通过 SetQRCodeService 设置
	)

	return nil
}

// SetScaleRepository 设置量表仓储，用于问卷发布时同步量表问卷版本。
func (m *SurveyModule) SetScaleRepository(scaleRepo domainScale.Repository) {
	if m == nil || m.Questionnaire == nil {
		return
	}

	validator := questionnaire.Validator{}
	lifecycle := questionnaire.NewLifecycle()
	m.Questionnaire.LifecycleService = quesApp.NewLifecycleService(
		m.Questionnaire.Repo,
		scaleRepo,
		validator,
		lifecycle,
		m.eventPublisher,
	)

	if m.Questionnaire.Handler != nil {
		m.Questionnaire.Handler = handler.NewQuestionnaireHandler(
			m.Questionnaire.LifecycleService,
			m.Questionnaire.ContentService,
			m.Questionnaire.QueryService,
			nil,
		)
	}
}

// SetQRCodeService 设置二维码服务（用于跨模块依赖注入）
func (m *SurveyModule) SetQRCodeService(qrCodeService qrcodeApp.QRCodeService) {
	if m.Questionnaire != nil && m.Questionnaire.Handler != nil {
		// 重新创建 Handler，传入 QRCodeService
		m.Questionnaire.Handler = handler.NewQuestionnaireHandler(
			m.Questionnaire.LifecycleService,
			m.Questionnaire.ContentService,
			m.Questionnaire.QueryService,
			qrCodeService,
		)
	}
}

// initAnswerSheetSubModule 初始化答卷子模块
func (m *SurveyModule) initAnswerSheetSubModule(mongoDB *mongo.Database) error {
	sub := m.AnswerSheet

	// 初始化 repository 层
	baseRepo, err := asMongoInfra.NewRepository(mongoDB)
	if err != nil {
		return err
	}
	sub.Repo = baseRepo

	// 获取问卷仓储（答卷服务需要依赖问卷仓储进行验证）
	quesRepo := m.Questionnaire.Repo

	// 创建批量验证器
	batchValidator := validation.NewBatchValidator()

	// 创建领域服务
	scoringDomainService := answersheet.NewScoringService()

	// 初始化 service 层 - 按行为者组织的服务（使用模块统一的事件发布器）
	sub.SubmissionService = asApp.NewSubmissionService(sub.Repo, baseRepo, quesRepo, batchValidator)
	sub.ManagementService = asApp.NewManagementService(sub.Repo)
	sub.ScoringService = asApp.NewAnswerSheetScoringService(sub.Repo, quesRepo, scoringDomainService)
	sub.SubmittedEventRelay = appEventing.NewOutboxRelay("mongo-domain-events", baseRepo, m.eventPublisher)

	// 初始化 handler 层
	sub.Handler = handler.NewAnswerSheetHandler(
		sub.ManagementService,
		sub.SubmissionService,
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
