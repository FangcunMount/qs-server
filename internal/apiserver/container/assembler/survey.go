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
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	asMongoInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	quesMongoInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	ruleengineInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleengine"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
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
	RedisClient         redis.UniversalClient
	CacheBuilder        *keyspace.Builder
	RankRedisClient     redis.UniversalClient
	RankCacheBuilder    *keyspace.Builder
	IdentityService     *iam.IdentityService
	QuestionnairePolicy cachepolicy.CachePolicy
	HotsetRecorder      cachetarget.HotsetRecorder
	Observer            *observability.ComponentObserver
	TopicResolver       eventcatalog.TopicResolver
	MongoLimiter        backpressure.Acquirer
	ScaleSyncer         quesApp.ScaleQuestionnaireBindingSyncer
}

// QuestionnaireSubModule 问卷子模块
type QuestionnaireSubModule struct {
	// repository 层
	Repo questionnaire.Repository

	// service 层 - 按行为者组织
	LifecycleService quesApp.QuestionnaireLifecycleService
	ContentService   quesApp.QuestionnaireContentService
	QueryService     quesApp.QuestionnaireQueryService
	Reader           surveyreadmodel.QuestionnaireReader
}

// AnswerSheetSubModule 答卷子模块
type AnswerSheetSubModule struct {
	// repository 层
	Repo answersheet.Repository

	// service 层 - 按行为者组织
	SubmissionService          asApp.AnswerSheetSubmissionService
	ManagementService          asApp.AnswerSheetManagementService
	ScoringService             asApp.AnswerSheetScoringService // 新增：计分服务
	Reader                     surveyreadmodel.AnswerSheetReader
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
		normalized.MongoDB,
		normalized.RedisClient,
		normalized.CacheBuilder,
		normalized.IdentityService,
		normalized.QuestionnairePolicy,
		normalized.HotsetRecorder,
		normalized.Observer,
		normalized.MongoLimiter,
		normalized.ScaleSyncer,
	); err != nil {
		return nil, err
	}

	// 初始化答卷子模块
	if err := module.initAnswerSheetSubModule(normalized.MongoDB, normalized.RankRedisClient, normalized.RankCacheBuilder, normalized.MongoLimiter); err != nil {
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
func (m *SurveyModule) initQuestionnaireSubModule(mongoDB *mongo.Database, redisClient redis.UniversalClient, cacheBuilder *keyspace.Builder, identitySvc *iam.IdentityService, policy cachepolicy.CachePolicy, hotset cachetarget.HotsetRecorder, observer *observability.ComponentObserver, limiter backpressure.Acquirer, scaleSyncer quesApp.ScaleQuestionnaireBindingSyncer) error {
	sub := m.Questionnaire

	// 初始化 repository 层（基础实现）
	baseRepo := quesMongoInfra.NewRepository(mongoDB, mongoBase.BaseRepositoryOptions{Limiter: limiter})
	sub.Reader = quesMongoInfra.NewQuestionnaireReadModel(baseRepo)
	// 如果提供了 Redis 客户端，使用缓存装饰器
	if redisClient != nil {
		sub.Repo = cacheInfra.NewCachedQuestionnaireRepositoryWithBuilderPolicyAndObserver(baseRepo, redisClient, cacheBuilder, policy, observer)
	} else {
		sub.Repo = baseRepo
	}

	// 初始化领域服务
	validator := questionnaire.Validator{}
	lifecycle := questionnaire.NewLifecycle()
	questionMgr := questionnaire.QuestionManager{}

	// 初始化 service 层 - 按行为者组织的服务（使用模块统一的事件发布器）
	sub.LifecycleService = quesApp.NewLifecycleService(sub.Repo, scaleSyncer, validator, lifecycle, m.eventPublisher)
	sub.ContentService = quesApp.NewContentService(sub.Repo, questionMgr)
	sub.QueryService = quesApp.NewQueryService(sub.Repo, identitySvc, hotset, sub.Reader)

	return nil
}

// initAnswerSheetSubModule 初始化答卷子模块
func (m *SurveyModule) initAnswerSheetSubModule(mongoDB *mongo.Database, rankRedisClient redis.UniversalClient, rankCacheBuilder *keyspace.Builder, limiter backpressure.Acquirer) error {
	sub := m.AnswerSheet

	// 初始化 repository 层
	baseRepo, err := asMongoInfra.NewRepositoryWithTopicResolver(mongoDB, m.topicResolver, mongoBase.BaseRepositoryOptions{Limiter: limiter})
	if err != nil {
		return err
	}
	sub.Repo = baseRepo

	// 获取问卷仓储（答卷服务需要依赖问卷仓储进行验证）
	quesRepo := m.Questionnaire.Repo

	// 创建答案校验引擎 adapter
	batchValidator := ruleengineInfra.NewAnswerValidator()

	// 创建领域服务
	answerScorer := ruleengineInfra.NewAnswerScorer()

	// 初始化 service 层 - 按行为者组织的服务（使用模块统一的事件发布器）
	mongoTxRunner := newMongoTransactionRunner(mongoDB)
	durableStore := asApp.NewTransactionalSubmissionDurableStore(mongoTxRunner, baseRepo, baseRepo)
	sub.Reader = asMongoInfra.NewAnswerSheetReadModel(baseRepo)
	sub.SubmissionService = asApp.NewSubmissionService(sub.Repo, durableStore, quesRepo, batchValidator, sub.Reader)
	sub.ManagementService = asApp.NewManagementService(sub.Repo, sub.Reader)
	sub.ScoringService = asApp.NewAnswerSheetScoringService(sub.Repo, quesRepo, answerScorer)
	hotRankProjection := cacheInfra.NewRedisScaleHotRankProjection(rankRedisClient, rankCacheBuilder)
	sub.SubmittedEventRelay = appEventing.NewDurableOutboxRelayWithHooks(
		"mongo-domain-events",
		baseRepo,
		m.eventPublisher,
		scaleApp.NewScaleHotRankProjectionHook(hotRankProjection),
	)
	sub.SubmittedEventStatusReader = appEventing.NamedOutboxStatusReader{
		Name:   "mongo-domain-events",
		Reader: baseRepo,
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
