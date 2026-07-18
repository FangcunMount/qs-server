package container

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/behaviorassessment"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	appmodelcatalog "github.com/FangcunMount/qs-server/internal/collection-server/application/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportnotify"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportwait"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/testee"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologyassessment"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologysession"
	collectioncache "github.com/FangcunMount/qs-server/internal/collection-server/cache"
	"github.com/FangcunMount/qs-server/internal/collection-server/concurrency"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/iam"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/collection-server/port/acl"
	"github.com/FangcunMount/qs-server/internal/collection-server/port/grpcbridge"
	resiliencesubsystem "github.com/FangcunMount/qs-server/internal/collection-server/resilience/subsystem"
	"github.com/FangcunMount/qs-server/internal/collection-server/transport/rest/catalogpeek"
	"github.com/FangcunMount/qs-server/internal/collection-server/transport/rest/handler"
	"github.com/FangcunMount/qs-server/internal/collection-server/transport/ws"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	controlredis "github.com/FangcunMount/qs-server/internal/pkg/resilience/control/redisadapter"
	locksubsystem "github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/subsystem"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/ratelimit"
	ratelimitredis "github.com/FangcunMount/qs-server/internal/pkg/resilience/ratelimit/redisadapter"
)

// Container 主容器，负责管理所有组件
type Container struct {
	initialized  bool
	opts         *options.Options
	opsHandle    *redisruntime.Handle
	locks        *locksubsystem.Subsystem
	resilience   *resiliencesubsystem.Subsystem
	familyStatus *observability.FamilyStatusRegistry

	// IAM 模块
	IAMModule *IAMModule

	// gRPC 客户端（由 GRPCClientRegistry 注入）
	answerSheetClient            *grpcclient.AnswerSheetClient
	questionnaireClient          *grpcclient.QuestionnaireClient
	testeeEvaluationClient       *grpcclient.TesteeEvaluationClient
	participantReportClient      *grpcclient.ParticipantReportClient
	assessmentIntakeClient       *grpcclient.AssessmentIntakeClient
	actorClient                  *grpcclient.ActorClient
	assessmentModelCatalogClient *grpcclient.AssessmentModelCatalogClient

	// 应用层服务
	submissionService                  *answersheet.SubmissionService
	questionnaireQueryService          *questionnaire.QueryService
	evaluationQueryService             *evaluation.QueryService
	waitReportService                  *reportwait.Service
	assessmentModelCatalogQueryService *appmodelcatalog.QueryService
	typologyModelQueryService          *typologymodel.QueryService
	typologyAssessmentQueryService     *typologyassessment.QueryService
	behaviorAssessmentQueryService     *behaviorassessment.QueryService
	typologySessionService             *typologysession.Service
	testeeService                      *testee.Service
	reportStatusReporter               *reportstatus.Reporter
	reportNotifier                     reportnotify.Notifier
	waitWatcherCancel                  context.CancelFunc
	resilienceCancel                   context.CancelFunc
	reportEventsHandler                *ws.ReportEventsHandler
	cacheSubsystem                     *collectioncache.Subsystem
	l1PeekRegistry                     *catalogpeek.Registry

	// 接口层处理器
	answerSheetHandler               *handler.AnswerSheetHandler
	questionnaireHandler             *handler.QuestionnaireHandler
	evaluationHandler                *handler.EvaluationHandler
	assessmentModelCatalogHandler    *handler.AssessmentModelCatalogHandler
	typologyModelHandler             *handler.TypologyModelHandler
	typologyAssessmentHandler        *handler.TypologyAssessmentHandler
	behaviorAssessmentHandler        *handler.BehaviorAssessmentHandler
	typologyAssessmentSessionHandler *handler.TypologyAssessmentSessionHandler
	testeeHandler                    *handler.TesteeHandler
	healthHandler                    *handler.HealthHandler

	queryConcurrencyGate      *concurrency.Gate
	catalogConcurrencyGate    *concurrency.Gate
	submitConcurrencyGate     *concurrency.Gate
	waitReportConcurrencyGate *concurrency.Gate
}

// ClientBundle is the collection-server runtime client graph produced by the
// gRPC integration stage and consumed by the container composition root.
type ClientBundle struct {
	AnswerSheet            *grpcclient.AnswerSheetClient
	Questionnaire          *grpcclient.QuestionnaireClient
	TesteeEvaluation       *grpcclient.TesteeEvaluationClient
	ParticipantReport      *grpcclient.ParticipantReportClient
	AssessmentIntake       *grpcclient.AssessmentIntakeClient
	Actor                  *grpcclient.ActorClient
	AssessmentModelCatalog *grpcclient.AssessmentModelCatalogClient
}

func (c *Container) TesteeService() *testee.Service {
	if c == nil {
		return nil
	}
	return c.testeeService
}

// NewContainer 创建新的容器
func NewContainer(opts *options.Options, opsHandle *redisruntime.Handle, locks *locksubsystem.Subsystem, familyStatus *observability.FamilyStatusRegistry) (*Container, error) {
	var backend ratelimit.Backend
	if opsHandle != nil && opsHandle.Client != nil {
		backend = ratelimitredis.NewBackend(opsHandle.Client, opsHandle.Builder)
	}
	var stateStore *controlredis.Store
	if opsHandle != nil {
		stateStore = controlredis.NewStore(opsHandle.Client, opsHandle.Builder)
	}
	var rateCfg *options.RateLimitOptions
	var concurrencyCfg *options.ConcurrencyOptions
	var waitCfg *options.WaitReportOptions
	var grpcCfg *options.GRPCClientOptions
	var controlEnabled *bool
	if opts != nil {
		rateCfg, concurrencyCfg, waitCfg, grpcCfg = opts.RateLimit, opts.Concurrency, opts.WaitReport, opts.GRPCClient
		if opts.Resilience != nil && opts.Resilience.Control != nil {
			controlEnabled = &opts.Resilience.Control.Enabled
		}
	}
	resilienceSubsystem, err := resiliencesubsystem.New(resiliencesubsystem.Options{
		RateLimit: rateCfg, Concurrency: concurrencyCfg, WaitReport: waitCfg, GRPCClient: grpcCfg,
		Backend: backend, Locks: locks, OpsAvailable: opsHandle != nil && opsHandle.Client != nil, StateStore: stateStore,
		ControlEnabled: controlEnabled,
	})
	if err != nil {
		return nil, err
	}
	c := &Container{
		opts:           opts,
		opsHandle:      opsHandle,
		locks:          locks,
		familyStatus:   familyStatus,
		initialized:    false,
		cacheSubsystem: collectioncache.NewSubsystem(collectionCacheConfig(opts), opsHandle),
		resilience:     resilienceSubsystem,
	}
	c.initConcurrencyGates()
	return c, nil
}

func collectionCacheConfig(opts *options.Options) collectioncache.Config {
	config := collectioncache.Config{Signaling: collectioncache.SignalOptions{Prefix: "qs:signal", BufferSize: 100}}
	if opts == nil {
		return config
	}
	if opts.Signaling != nil && opts.Signaling.Redis != nil {
		redis := opts.Signaling.Redis
		config.Signaling.Enabled = redis.Enabled
		if redis.Prefix != "" {
			config.Signaling.Prefix = redis.Prefix
		}
		config.Signaling.Channel = redis.Channel
		if redis.BufferSize > 0 {
			config.Signaling.BufferSize = redis.BufferSize
		}
	}
	if opts.Cache == nil || opts.Cache.Capabilities == nil {
		return config
	}
	capabilities := opts.Cache.Capabilities
	if capabilities.ReportStatus != nil {
		config.ReportStatusTTL = time.Duration(capabilities.ReportStatus.TTLSeconds) * time.Second
	}
	if capabilities.Catalog == nil {
		return config
	}
	var questionnaireOptions *options.CatalogL1CacheOptions
	if capabilities.Catalog.Questionnaire != nil {
		questionnaireOptions = &capabilities.Catalog.Questionnaire.CatalogL1CacheOptions
	}
	var typologyOptions *options.CatalogL1CacheOptions
	if capabilities.Catalog.Typology != nil {
		typologyOptions = &capabilities.Catalog.Typology.CatalogL1CacheOptions
	}
	config.Questionnaire = catalogBinding("catalog.questionnaire", "cache.capabilities.catalog.questionnaire", questionnaireOptions)
	config.Typology = catalogBinding("catalog.typology", "cache.capabilities.catalog.typology", typologyOptions)
	return config
}

func catalogBinding(id, source string, cfg *options.CatalogL1CacheOptions) collectioncache.CatalogBinding {
	binding := collectioncache.CatalogBinding{Capability: sharedcache.Capability(id), Source: source}
	if cfg == nil {
		return binding
	}
	binding.Enabled = cfg.Enabled
	binding.Policy = sharedcache.Policy{
		TTL: time.Duration(cfg.TTLSeconds) * time.Second, JitterRatio: cfg.TTLJitterRatio,
		Singleflight: sharedcache.PolicySwitchFromBool(cfg.Singleflight),
	}
	binding.MaxEntries = cfg.MaxEntries
	binding.Singleflight = cfg.Singleflight
	binding.SignalEvict = cfg.SignalEvictEnabled
	return binding
}

func (c *Container) initConcurrencyGates() {
	if c == nil || c.resilience == nil {
		return
	}
	c.queryConcurrencyGate = c.resilience.Gate(resiliencesubsystem.GateQuery)
	c.catalogConcurrencyGate = c.resilience.Gate(resiliencesubsystem.GateCatalog)
	c.submitConcurrencyGate = c.resilience.Gate(resiliencesubsystem.GateSubmit)
	c.waitReportConcurrencyGate = c.resilience.Gate(resiliencesubsystem.GateWaitReport)
}

// Initialize 初始化容器中的所有组件
func (c *Container) Initialize() error {
	if c.initialized {
		return nil
	}

	log.Info("🔧 Initializing Collection Server Container...")

	// 1. 初始化应用层
	c.initApplicationServices()
	if c.resilience != nil {
		if err := c.resilience.Sync(context.Background()); err != nil {
			log.Warnf("collection resilience initial control sync pending: %v", err)
		}
	}

	// 2. 初始化接口层
	c.initHandlers()
	if c.resilience != nil {
		c.resilienceCancel = c.resilience.Start(context.Background())
	}

	if c.cacheSubsystem != nil {
		c.cacheSubsystem.BindWarmup(c.typologyModelQueryService)
	}

	c.initialized = true
	log.Info("✅ Collection Server Container initialized successfully")

	return nil
}

// initApplicationServices 初始化应用层服务
func (c *Container) initApplicationServices() {
	log.Info("🎯 Initializing application services...")

	profileLinkService, profileService := c.profileServices()

	catalogRuntime := c.buildCatalogRuntime()
	c.questionnaireQueryService = catalogRuntime.questionnaire
	c.assessmentModelCatalogQueryService = catalogRuntime.assessmentModels
	c.typologyModelQueryService = catalogRuntime.typology

	submitRuntime := c.buildSubmitRuntime(profileLinkService, c.questionnaireQueryService)
	c.submissionService = submitRuntime.submission
	c.evaluationQueryService = evaluation.NewQueryService(
		grpcbridge.NewEvaluationBFFReader(c.testeeEvaluationClient, c.participantReportClient, c.assessmentIntakeClient),
		c.assessmentModelCatalogQueryService,
	)
	reportRuntime := c.buildReportRuntime(c.evaluationQueryService)
	c.reportStatusReporter = reportRuntime.reporter
	c.reportNotifier = reportRuntime.notifier
	c.waitReportService = reportRuntime.waitReport
	c.waitWatcherCancel = reportRuntime.waitWatcherCancel

	c.typologyAssessmentQueryService = typologyassessment.NewQueryService(
		grpcbridge.NewEvaluationBFFReader(c.testeeEvaluationClient, c.participantReportClient, c.assessmentIntakeClient),
		c.waitReportService,
	)
	c.behaviorAssessmentQueryService = behaviorassessment.NewQueryService(
		grpcbridge.NewEvaluationBFFReader(c.testeeEvaluationClient, c.participantReportClient, c.assessmentIntakeClient),
		c.waitReportService,
	)
	c.typologySessionService = typologysession.NewService(c.typologyModelQueryService, c.questionnaireQueryService)
	c.testeeService = testee.NewService(acl.NewTesteeActorAdapter(c.actorClient), profileLinkService, profileService)
	c.reportEventsHandler = c.buildReportEventsHandler()

	log.Info("✅ Application services initialized")
}

// initHandlers 初始化接口层处理器
func (c *Container) initHandlers() {
	log.Info("🌐 Initializing REST handlers...")

	// 获取 ProfileLinkService（如果 IAM 启用）
	var profileLinkService *iam.ProfileLinkService
	if c.IAMModule != nil && c.IAMModule.IsEnabled() {
		profileLinkService = c.IAMModule.ProfileLinkService()
	}

	c.answerSheetHandler = handler.NewAnswerSheetHandler(c.submissionService)
	c.questionnaireHandler = handler.NewQuestionnaireHandler(c.questionnaireQueryService)
	c.evaluationHandler = handler.NewEvaluationHandler(c.evaluationQueryService, c.waitReportService)
	c.assessmentModelCatalogHandler = handler.NewAssessmentModelCatalogHandler(c.assessmentModelCatalogQueryService)
	c.typologyModelHandler = handler.NewTypologyModelHandler(c.typologyModelQueryService)
	c.typologyAssessmentHandler = handler.NewTypologyAssessmentHandler(c.typologyAssessmentQueryService, c.waitReportService)
	c.behaviorAssessmentHandler = handler.NewBehaviorAssessmentHandler(c.behaviorAssessmentQueryService, c.waitReportService)
	c.typologyAssessmentSessionHandler = handler.NewTypologyAssessmentSessionHandler(c.typologySessionService)
	c.testeeHandler = handler.NewTesteeHandler(c.testeeService, profileLinkService)
	c.healthHandler = handler.NewHealthHandlerWithResilience("collection-server", "2.0.0", c.familyStatus, c.ResilienceSnapshot, c.resilience.ControlSynchronized)

	log.Info("✅ REST handlers initialized")
}

// Cleanup 清理资源
func (c *Container) Cleanup() {
	log.Info("🧹 Cleaning up container resources...")
	if c.waitWatcherCancel != nil {
		c.waitWatcherCancel()
		c.waitWatcherCancel = nil
	}
	if c.resilienceCancel != nil {
		c.resilienceCancel()
		c.resilienceCancel = nil
	}
	if c.cacheSubsystem != nil {
		_ = c.cacheSubsystem.Close()
	}

	c.initialized = false
	log.Info("🏁 Container cleanup completed")
}

// StartCacheSubsystem starts collection cache lifecycle after composition completes.
func (c *Container) StartCacheSubsystem(ctx context.Context) error {
	if c == nil || c.cacheSubsystem == nil {
		return nil
	}
	return c.cacheSubsystem.Start(ctx)
}

// IsInitialized 检查容器是否已初始化
func (c *Container) IsInitialized() bool {
	return c.initialized
}

// ==================== Getters ====================

// AnswerSheetHandler 获取答卷处理器
func (c *Container) AnswerSheetHandler() *handler.AnswerSheetHandler {
	return c.answerSheetHandler
}

// QuestionnaireHandler 获取问卷处理器
func (c *Container) QuestionnaireHandler() *handler.QuestionnaireHandler {
	return c.questionnaireHandler
}

// HealthHandler 获取健康检查处理器
func (c *Container) HealthHandler() *handler.HealthHandler {
	return c.healthHandler
}

// EvaluationHandler 获取测评处理器
func (c *Container) EvaluationHandler() *handler.EvaluationHandler {
	return c.evaluationHandler
}

// TesteeHandler 获取受试者处理器
func (c *Container) TesteeHandler() *handler.TesteeHandler {
	return c.testeeHandler
}

// AssessmentModelCatalogHandler returns the generic published-model catalogue handler.
func (c *Container) AssessmentModelCatalogHandler() *handler.AssessmentModelCatalogHandler {
	return c.assessmentModelCatalogHandler
}

// TypologyModelHandler 获取人格测评模型处理器
func (c *Container) TypologyModelHandler() *handler.TypologyModelHandler {
	return c.typologyModelHandler
}

// TypologyAssessmentSessionHandler 获取人格测评会话处理器
func (c *Container) TypologyAssessmentSessionHandler() *handler.TypologyAssessmentSessionHandler {
	return c.typologyAssessmentSessionHandler
}

// TypologyAssessmentHandler 获取人格测评处理器
func (c *Container) TypologyAssessmentHandler() *handler.TypologyAssessmentHandler {
	return c.typologyAssessmentHandler
}

// BehaviorAssessmentHandler 获取行为能力测评处理器。
func (c *Container) BehaviorAssessmentHandler() *handler.BehaviorAssessmentHandler {
	return c.behaviorAssessmentHandler
}

// RateLimitOptions 获取限流配置
func (c *Container) RateLimitOptions() *options.RateLimitOptions {
	return c.opts.RateLimit
}

// OpsHandle returns the collection-server operational Redis handle.
func (c *Container) OpsHandle() *redisruntime.Handle {
	return c.opsHandle
}

func (c *Container) RateLimitBackend() ratelimit.Backend {
	if c == nil || c.opsHandle == nil || c.opsHandle.Client == nil {
		return nil
	}
	return ratelimitredis.NewBackend(c.opsHandle.Client, c.opsHandle.Builder)
}

func (c *Container) RateBudgetProvider() ratelimit.RateBudgetProvider {
	if c == nil {
		return nil
	}
	return c.resilience
}

func (c *Container) ConcurrencyOptions() *options.ConcurrencyOptions {
	if c == nil || c.opts == nil || c.opts.Concurrency == nil {
		return options.NewOptions().Concurrency
	}
	return c.opts.Concurrency
}

func (c *Container) SubmitOptions() *options.SubmitOptions {
	if c == nil || c.opts == nil || c.opts.Submit == nil {
		return options.NewSubmitOptions()
	}
	return c.opts.Submit
}

func (c *Container) QueryConcurrencyGate() *concurrency.Gate {
	if c == nil {
		return nil
	}
	return c.queryConcurrencyGate
}

func (c *Container) GRPCDownstreamGate() *concurrency.Gate {
	if c == nil || c.resilience == nil {
		return nil
	}
	return c.resilience.Gate(resiliencesubsystem.GateGRPCDownstream)
}

func (c *Container) CatalogConcurrencyGate() *concurrency.Gate {
	if c == nil {
		return nil
	}
	return c.catalogConcurrencyGate
}

func (c *Container) AssessmentModelCatalogQueryService() *appmodelcatalog.QueryService {
	if c == nil {
		return nil
	}
	return c.assessmentModelCatalogQueryService
}

func (c *Container) TypologyModelQueryService() *typologymodel.QueryService {
	if c == nil {
		return nil
	}
	return c.typologyModelQueryService
}

func (c *Container) QuestionnaireQueryService() *questionnaire.QueryService {
	if c == nil {
		return nil
	}
	return c.questionnaireQueryService
}

func (c *Container) CatalogL1PeekRegistry() *catalogpeek.Registry {
	if c == nil {
		return nil
	}
	return c.l1PeekRegistry
}

func (c *Container) SubmitConcurrencyGate() *concurrency.Gate {
	if c == nil {
		return nil
	}
	return c.submitConcurrencyGate
}

func (c *Container) WaitReportConcurrencyGate() *concurrency.Gate {
	if c == nil {
		return nil
	}
	return c.waitReportConcurrencyGate
}

func (c *Container) WaitReportOptions() *options.WaitReportOptions {
	if c == nil || c.opts == nil || c.opts.WaitReport == nil {
		return options.NewWaitReportOptions()
	}
	return c.opts.WaitReport
}

func (c *Container) ReportEventsHandler() *ws.ReportEventsHandler {
	if c == nil {
		return nil
	}
	return c.reportEventsHandler
}

func (c *Container) ReportEventsOptions() *options.ReportEventsOptions {
	if c == nil || c.opts == nil || c.opts.ReportEvents == nil {
		return options.NewReportEventsOptions()
	}
	return c.opts.ReportEvents
}

func (c *Container) ResilienceSnapshot() resilience.RuntimeSnapshot {
	if c != nil && c.resilience != nil {
		return c.resilience.Snapshot(time.Now())
	}
	return resilience.RuntimeSnapshot{Component: "collection-server"}
}

// InitializeRuntimeClients installs the runtime client bundle built by the
// integration stage. It replaces per-client setter wiring with one explicit
// composition edge.
func (c *Container) InitializeRuntimeClients(bundle ClientBundle) {
	if c == nil {
		return
	}
	c.answerSheetClient = bundle.AnswerSheet
	c.questionnaireClient = bundle.Questionnaire
	c.testeeEvaluationClient = bundle.TesteeEvaluation
	c.participantReportClient = bundle.ParticipantReport
	c.assessmentIntakeClient = bundle.AssessmentIntake
	c.actorClient = bundle.Actor
	c.assessmentModelCatalogClient = bundle.AssessmentModelCatalog
}

// ActorClient 获取 Actor 客户端
func (c *Container) ActorClient() *grpcclient.ActorClient {
	return c.actorClient
}
