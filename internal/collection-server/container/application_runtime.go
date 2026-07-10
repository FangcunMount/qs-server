package container

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	signalredis "github.com/FangcunMount/component-base/pkg/signaling/redis"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	appmodelcatalog "github.com/FangcunMount/qs-server/internal/collection-server/application/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportevents"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportnotify"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportwait"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/iam"
	redisops "github.com/FangcunMount/qs-server/internal/collection-server/infra/redisops"
	"github.com/FangcunMount/qs-server/internal/collection-server/port/acl"
	"github.com/FangcunMount/qs-server/internal/collection-server/port/grpcbridge"
	"github.com/FangcunMount/qs-server/internal/collection-server/transport/rest/catalogpeek"
	"github.com/FangcunMount/qs-server/internal/collection-server/transport/ws"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
)

type submitRuntime struct {
	submission *answersheet.SubmissionService
}

type catalogRuntime struct {
	questionnaire    *questionnaire.QueryService
	assessmentModels *appmodelcatalog.QueryService
	typology         *typologymodel.QueryService
}

type reportRuntime struct {
	reporter          *reportstatus.Reporter
	notifier          reportnotify.Notifier
	waitReport        *reportwait.Service
	waitWatcherCancel context.CancelFunc
}

func (c *Container) profileServices() (*iam.ProfileLinkService, *iam.ProfileService) {
	if c.IAMModule == nil || !c.IAMModule.IsEnabled() {
		return nil, nil
	}
	return c.IAMModule.ProfileLinkService(), c.IAMModule.ProfileService()
}

func (c *Container) buildSubmitRuntime(profileLinkService *iam.ProfileLinkService) submitRuntime {
	// answersheet / testee 经 acl 适配（REST↔gRPC 字段差异）；catalog / evaluation 经 grpcbridge 直出 application DTO。
	submitGuard := redisops.NewSubmitGuard(c.opsHandle, c.lockManager)
	return submitRuntime{
		submission: answersheet.NewSubmissionService(
			acl.NewAnswerSheetBFFWriter(c.answerSheetClient),
			acl.NewAnswerSheetBFFReader(c.answerSheetClient),
			acl.NewTesteeActorLookup(c.actorClient),
			profileLinkService,
			c.opts.SubmitQueue,
			submitGuard,
			grpcbridge.NewEvaluationBFFReader(c.evaluationClient),
		),
	}
}

func (c *Container) buildCatalogRuntime() catalogRuntime {
	// catalog 读路径不经 acl：grpcbridge catalog reader 直接产出 application DTO。
	catalogCaches := c.initCatalogCaches()
	rt := catalogRuntime{
		questionnaire: questionnaire.NewQueryService(
			grpcbridge.NewQuestionnaireCatalogReader(c.questionnaireClient),
			catalogCaches.questionnaire,
			catalogL1SingleflightEnabled(c.opts, catalogKindQuestionnaire),
		),
		assessmentModels: appmodelcatalog.NewQueryService(grpcbridge.NewAssessmentModelCatalogReader(c.assessmentModelCatalogClient)),
		typology: typologymodel.NewQueryService(
			grpcbridge.NewTypologyCatalogProjector(appmodelcatalog.NewQueryService(grpcbridge.NewAssessmentModelCatalogReader(c.assessmentModelCatalogClient))),
			catalogCaches.typology,
			catalogL1SingleflightEnabled(c.opts, catalogKindTypology),
		),
	}
	c.l1PeekRegistry = catalogpeek.NewRegistry()
	catalogpeek.RegisterCatalogL1(c.l1PeekRegistry, rt.typology, rt.questionnaire)
	return rt
}

func (c *Container) buildReportRuntime(evaluationQuery *evaluation.QueryService) reportRuntime {
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

	notifier := reportnotify.NewInMemoryNotifier()
	var signaler *signalredis.Signaler[reportstatus.ChangedSignal]
	if reporter != nil {
		signaler = reporter.Signaler()
	}
	waitReport := reportwait.NewService(
		evaluationQuery,
		reportwait.NewStatusCache(reportstatus.NewCache(c.opsHandle)),
		notifier,
		signaler,
		cfg,
	)

	var cancel context.CancelFunc
	if cfg.SignalingEnabled && signaler != nil {
		watchCtx, watchCancel := context.WithCancel(context.Background())
		waitReport.StartSignalWatcher(watchCtx)
		cancel = watchCancel
	}

	return reportRuntime{
		reporter:          reporter,
		notifier:          notifier,
		waitReport:        waitReport,
		waitWatcherCancel: cancel,
	}
}

func (c *Container) buildReportEventsHandler() *ws.ReportEventsHandler {
	return ws.NewReportEventsHandler(ws.Dependencies{
		Notifier: c.reportNotifier,
		Events: reportevents.NewService(newReportStatusResolver(
			c.evaluationQueryService,
			c.waitReportService,
			c.typologyAssessmentQueryService,
		)),
		Options:      c.opts.ReportEvents,
		RateLimit:    c.RateLimitBackend(),
		RateLimitCfg: c.opts.RateLimit,
	})
}
