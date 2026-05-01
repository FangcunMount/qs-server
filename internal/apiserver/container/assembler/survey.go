package assembler

import (
	"go.mongodb.org/mongo-driver/mongo"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	asApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	cacheInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	ruleengineInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleengine"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
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
	topicResolver  eventcatalog.TopicResolver
}

// SurveyModuleDeps 定义 Survey 模块的显式构造依赖。
type SurveyModuleDeps struct {
	MongoDB             *mongo.Database
	EventPublisher      event.EventPublisher
	RankRedisClient     redis.UniversalClient
	RankCacheBuilder    *keyspace.Builder
	IdentityService     *iam.IdentityService
	HotsetRecorder      cachetarget.HotsetRecorder
	TopicResolver       eventcatalog.TopicResolver
	ScaleSyncer         quesApp.ScaleQuestionnaireBindingSyncer
	QuestionnaireRepo   questionnaire.Repository
	QuestionnaireReader surveyreadmodel.QuestionnaireReader
	AnswerSheetRepo     AnswerSheetStore
	AnswerSheetReader   surveyreadmodel.AnswerSheetReader
}

type AnswerSheetStore interface {
	answersheet.Repository
	asApp.SubmissionDurableWriter
	asApp.EventStager
	asApp.SubmittedEventOutboxStore
	appEventing.OutboxStatusReader
}

// QuestionnaireSubModule 问卷子模块
type QuestionnaireSubModule struct {
	// service 层 - 按行为者组织
	LifecycleService quesApp.QuestionnaireLifecycleService
	ContentService   quesApp.QuestionnaireContentService
	QueryService     quesApp.QuestionnaireQueryService
}

// AnswerSheetSubModule 答卷子模块
type AnswerSheetSubModule struct {
	// service 层 - 按行为者组织
	SubmissionService          asApp.AnswerSheetSubmissionService
	ManagementService          asApp.AnswerSheetManagementService
	ScoringService             asApp.AnswerSheetScoringService // 新增：计分服务
	SubmittedEventRelay        asApp.SubmittedEventRelay
	SubmittedEventStatusReader appEventing.NamedOutboxStatusReader
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
	module.topicResolver = normalized.TopicResolver

	// 初始化问卷子模块
	if err := module.initQuestionnaireSubModule(
		normalized.IdentityService,
		normalized.HotsetRecorder,
		normalized.ScaleSyncer,
		normalized.QuestionnaireRepo,
		normalized.QuestionnaireReader,
	); err != nil {
		return nil, err
	}

	// 初始化答卷子模块
	if err := module.initAnswerSheetSubModule(normalized.MongoDB, normalized.RankRedisClient, normalized.RankCacheBuilder, normalized.AnswerSheetRepo, normalized.AnswerSheetReader, normalized.QuestionnaireRepo); err != nil {
		return nil, err
	}

	return module, nil
}

func normalizeSurveyModuleDeps(deps SurveyModuleDeps) (SurveyModuleDeps, error) {
	if deps.MongoDB == nil {
		return SurveyModuleDeps{}, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}
	if deps.QuestionnaireRepo == nil || deps.QuestionnaireReader == nil || deps.AnswerSheetRepo == nil || deps.AnswerSheetReader == nil {
		return SurveyModuleDeps{}, errors.WithCode(code.ErrModuleInitializationFailed, "survey repositories and read models are required")
	}
	if deps.EventPublisher == nil {
		deps.EventPublisher = event.NewNopEventPublisher()
	}
	return deps, nil
}

// initQuestionnaireSubModule 初始化问卷子模块
func (m *SurveyModule) initQuestionnaireSubModule(identitySvc *iam.IdentityService, hotset cachetarget.HotsetRecorder, scaleSyncer quesApp.ScaleQuestionnaireBindingSyncer, repo questionnaire.Repository, reader surveyreadmodel.QuestionnaireReader) error {
	sub := m.Questionnaire

	// 初始化领域服务
	validator := questionnaire.Validator{}
	lifecycle := questionnaire.NewLifecycle()
	questionMgr := questionnaire.QuestionManager{}

	// 初始化 service 层 - 按行为者组织的服务（使用模块统一的事件发布器）
	sub.LifecycleService = quesApp.NewLifecycleService(repo, scaleSyncer, validator, lifecycle, m.eventPublisher)
	sub.ContentService = quesApp.NewContentService(repo, questionMgr)
	sub.QueryService = quesApp.NewQueryService(repo, identitySvc, hotset, reader)

	return nil
}

// initAnswerSheetSubModule 初始化答卷子模块
func (m *SurveyModule) initAnswerSheetSubModule(mongoDB *mongo.Database, rankRedisClient redis.UniversalClient, rankCacheBuilder *keyspace.Builder, repo AnswerSheetStore, reader surveyreadmodel.AnswerSheetReader, questionnaireRepo questionnaire.Repository) error {
	sub := m.AnswerSheet

	// 创建答案校验引擎 adapter
	batchValidator := ruleengineInfra.NewAnswerValidator()

	// 创建领域服务
	answerScorer := ruleengineInfra.NewAnswerScorer()

	// 初始化 service 层 - 按行为者组织的服务（使用模块统一的事件发布器）
	mongoTxRunner := newMongoTransactionRunner(mongoDB)
	durableStore := asApp.NewTransactionalSubmissionDurableStore(mongoTxRunner, repo, repo)
	sub.SubmissionService = asApp.NewSubmissionService(repo, durableStore, questionnaireRepo, batchValidator, reader)
	sub.ManagementService = asApp.NewManagementService(repo, reader)
	sub.ScoringService = asApp.NewAnswerSheetScoringService(repo, questionnaireRepo, answerScorer)
	hotRankProjection := cacheInfra.NewRedisScaleHotRankProjection(rankRedisClient, rankCacheBuilder)
	sub.SubmittedEventRelay = appEventing.NewDurableOutboxRelayWithHooks(
		"mongo-domain-events",
		repo,
		m.eventPublisher,
		scaleApp.NewScaleHotRankProjectionHook(hotRankProjection),
	)
	sub.SubmittedEventStatusReader = appEventing.NamedOutboxStatusReader{
		Name:   "mongo-domain-events",
		Reader: repo,
	}

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
