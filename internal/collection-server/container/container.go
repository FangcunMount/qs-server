package container

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	signalredis "github.com/FangcunMount/component-base/pkg/signaling/redis"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/personalityassessment"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/personalitymodel"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportwait"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/testee"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/iam"
	redisops "github.com/FangcunMount/qs-server/internal/collection-server/infra/redisops"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/collection-server/transport/rest/handler"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
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
	testeeService                     *testee.Service
	reportStatusReporter              *reportstatus.Reporter
	waitHub                           reportwait.WaitHub
	waitWatcherCancel                 context.CancelFunc

	// 接口层处理器
	answerSheetHandler           *handler.AnswerSheetHandler
	questionnaireHandler         *handler.QuestionnaireHandler
	evaluationHandler            *handler.EvaluationHandler
	scaleHandler                 *handler.ScaleHandler
	personalityModelHandler      *handler.PersonalityModelHandler
	personalityAssessmentHandler *handler.PersonalityAssessmentHandler
	testeeHandler                *handler.TesteeHandler
	healthHandler                *handler.HealthHandler
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
	return &Container{
		opts:         opts,
		opsHandle:    opsHandle,
		lockManager:  lockManager,
		familyStatus: familyStatus,
		initialized:  false,
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

	c.initialized = true
	log.Info("✅ Collection Server Container initialized successfully")

	return nil
}

// initApplicationServices 初始化应用层服务
func (c *Container) initApplicationServices() {
	log.Info("🎯 Initializing application services...")

	// 获取 ProfileLinkService（如果 IAM 启用）
	var profileLinkService *iam.ProfileLinkService
	var profileService *iam.ProfileService
	if c.IAMModule != nil && c.IAMModule.IsEnabled() {
		profileLinkService = c.IAMModule.ProfileLinkService()
		profileService = c.IAMModule.ProfileService()
	}
	submitGuard := redisops.NewSubmitGuard(c.opsHandle, c.lockManager)

	c.submissionService = answersheet.NewSubmissionService(
		c.answerSheetClient,
		c.actorClient,
		profileLinkService,
		c.opts.SubmitQueue,
		submitGuard,
	)
	c.questionnaireQueryService = questionnaire.NewQueryService(c.questionnaireClient)
	c.evaluationQueryService = evaluation.NewQueryService(c.evaluationClient, c.scaleClient)
	var reportOpts *genericoptions.ReportStatusOptions
	var sigOpts *genericoptions.SignalingOptions
	if c.opts != nil {
		reportOpts = c.opts.ReportStatus
		sigOpts = c.opts.Signaling
	}
	reportStatusRuntime := reportstatus.ConfigFromOptions(reportOpts, sigOpts, "collection-server")
	reporter, err := reportstatus.NewReporter(c.opsHandle, reportStatusRuntime)
	if err != nil {
		log.Warnf("report status reporter disabled: %v", err)
	}
	c.reportStatusReporter = reporter

	cfg := reportwait.DefaultConfig()
	if c.opts != nil && c.opts.WaitReport != nil {
		cfg.DefaultTimeout = time.Duration(c.opts.WaitReport.DefaultTimeoutSeconds) * time.Second
		cfg.MinTimeout = time.Duration(c.opts.WaitReport.MinTimeoutSeconds) * time.Second
		cfg.MaxTimeout = time.Duration(c.opts.WaitReport.MaxTimeoutSeconds) * time.Second
		cfg.PollInterval = time.Duration(c.opts.WaitReport.PollIntervalMs) * time.Millisecond
		cfg.StatusTTL = time.Duration(c.opts.WaitReport.StatusTTLSeconds) * time.Second
		cfg.MaxActiveWaiters = c.opts.WaitReport.MaxActiveWaiters
		cfg.SignalingEnabled = reportStatusRuntime.Signaling.Enabled
		if c.opts.WaitReport.PubSubEnabled {
			cfg.SignalingEnabled = true
		}
	}
	c.waitHub = reportwait.NewInMemoryWaitHub()
	var signaler *signalredis.Signaler[reportstatus.ChangedSignal]
	if reporter != nil {
		signaler = reporter.Signaler()
	}
	c.waitReportService = reportwait.NewService(
		c.evaluationQueryService,
		reportwait.NewStatusCache(reportstatus.NewCache(c.opsHandle)),
		c.waitHub,
		signaler,
		cfg,
	)
	if cfg.SignalingEnabled && signaler != nil {
		watchCtx, cancel := context.WithCancel(context.Background())
		c.waitReportService.StartSignalWatcher(watchCtx)
		c.waitWatcherCancel = cancel
	}
	c.scaleQueryService = scale.NewQueryService(c.scaleClient)
	c.personalityModelQueryService = personalitymodel.NewQueryService(c.personalityModelClient)
	c.personalityAssessmentQueryService = personalityassessment.NewQueryService(c.evaluationClient, c.waitReportService)
	c.testeeService = testee.NewService(c.actorClient, profileLinkService, profileService)

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
