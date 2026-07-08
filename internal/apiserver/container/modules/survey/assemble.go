package survey

import (
	"go.mongodb.org/mongo-driver/mongo"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring"
	asApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/internal/outboxruntime"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	cacheInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/redis/outboxready"
	ruleengineInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleengine"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// Module assembles survey application services.
type Module struct {
	Questionnaire *QuestionnaireSubModule
	AnswerSheet   *AnswerSheetSubModule

	eventPublisher event.EventPublisher
	topicResolver  eventcatalog.TopicResolver
}

// Deps defines explicit constructor dependencies for the survey module.
type Deps struct {
	MongoDB                           *mongo.Database
	EventPublisher                    event.EventPublisher
	RankRedisClient                   redis.UniversalClient
	RankCacheBuilder                  *keyspace.Builder
	IdentityService                   *iam.IdentityService
	HotsetRecorder                    cachetarget.HotsetRecorder
	TopicResolver                     eventcatalog.TopicResolver
	ScaleSyncer                       quesApp.ScaleQuestionnaireBindingSyncer
	QuestionnaireRepo                 questionnaire.Repository
	QuestionnaireReader               surveyreadmodel.QuestionnaireReader
	AnswerSheetRepo                   AnswerSheetStore
	AnswerSheetReader                 surveyreadmodel.AnswerSheetReader
	OutboxRelayBatchSize              int
	OutboxRelayPublishWorkers         int
	OutboxRelayImmediateMaxConcurrent int
	CacheSignalNotifier               quesApp.CacheSignalNotifier
	OpsHandle                         *cacheplane.Handle
}

// AnswerSheetStore combines answer-sheet persistence and outbox ports.
type AnswerSheetStore interface {
	answersheet.Repository
	asApp.SubmissionDurableWriter
	asApp.EventStager
	asApp.SubmittedEventOutboxStore
	appEventing.OutboxStatusReader
}

// QuestionnaireSubModule hosts questionnaire application services.
type QuestionnaireSubModule struct {
	LifecycleService quesApp.QuestionnaireLifecycleService
	ContentService   quesApp.QuestionnaireContentService
	QueryService     quesApp.QuestionnaireQueryService
}

// AnswerSheetSubModule hosts answer-sheet application services.
type AnswerSheetSubModule struct {
	SubmissionService          asApp.AnswerSheetSubmissionService
	ManagementService          asApp.AnswerSheetManagementService
	ScoringService             asApp.AnswerSheetScoringService
	SubmittedEventRelay        asApp.SubmittedEventRelay
	SubmittedEventStatusReader appEventing.NamedOutboxStatusReader
	OutboxReadyIndex           *outboxready.Index
}

// New assembles the survey module.
func New(deps Deps) (*Module, error) {
	normalized, err := normalizeDeps(deps)
	if err != nil {
		return nil, err
	}

	module := &Module{
		Questionnaire: &QuestionnaireSubModule{},
		AnswerSheet:   &AnswerSheetSubModule{},
	}

	module.eventPublisher = normalized.EventPublisher
	module.topicResolver = normalized.TopicResolver

	if err := module.initQuestionnaireSubModule(
		normalized.IdentityService,
		normalized.HotsetRecorder,
		normalized.ScaleSyncer,
		normalized.QuestionnaireRepo,
		normalized.QuestionnaireReader,
		normalized.CacheSignalNotifier,
	); err != nil {
		return nil, err
	}

	if err := module.initAnswerSheetSubModule(
		normalized.MongoDB,
		normalized.RankRedisClient,
		normalized.RankCacheBuilder,
		normalized.AnswerSheetRepo,
		normalized.AnswerSheetReader,
		normalized.QuestionnaireRepo,
		normalized.OutboxRelayBatchSize,
		normalized.OutboxRelayPublishWorkers,
		normalized.OutboxRelayImmediateMaxConcurrent,
		normalized.OpsHandle,
	); err != nil {
		return nil, err
	}

	return module, nil
}

func normalizeDeps(deps Deps) (Deps, error) {
	if deps.MongoDB == nil {
		return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}
	if deps.QuestionnaireRepo == nil || deps.QuestionnaireReader == nil || deps.AnswerSheetRepo == nil || deps.AnswerSheetReader == nil {
		return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "survey repositories and read models are required")
	}
	if deps.EventPublisher == nil {
		deps.EventPublisher = event.NewNopEventPublisher()
	}
	return deps, nil
}

func (m *Module) initQuestionnaireSubModule(identitySvc *iam.IdentityService, hotset cachetarget.HotsetRecorder, scaleSyncer quesApp.ScaleQuestionnaireBindingSyncer, repo questionnaire.Repository, reader surveyreadmodel.QuestionnaireReader, cacheSignalNotifier quesApp.CacheSignalNotifier) error {
	sub := m.Questionnaire

	validator := questionnaire.Validator{}
	lifecycle := questionnaire.NewLifecycle()

	sub.LifecycleService = quesApp.NewLifecycleService(
		repo,
		scaleSyncer,
		validator,
		lifecycle,
		m.eventPublisher,
		quesApp.WithCacheSignalNotifier(cacheSignalNotifier),
	)
	sub.ContentService = quesApp.NewContentService(repo)
	sub.QueryService = quesApp.NewQueryService(repo, identitySvc, hotset, reader)

	return nil
}

func (m *Module) initAnswerSheetSubModule(mongoDB *mongo.Database, rankRedisClient redis.UniversalClient, rankCacheBuilder *keyspace.Builder, repo AnswerSheetStore, reader surveyreadmodel.AnswerSheetReader, questionnaireRepo questionnaire.Repository, outboxRelayBatchSize int, outboxRelayPublishWorkers int, outboxRelayImmediateMaxConcurrent int, opsHandle *cacheplane.Handle) error {
	sub := m.AnswerSheet

	batchValidator := ruleengineInfra.NewAnswerValidator()
	answerScorer := ruleengineInfra.NewAnswerScorer()

	mongoTxRunner := modtx.NewMongoRunner(mongoDB)
	var opsClient redis.UniversalClient
	if opsHandle != nil {
		opsClient = opsHandle.Client
	}
	readyIndex := outboxready.NewIndex(opsClient, outboxready.StoreMongoDomainEvents)
	hotRankProjection := cacheInfra.NewRedisScaleHotRankProjection(rankRedisClient, rankCacheBuilder)
	outboxRuntime := outboxruntime.Build(outboxruntime.Spec{
		Name:                    "mongo-domain-events",
		Store:                   repo,
		Publisher:               m.eventPublisher,
		ReadyIndex:              readyIndex,
		BatchSize:               outboxRelayBatchSize,
		PublishWorkers:          outboxRelayPublishWorkers,
		ImmediateMaxConcurrent:  outboxRelayImmediateMaxConcurrent,
		ImmediateEnabled:        true,
		RequireDurablePublisher: true,
		BeforePublishHooks: []appEventing.OutboxBeforePublishHook{
			scaleApp.NewScaleHotRankProjectionHook(hotRankProjection),
		},
	})
	durableStore := asApp.NewTransactionalSubmissionDurableStore(mongoTxRunner, repo, repo, outboxRuntime.Immediate)
	sub.SubmissionService = asApp.NewSubmissionService(repo, durableStore, questionnaireRepo, batchValidator, reader)
	sub.ManagementService = asApp.NewManagementService(repo, reader)
	sub.ScoringService = asApp.NewAnswerSheetScoringService(repo, questionnaireRepo, answerScorer)
	sub.SubmittedEventRelay = outboxRuntime.Relay
	sub.SubmittedEventStatusReader = appEventing.NamedOutboxStatusReader{
		Name:   "mongo-domain-events",
		Reader: repo,
	}
	sub.OutboxReadyIndex = readyIndex

	return nil
}

// Cleanup releases module resources.
func (m *Module) Cleanup() error {
	return nil
}

// CheckHealth verifies module health.
func (m *Module) CheckHealth() error {
	return nil
}

// ModuleInfo returns module metadata.
func (m *Module) ModuleInfo() modules.ModuleInfo {
	return modules.ModuleInfo{
		Name:        string(Name),
		Version:     "1.0.0",
		Description: "问卷量表模块（包含问卷和答卷子模块）",
	}
}
