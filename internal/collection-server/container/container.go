package container

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/catalogcache"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/personalityassessment"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/personalitymodel"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/personalitysession"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportnotify"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportwait"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/testee"
	"github.com/FangcunMount/qs-server/internal/collection-server/concurrency"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/iam"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/collection-server/port/acl"
	"github.com/FangcunMount/qs-server/internal/collection-server/port/grpcbridge"
	"github.com/FangcunMount/qs-server/internal/collection-server/transport/rest/catalogpeek"
	"github.com/FangcunMount/qs-server/internal/collection-server/transport/rest/handler"
	"github.com/FangcunMount/qs-server/internal/collection-server/transport/ws"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/ratelimit"
	ratelimitredis "github.com/FangcunMount/qs-server/internal/pkg/ratelimit/redisadapter"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

// Container 主容器，负责管理所有组件
type Container struct {
	initialized  bool
	opts         *options.Options
	opsHandle    *cacheplane.Handle
	lockManager  locklease.Manager
	familyStatus *observability.FamilyStatusRegistry

	// IAM 模块
	IAMModule *IAMModule

	// gRPC 客户端（由 GRPCClientRegistry 注入）
	answerSheetClient      *grpcclient.AnswerSheetClient
	questionnaireClient    *grpcclient.QuestionnaireClient
	evaluationClient       *grpcclient.EvaluationClient
	actorClient            *grpcclient.ActorClient
	scaleClient            *grpcclient.ScaleClient
	personalityModelClient *grpcclient.PersonalityModelClient

	// 应用层服务
	submissionService                 *answersheet.SubmissionService
	questionnaireQueryService         *questionnaire.QueryService
	evaluationQueryService            *evaluation.QueryService
	waitReportService                 *reportwait.Service
	scaleQueryService                 *scale.QueryService
	personalityModelQueryService      *personalitymodel.QueryService
	personalityAssessmentQueryService *personalityassessment.QueryService
	personalitySessionService         *personalitysession.Service
	testeeService                     *testee.Service
	reportStatusReporter              *reportstatus.Reporter
	reportNotifier                    reportnotify.Notifier
	waitWatcherCancel                 context.CancelFunc
	reportEventsHandler               *ws.ReportEventsHandler
	catalogCacheWatcherCancels        []context.CancelFunc
	l1PeekRegistry                    *catalogpeek.Registry

	// 接口层处理器
	answerSheetHandler                  *handler.AnswerSheetHandler
	questionnaireHandler                *handler.QuestionnaireHandler
	evaluationHandler                   *handler.EvaluationHandler
	scaleHandler                        *handler.ScaleHandler
	personalityModelHandler             *handler.PersonalityModelHandler
	personalityAssessmentHandler        *handler.PersonalityAssessmentHandler
	personalityAssessmentSessionHandler *handler.PersonalityAssessmentSessionHandler
	testeeHandler                       *handler.TesteeHandler
	healthHandler                       *handler.HealthHandler

	queryConcurrencyGate      *concurrency.Gate
	catalogConcurrencyGate    *concurrency.Gate
	submitConcurrencyGate     *concurrency.Gate
	waitReportConcurrencyGate *concurrency.Gate
}

// ClientBundle is the collection-server runtime client graph produced by the
// gRPC integration stage and consumed by the container composition root.
type ClientBundle struct {
	AnswerSheet      *grpcclient.AnswerSheetClient
	Questionnaire    *grpcclient.QuestionnaireClient
	Evaluation       *grpcclient.EvaluationClient
	Actor            *grpcclient.ActorClient
	Scale            *grpcclient.ScaleClient
	PersonalityModel *grpcclient.PersonalityModelClient
}

// NewContainer 创建新的容器
func NewContainer(opts *options.Options, opsHandle *cacheplane.Handle, lockManager locklease.Manager, familyStatus *observability.FamilyStatusRegistry) *Container {
	c := &Container{
		opts:         opts,
		opsHandle:    opsHandle,
		lockManager:  lockManager,
		familyStatus: familyStatus,
		initialized:  false,
	}
	c.initConcurrencyGates()
	return c
}

func (c *Container) initConcurrencyGates() {
	var concurrencyOpts *options.ConcurrencyOptions
	if c.opts != nil {
		concurrencyOpts = c.opts.Concurrency
	}
	maxQuery := 0
	maxCatalog := 0
	maxSubmit := 0
	if concurrencyOpts != nil {
		maxQuery = concurrencyOpts.ResolvedQueryConcurrency()
		maxCatalog = concurrencyOpts.ResolvedCatalogConcurrency()
		maxSubmit = concurrencyOpts.ResolvedSubmitConcurrency()
	}
	maxWaitReport := 0
	degradeEnabled := true
	if c.opts != nil && c.opts.WaitReport != nil {
		maxWaitReport = c.opts.WaitReport.MaxHTTPConcurrency
		degradeEnabled = c.opts.WaitReport.DegradeImmediateEnabled
	}
	if maxWaitReport <= 0 {
		maxWaitReport = 400
	}
	c.queryConcurrencyGate = concurrency.NewGate(maxQuery)
	c.catalogConcurrencyGate = concurrency.NewGate(maxCatalog)
	c.submitConcurrencyGate = concurrency.NewGate(maxSubmit)
	if degradeEnabled {
		c.waitReportConcurrencyGate = concurrency.NewGate(maxWaitReport)
	} else {
		c.waitReportConcurrencyGate = c.queryConcurrencyGate
	}
}

// Initialize 初始化容器中的所有组件
func (c *Container) Initialize() error {
	if c.initialized {
		return nil
	}

	log.Info("🔧 Initializing Collection Server Container...")

	// 1. 初始化应用层
	c.initApplicationServices()

	// 2. 初始化接口层
	c.initHandlers()

	if c.scaleClient != nil && c.personalityModelClient != nil {
		catalogcache.WarmCatalogOnStartup(c.scaleQueryService, c.personalityModelQueryService)
	}

	c.initialized = true
	log.Info("✅ Collection Server Container initialized successfully")

	return nil
}

// initApplicationServices 初始化应用层服务
func (c *Container) initApplicationServices() {
	log.Info("🎯 Initializing application services...")

	profileLinkService, profileService := c.profileServices()

	submitRuntime := c.buildSubmitRuntime(profileLinkService)
	c.submissionService = submitRuntime.submission

	catalogRuntime := c.buildCatalogRuntime()
	c.questionnaireQueryService = catalogRuntime.questionnaire
	c.scaleQueryService = catalogRuntime.scale
	c.personalityModelQueryService = catalogRuntime.personality

	c.evaluationQueryService = evaluation.NewQueryService(
		grpcbridge.NewEvaluationBFFReader(c.evaluationClient),
		grpcbridge.NewScaleCatalogReader(c.scaleClient),
	)
	reportRuntime := c.buildReportRuntime(c.evaluationQueryService)
	c.reportStatusReporter = reportRuntime.reporter
	c.reportNotifier = reportRuntime.notifier
	c.waitReportService = reportRuntime.waitReport
	c.waitWatcherCancel = reportRuntime.waitWatcherCancel

	c.personalityAssessmentQueryService = personalityassessment.NewQueryService(
		grpcbridge.NewEvaluationBFFReader(c.evaluationClient),
		c.waitReportService,
	)
	c.personalitySessionService = personalitysession.NewService(c.personalityModelQueryService, c.questionnaireQueryService)
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
	c.evaluationHandler = handler.NewEvaluationHandler(c.evaluationQueryService, c.submissionService, c.waitReportService)
	c.scaleHandler = handler.NewScaleHandler(c.scaleQueryService)
	c.personalityModelHandler = handler.NewPersonalityModelHandler(c.personalityModelQueryService)
	c.personalityAssessmentHandler = handler.NewPersonalityAssessmentHandler(c.personalityAssessmentQueryService, c.waitReportService)
	c.personalityAssessmentSessionHandler = handler.NewPersonalityAssessmentSessionHandler(c.personalitySessionService)
	c.testeeHandler = handler.NewTesteeHandler(c.testeeService, profileLinkService)
	c.healthHandler = handler.NewHealthHandlerWithResilience("collection-server", "2.0.0", c.familyStatus, c.ResilienceSnapshot)

	log.Info("✅ REST handlers initialized")
}

// Cleanup 清理资源
func (c *Container) Cleanup() {
	log.Info("🧹 Cleaning up container resources...")
	if c.waitWatcherCancel != nil {
		c.waitWatcherCancel()
		c.waitWatcherCancel = nil
	}
	c.cleanupCatalogCaches()

	c.initialized = false
	log.Info("🏁 Container cleanup completed")
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

// ScaleHandler 获取量表处理器
func (c *Container) ScaleHandler() *handler.ScaleHandler {
	return c.scaleHandler
}

// PersonalityModelHandler 获取人格测评模型处理器
func (c *Container) PersonalityModelHandler() *handler.PersonalityModelHandler {
	return c.personalityModelHandler
}

// PersonalityAssessmentSessionHandler 获取人格测评会话处理器
func (c *Container) PersonalityAssessmentSessionHandler() *handler.PersonalityAssessmentSessionHandler {
	return c.personalityAssessmentSessionHandler
}

// PersonalityAssessmentHandler 获取人格测评处理器
func (c *Container) PersonalityAssessmentHandler() *handler.PersonalityAssessmentHandler {
	return c.personalityAssessmentHandler
}

// RateLimitOptions 获取限流配置
func (c *Container) RateLimitOptions() *options.RateLimitOptions {
	return c.opts.RateLimit
}

// OpsHandle returns the collection-server operational Redis handle.
func (c *Container) OpsHandle() *cacheplane.Handle {
	return c.opsHandle
}

func (c *Container) RateLimitBackend() ratelimit.Backend {
	if c == nil || c.opsHandle == nil || c.opsHandle.Client == nil {
		return nil
	}
	return ratelimitredis.NewBackend(c.opsHandle.Client, c.opsHandle.Builder)
}

func (c *Container) ConcurrencyOptions() *options.ConcurrencyOptions {
	if c == nil || c.opts == nil || c.opts.Concurrency == nil {
		return options.NewOptions().Concurrency
	}
	return c.opts.Concurrency
}

func (c *Container) QueryConcurrencyGate() *concurrency.Gate {
	if c == nil {
		return nil
	}
	return c.queryConcurrencyGate
}

func (c *Container) CatalogConcurrencyGate() *concurrency.Gate {
	if c == nil {
		return nil
	}
	return c.catalogConcurrencyGate
}

func (c *Container) ScaleQueryService() *scale.QueryService {
	if c == nil {
		return nil
	}
	return c.scaleQueryService
}

func (c *Container) PersonalityModelQueryService() *personalitymodel.QueryService {
	if c == nil {
		return nil
	}
	return c.personalityModelQueryService
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

func (c *Container) ResilienceSnapshot() resilienceplane.RuntimeSnapshot {
	now := time.Now()
	snapshot := resilienceplane.NewRuntimeSnapshot("collection-server", now)
	var rateCfg *options.RateLimitOptions
	if c != nil && c.opts != nil {
		rateCfg = c.opts.RateLimit
	}
	strategy := "local"
	if c != nil && c.opsHandle != nil && c.opsHandle.Client != nil {
		strategy = "redis"
	}
	if rateCfg != nil {
		snapshot.RateLimits = []resilienceplane.CapabilitySnapshot{
			{Name: "submit_global", Kind: resilienceplane.ProtectionRateLimit.String(), Strategy: strategy, Configured: rateCfg.Enabled},
			{Name: "submit_user", Kind: resilienceplane.ProtectionRateLimit.String(), Strategy: strategy, Configured: rateCfg.Enabled},
			{Name: "query_global", Kind: resilienceplane.ProtectionRateLimit.String(), Strategy: strategy, Configured: rateCfg.Enabled},
			{Name: "query_user", Kind: resilienceplane.ProtectionRateLimit.String(), Strategy: strategy, Configured: rateCfg.Enabled},
		}
	}
	if c != nil && c.submissionService != nil {
		snapshot.Queues = []resilienceplane.QueueSnapshot{c.submissionService.SubmitQueueStatusSnapshot(now)}
	}
	idempotencyConfigured := c != nil && c.lockManager != nil && c.opsHandle != nil && c.opsHandle.Client != nil
	snapshot.Idempotency = []resilienceplane.CapabilitySnapshot{{
		Name:       "answersheet_submit",
		Kind:       resilienceplane.ProtectionIdempotency.String(),
		Strategy:   "redis_lock",
		Configured: idempotencyConfigured,
		Degraded:   !idempotencyConfigured,
		Reason:     resilienceReason(idempotencyConfigured, "submit guard redis runtime unavailable"),
	}}
	return resilienceplane.FinalizeRuntimeSnapshot(snapshot)
}

func resilienceReason(ok bool, reason string) string {
	if ok {
		return ""
	}
	return reason
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
	c.evaluationClient = bundle.Evaluation
	c.actorClient = bundle.Actor
	c.scaleClient = bundle.Scale
	c.personalityModelClient = bundle.PersonalityModel
}

// ActorClient 获取 Actor 客户端
func (c *Container) ActorClient() *grpcclient.ActorClient {
	return c.actorClient
}
