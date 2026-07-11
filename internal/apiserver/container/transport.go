package container

import (
	auth "github.com/FangcunMount/iam/v2/pkg/sdk/auth/verifier"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	operatorApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	consistencyApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/consistency"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	reportqueryjourney "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/reportquery"
	reportwaitjourney "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/reportwait"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	answersheetApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	systemgovApp "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	workbenchApp "github.com/FangcunMount/qs-server/internal/apiserver/application/workbench"
	platformmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/platform"
	statmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/statistics"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"
	resttransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	grpcpkg "github.com/FangcunMount/qs-server/internal/pkg/grpc"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

func (c *Container) BuildRESTDeps(rateCfg *options.RateLimitOptions) resttransport.Deps {
	deps := resttransport.Deps{RateLimit: rateCfg}
	if c == nil {
		return deps
	}

	platformDeps := platformmod.ExportRESTIntegrationDeps(platformmod.RESTIntegrationDeps{
		CodesService:            c.CodesService,
		QRCodeObjectStore:       c.QRCodeObjectStore,
		QRCodeObjectKeyPrefix:   c.QRCodeObjectKeyPrefix,
		GovernanceStatusService: c.CacheGovernanceStatusService(),
		EventStatusService:      c.buildRESTEventStatusService(),
		Backpressure:            c.buildBackpressureSnapshots(),
		IAM:                     c.exportRESTIAMDeps(),
	})
	deps.CodesService = platformDeps.CodesService
	deps.QRCodeObjectStore = platformDeps.QRCodeObjectStore
	deps.QRCodeObjectKeyPrefix = platformDeps.QRCodeObjectKeyPrefix
	deps.GovernanceStatusService = platformDeps.GovernanceStatusService
	deps.EventStatusService = platformDeps.EventStatusService
	deps.Backpressure = platformDeps.Backpressure
	deps.IAM = platformDeps.IAM

	if c.SurveyModule != nil {
		deps.Survey = c.SurveyModule.ExportRESTDeps(surveymod.RESTExportOptions{
			QRCodeService: c.QRCodeService,
		})
	}
	if c.AssessmentModelModule != nil {
		exports := c.AssessmentModelModule.ExportRESTDeps(c.QRCodeService, c.CodesService, deps.Survey.QuestionnaireQueryService)
		deps.AssessmentModel = exports.AssessmentModel
	}
	if c.ActorModule != nil {
		deps.Actor = c.ActorModule.ExportRESTDeps(c.QRCodeService)
	}
	if c.EvaluationModule != nil {
		deps.Evaluation = c.EvaluationModule.ExportRESTDeps()
		if c.ActorModule != nil {
			deps.Actor.TesteeScaleAnalysisService = c.EvaluationModule.ExportTesteeScaleAnalysisService()
		}
	}
	if c.EvaluationModule != nil && c.ReportModule != nil {
		reportQuery := reportqueryjourney.NewService(c.EvaluationModule.AccessQueryService, c.ReportModule.QueryService)
		deps.Interpretation.ReportQueryJourney = reportQuery
		deps.Interpretation.ReportWaitJourney = reportwaitjourney.NewService(
			c.EvaluationModule.AccessQueryService,
			c.EvaluationModule.WorkerResultReader,
			reportQuery,
		)
	}
	if c.PlanModule != nil {
		var testeeAccess actorAccessApp.TesteeAccessService
		if c.ActorModule != nil {
			testeeAccess = c.ActorModule.TesteeAccessService
		}
		deps.Plan = c.PlanModule.ExportRESTDeps(testeeAccess)
	}
	deps.Workbench = composeRESTWorkbenchDeps(c)
	if c.StatisticsModule != nil {
		var testeeAccess statmod.RESTExportOptions
		if c.ActorModule != nil {
			testeeAccess.TesteeAccessService = c.ActorModule.TesteeAccessService
		}
		testeeAccess.WarmupCoordinator = c.WarmupCoordinator()
		testeeAccess.CacheGovernanceStatusService = c.CacheGovernanceStatusService()
		deps.Statistics = c.StatisticsModule.ExportRESTDeps(testeeAccess)
	}

	deps.SystemGovernanceFacade = c.buildRESTSystemGovernanceFacade(rateCfg, deps.Statistics)

	return deps
}

func (c *Container) buildRESTSystemGovernanceFacade(rateCfg *options.RateLimitOptions, statisticsDeps resttransport.StatisticsDeps) systemgovApp.Facade {
	if c == nil {
		return platformmod.BuildRESTSystemGovernanceFacade(platformmod.RESTSystemGovernanceInput{})
	}
	eventStatus := c.buildRESTEventStatusService()
	outboxes := make([]appEventing.NamedOutboxStatusReader, 0, 2)
	if c.SurveyModule != nil {
		outboxes = append(outboxes, c.SurveyModule.ExportRESTEventStatusOutbox())
	}
	if c.EvaluationModule != nil {
		outboxes = append(outboxes, c.EvaluationModule.ExportRESTEventStatusOutbox())
	}
	cacheGovernance := statisticsApp.NewGovernanceFacade(
		"apiserver",
		statisticsDeps.WarmupCoordinator,
		statisticsDeps.CacheGovernanceStatusService,
	)
	return platformmod.BuildRESTSystemGovernanceFacade(platformmod.RESTSystemGovernanceInput{
		Options:            c.systemGovernanceOptions,
		EventStatusService: eventStatus,
		EventOutboxes:      outboxes,
		CacheGovernance:    cacheGovernance,
		MySQLDB:            c.mysqlDB,
		LocalResilienceSnapshot: platformmod.BuildLocalResilienceSnapshot(
			"apiserver",
			rateCfg != nil && rateCfg.Enabled,
			c.buildBackpressureSnapshots(),
		),
	})
}

func (c *Container) buildRESTEventStatusService() appEventing.StatusService {
	if c == nil {
		return platformmod.BuildRESTEventStatusService(platformmod.RESTEventStatusInput{})
	}
	input := platformmod.RESTEventStatusInput{Catalog: c.eventCatalog}
	if c.SurveyModule != nil {
		input.SurveyAnswerSheetOutbox = c.SurveyModule.ExportRESTEventStatusOutbox()
	}
	if c.EvaluationModule != nil {
		input.EvaluationAssessmentOutbox = c.EvaluationModule.ExportRESTEventStatusOutbox()
	}
	return platformmod.BuildRESTEventStatusService(input)
}

func (c *Container) exportRESTIAMDeps() platformmod.RESTIAMDeps {
	deps := platformmod.RESTIAMDeps{}
	if c == nil || c.IAMModule == nil {
		return deps
	}
	deps.Enabled = c.IAMModule.IsEnabled()
	deps.TokenVerifier = c.IAMModule.SDKTokenVerifier()
	deps.SnapshotLoader = c.IAMModule.AuthzSnapshotLoader()
	if client := c.IAMModule.Client(); client != nil && client.Config() != nil && client.Config().JWT != nil {
		deps.ForceRemoteVerification = client.Config().JWT.ForceRemoteVerification
	}
	return deps
}

type backpressureSnapshotter interface {
	Snapshot(name string) resilienceplane.BackpressureSnapshot
}

func (c *Container) buildBackpressureSnapshots() []resilienceplane.BackpressureSnapshot {
	if c == nil {
		return nil
	}
	return []resilienceplane.BackpressureSnapshot{
		backpressureSnapshot("mysql", c.backpressure.MySQL),
		backpressureSnapshot("mongo", c.backpressure.Mongo),
		backpressureSnapshot("iam", c.backpressure.IAM),
	}
}

func backpressureSnapshot(name string, limiter interface{}) resilienceplane.BackpressureSnapshot {
	if snapshotter, ok := limiter.(backpressureSnapshotter); ok {
		return snapshotter.Snapshot(name)
	}
	return resilienceplane.BackpressureSnapshot{
		Component:  "apiserver",
		Name:       name,
		Dependency: name,
		Strategy:   "semaphore",
		Enabled:    false,
		Reason:     "backpressure disabled",
	}
}

func (c *Container) BuildGRPCDeps(server *grpcpkg.Server) grpctransport.Deps {
	deps := grpctransport.Deps{Server: server}
	if c == nil {
		return deps
	}

	platformDeps := platformmod.ExportGRPCIntegrationDeps(platformmod.GRPCIntegrationDeps{
		WarmupCoordinator:                  c.WarmupCoordinator(),
		QRCodeService:                      c.QRCodeService,
		MiniProgramTaskNotificationService: c.MiniProgramTaskNotificationService,
		AuthzSnapshotLoader:                c.exportGRPCAuthzSnapshotLoader(),
		PublishedModelCatalog:              c.exportGRPCPublishedModelCatalog(),
	})
	deps.WarmupCoordinator = platformDeps.WarmupCoordinator
	deps.QRCodeService = platformDeps.QRCodeService
	deps.MiniProgramTaskNotificationService = platformDeps.MiniProgramTaskNotificationService
	deps.IAM = platformDeps.IAM
	deps.PublishedModelCatalog = platformDeps.PublishedModelCatalog

	if c.SurveyModule != nil {
		deps.Survey = c.SurveyModule.ExportGRPCDeps()
	}
	if c.ActorModule != nil {
		deps.Actor = c.ActorModule.ExportGRPCDeps()
	}
	if c.EvaluationModule != nil {
		deps.Evaluation = c.EvaluationModule.ExportGRPCDeps()
	}
	if c.ReportModule != nil {
		deps.Interpretation = c.ReportModule.ExportGRPCDeps()
	}
	if c.AssessmentModelModule != nil {
		exports := c.AssessmentModelModule.ExportGRPCDeps()
		deps.AssessmentModelCatalog = exports.AssessmentModelCatalog
	}
	if c.PlanModule != nil {
		deps.Plan = c.PlanModule.ExportGRPCDeps()
	}
	if c.StatisticsModule != nil {
		deps.Statistics = c.StatisticsModule.ExportGRPCDeps()
	}

	return deps
}

func (c *Container) exportGRPCAuthzSnapshotLoader() *iaminfra.AuthzSnapshotLoader {
	if c == nil || c.IAMModule == nil {
		return nil
	}
	return c.IAMModule.AuthzSnapshotLoader()
}

func (c *Container) exportGRPCPublishedModelCatalog() rulesetport.Catalog {
	if c == nil {
		return nil
	}
	if c.publishedModelCatalog != nil {
		return c.publishedModelCatalog
	}
	catalog, err := c.ensurePublishedModelCatalog()
	if err != nil {
		return nil
	}
	if catalog != nil {
		c.publishedModelCatalog = catalog
	}
	return catalog
}

func composeRESTWorkbenchDeps(c *Container) resttransport.WorkbenchDeps {
	deps := resttransport.WorkbenchDeps{}
	if c == nil || c.ActorModule == nil || c.EvaluationModule == nil || c.PlanModule == nil {
		return deps
	}
	if c.ActorModule.OperatorQueryService == nil ||
		c.ActorModule.ClinicianQueryService == nil ||
		c.ActorModule.ClinicianRelationshipService == nil ||
		c.ActorModule.ReadModel == nil ||
		c.EvaluationModule.LatestRiskReader == nil ||
		c.PlanModule.FollowUpQueueReader == nil {
		return deps
	}
	deps.WorkbenchService = workbenchApp.NewService(
		c.ActorModule.OperatorQueryService,
		c.ActorModule.ClinicianQueryService,
		c.ActorModule.ClinicianRelationshipService,
		c.ActorModule.ReadModel,
		c.ActorModule.ReadModel,
		c.EvaluationModule.LatestRiskReader,
		c.PlanModule.FollowUpQueueReader,
	)
	return deps
}

// ServerGRPCBootstrapDeps describes the narrow container-owned dependencies
// needed to build the process gRPC server.
type ServerGRPCBootstrapDeps struct {
	AuthzSnapshotLoader           *iaminfra.AuthzSnapshotLoader
	OperatorRoleProjectionUpdater operatorApp.OperatorRoleProjectionUpdater
	ActiveOperatorChecker         operatorApp.ActiveOperatorChecker
	TokenVerifier                 *auth.TokenVerifier
}

// ServerRuntimeDeps describes the narrow container-owned dependencies needed by
// background runtimes started from the apiserver process.
type ServerRuntimeDeps struct {
	LockBuilder                           *keyspace.Builder
	LockManager                           locklease.Manager
	WarmupCoordinator                     cachegov.Coordinator
	PlanCommandService                    planApp.PlanCommandService
	StatisticsSyncService                 statisticsApp.StatisticsSyncService
	BehaviorProjectorService              statisticsApp.BehaviorProjectorService
	BehaviorJourneyScanService            statisticsApp.BehaviorJourneyScanService
	EvaluationConsistencyReconcileService consistencyApp.Service
	AnswerSheetSubmittedRelay             answersheetApp.SubmittedEventRelay
	AssessmentOutboxRelay                 appEventing.OutboxRelay
}

func (c *Container) BuildServerGRPCBootstrapDeps() ServerGRPCBootstrapDeps {
	var deps ServerGRPCBootstrapDeps
	if c == nil {
		return deps
	}
	if c.IAMModule != nil {
		deps.AuthzSnapshotLoader = c.IAMModule.AuthzSnapshotLoader()
		deps.TokenVerifier = c.IAMModule.SDKTokenVerifier()
	}
	if c.ActorModule != nil {
		deps.OperatorRoleProjectionUpdater = c.ActorModule.OperatorRoleProjectionUpdater
		deps.ActiveOperatorChecker = c.ActorModule.ActiveOperatorChecker
	}
	return deps
}

func (c *Container) BuildServerRuntimeDeps() ServerRuntimeDeps {
	var deps ServerRuntimeDeps
	if c == nil {
		return deps
	}

	deps.LockBuilder = c.CacheBuilder(cacheplane.FamilyLock)
	deps.LockManager = c.CacheLockManager()
	deps.WarmupCoordinator = c.WarmupCoordinator()

	if c.PlanModule != nil {
		deps.PlanCommandService = c.PlanModule.CommandService
	}
	if c.StatisticsModule != nil {
		deps.StatisticsSyncService = c.StatisticsModule.SyncService
		deps.BehaviorProjectorService = c.StatisticsModule.BehaviorProjectorService
		deps.BehaviorJourneyScanService = c.StatisticsModule.BehaviorJourneyScanService
	}
	if c.SurveyModule != nil && c.SurveyModule.AnswerSheet != nil {
		deps.AnswerSheetSubmittedRelay = c.SurveyModule.AnswerSheet.SubmittedEventRelay
	}
	if c.EvaluationModule != nil {
		deps.AssessmentOutboxRelay = c.EvaluationModule.AssessmentOutboxRelay
		deps.EvaluationConsistencyReconcileService = c.EvaluationModule.ConsistencyReconcileService
	}

	return deps
}
