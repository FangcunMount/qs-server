package interpretation

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"

	"github.com/FangcunMount/component-base/pkg/errors"
	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	interpretationadmin "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/administration"
	aiexplanationadministration "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/administration"
	aiexplanationevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	aiexplanationexecution "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/execution"
	aiexplanationgovernance "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/governance"
	aiexplanationparticipant "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/participant"
	aiexplanationpersistence "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/persistence"
	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	aiexplanationrecovery "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/recovery"
	aiexplanationsource "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/source"
	aiexplanationsubjectexport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/subjectexport"
	interpretationautomation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation"
	interpretationexecution "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	interpretationcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/catalogreconcile"
	interpretationclinician "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/clinician"
	interpretationoperations "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/operations"
	interpretationparticipant "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/participant"
	interpretationreadmission "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/readmission"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reportprojection"
	appreporttemplate "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporttemplate"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	interpretationadmission "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/admission"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	aiexplanationevents "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/events"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	interpretationbuilder "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/builder"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rendering"
	domainreporttemplate "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/reporttemplate"
	aiexplanationprompt "github.com/FangcunMount/qs-server/internal/apiserver/infra/aiexplanation/prompt"
	aiexplanationprovider "github.com/FangcunMount/qs-server/internal/apiserver/infra/aiexplanation/provider"
	aiexplanationresponsesapi "github.com/FangcunMount/qs-server/internal/apiserver/infra/aiexplanation/responsesapi"
	aiexplanationsafety "github.com/FangcunMount/qs-server/internal/apiserver/infra/aiexplanation/safety"
	aiexplanationschema "github.com/FangcunMount/qs-server/internal/apiserver/infra/aiexplanation/schema"
	aiexplanationsemantic "github.com/FangcunMount/qs-server/internal/apiserver/infra/aiexplanation/semantic"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongoEval "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	mongoAIExplanation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation/aiexplanation"
	mysqlEval "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/interpretation"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	evaluationreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	mysqlBase "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"gorm.io/gorm"
)

// Module assembles report read/query, builder-registry, and durable write capabilities.
type Module struct {
	reader                      evaluationreadmodel.ReportReader
	reportCatalog               evaluationreadmodel.BatchReportMetadataReader
	executionExecutor           interpretationexecution.Executor
	generationRepo              *mongoEval.GenerationRepository
	runRepo                     *mongoEval.RunRepository
	reportRepo                  *mongoEval.ReportRepository
	admissionRepo               interpretationadmission.QueryRepository
	reportTemplateRepo          *mongoEval.ReportTemplateRepository
	reportTemplateService       appreporttemplate.Service
	automationService           interpretationautomation.Service
	projectionMapper            reportprojection.Mapper
	participantService          interpretationparticipant.Service
	administrationService       interpretationadmin.Service
	clinicianService            interpretationclinician.Service
	operationsService           interpretationoperations.Service
	catalogReconcile            interpretationcatalog.Service
	catalogAudit                interpretationcatalog.RunnerService
	governedRetryService        interpretationautomation.GovernedRetryService
	readmissionService          interpretationreadmission.Service
	leaseRecoverer              interpretationautomation.LeaseRecoverer
	aiExplanationEnabled        bool
	aiExplanationExecutor       aiexplanationexecution.Executor
	aiExplanationService        aiexplanationparticipant.Service
	aiGenerationRepo            *mongoAIExplanation.GenerationRepository
	aiRunRepo                   *mongoAIExplanation.RunRepository
	aiArtifactRepo              *mongoAIExplanation.ArtifactRepository
	aiProfileRepo               *mongoAIExplanation.ProfileRepository
	aiEvaluationRepo            *mongoAIExplanation.PromptEvaluationRepository
	aiEvaluationBudgetRepo      *mongoAIExplanation.PromptEvaluationBudgetRepository
	aiParticipantBudgetRepo     *mongoAIExplanation.ParticipantBudgetRepository
	aiParticipantActiveRepo     *mongoAIExplanation.ParticipantActiveCapacityRepository
	aiEvidenceService           *aiexplanationevaluation.EvidenceService
	aiEvaluationCommitter       *aiexplanationevaluation.DurableCommitter
	aiEvaluationLeaseRecoverer  *aiexplanationevaluation.PreparedLeaseRecoverer
	aiParticipantLeaseRecoverer *aiexplanationrecovery.LeaseRecoverer
	aiSubjectExport             *aiexplanationsubjectexport.Service
	aiReviewService             *aiexplanationevaluation.ReviewService
	aiAdministration            aiexplanationadministration.Service
	aiOnlineEvalRunner          *aiexplanationevaluation.OnlineRunner
	aiGovernanceService         *aiexplanationgovernance.Service
	aiPromptCatalog             *aiexplanationprompt.Catalog
	aiRouteCatalog              *aiexplanationprovider.Catalog
	aiCommitter                 *aiexplanationpersistence.Committer
	aiOutcomeRepo               domainoutcome.Repository
	participantAccess           interpretationparticipant.Access
	txRunner                    apptransaction.Runner
	eventStager                 appEventing.EventStager
	ReportStatusReporter        *reportstatus.Reporter
}

// Deps defines explicit constructor dependencies for the report module.
type Deps struct {
	MySQLDB            *gorm.DB
	MySQLLimiter       backpressure.Acquirer
	MongoDB            *mongo.Database
	MongoLimiter       backpressure.Acquirer
	OpsHandle          *redisruntime.Handle
	ReportStatusConfig reportstatus.Config
	OutboxProfile      appEventing.ProfileBinding
	RunLeaseDuration   time.Duration
	AIExplanation      *apiserveroptions.AIExplanationOptions
}

// New assembles the report module.
func New(deps Deps) (*Module, error) {
	if deps.MySQLDB == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "MySQL database connection is nil or invalid")
	}
	if deps.MongoDB == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "MongoDB database connection is nil or invalid")
	}

	module := &Module{}
	reportStatusReporter, err := reportstatus.NewReporter(deps.OpsHandle, deps.ReportStatusConfig)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report status reporter: %v", err)
	}
	module.ReportStatusReporter = reportStatusReporter
	mongoOptions := mongoBase.BaseRepositoryOptions{Limiter: deps.MongoLimiter}
	reportReadModel := mongoEval.NewReportReadModel(deps.MongoDB, mongoOptions)
	module.reader = reportReadModel
	module.reportCatalog = reportReadModel
	catalogReconcileStore, err := mongoEval.NewCatalogReconcileStore(deps.MongoDB, mongoOptions)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report catalog reconcile store: %v", err)
	}
	catalogCheckpointRepo := mysqlEval.NewCatalogAuditCheckpointRepository(deps.MySQLDB)
	catalogStoreAdapter := catalogReconcileStoreAdapter{store: catalogReconcileStore, checkpoint: catalogCheckpointRepo}
	catalogService := interpretationcatalog.NewService(catalogStoreAdapter, catalogStoreAdapter)
	module.catalogReconcile = catalogService
	module.catalogAudit = catalogService
	generationRepo, err := mongoEval.NewGenerationRepository(deps.MongoDB, mongoOptions)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report generation repository: %v", err)
	}
	module.generationRepo = generationRepo
	runRepo, err := mongoEval.NewRunRepository(deps.MongoDB, mongoOptions)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize interpretation run repository: %v", err)
	}
	module.runRepo = runRepo
	reportRepo, err := mongoEval.NewReportRepository(deps.MongoDB, mongoOptions)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize interpretation report repository: %v", err)
	}
	module.reportRepo = reportRepo
	admissionRepo := mysqlEval.NewAdmissionFailureRepository(deps.MySQLDB, mysqlBase.BaseRepositoryOptions{Limiter: deps.MySQLLimiter})
	module.admissionRepo = admissionRepo
	reportTemplateManifests, err := rendering.NewBuiltinReleaseManifestCatalog()
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report template manifest catalog: %v", err)
	}
	reportTemplateRepo, err := mongoEval.NewReportTemplateRepository(deps.MongoDB, reportTemplateManifests, mongoOptions)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report template repository: %v", err)
	}
	module.reportTemplateRepo = reportTemplateRepo
	module.reportTemplateService = appreporttemplate.NewService(reportTemplateRepo, reportTemplateManifests)
	catalogProjector, err := mongoEval.NewReportCatalogProjector(deps.MongoDB, mongoOptions)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report catalog projector: %v", err)
	}

	if deps.OutboxProfile.Stager == nil || deps.OutboxProfile.PostCommit == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "mongo domain event profile is required")
	}
	startTxRunner := modtx.NewMongoRunner(deps.MongoDB, modtx.MongoRunnerOptions{
		Boundary: "interpretation_start",
		Limiter:  deps.MongoLimiter,
	})
	commitTxRunner := modtx.NewMongoRunner(deps.MongoDB, modtx.MongoRunnerOptions{
		Boundary: "interpretation_commit",
		Limiter:  deps.MongoLimiter,
	})
	retryTxRunner := modtx.NewMongoRunner(deps.MongoDB, modtx.MongoRunnerOptions{
		Boundary: "interpretation_retry",
		Limiter:  deps.MongoLimiter,
	})
	module.txRunner = retryTxRunner
	module.eventStager = deps.OutboxProfile.Stager
	{
		registry, err := buildReportBuilderRegistry()
		if err != nil {
			return nil, err
		}
		starter, err := interpretationexecution.NewStarter(startTxRunner, module.generationRepo, module.runRepo, module.reportRepo, deps.RunLeaseDuration)
		if err != nil {
			return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report generation starter: %v", err)
		}
		committer, err := interpretationexecution.NewInterpretationCommitter(commitTxRunner, module.generationRepo, module.runRepo, module.reportRepo, deps.OutboxProfile.Stager, deps.OutboxProfile.PostCommit, catalogProjector)
		if err != nil {
			return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize interpretation committer: %v", err)
		}
		executor, err := interpretationexecution.NewExecutor(starter, registry, committer)
		if err != nil {
			return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize interpretation execution: %v", err)
		}
		module.executionExecutor = executor
	}
	if err := module.assembleAIExplanation(deps, mongoOptions); err != nil {
		return nil, err
	}

	return module, nil
}

func (m *Module) assembleAIExplanation(deps Deps, mongoOptions mongoBase.BaseRepositoryOptions) error {
	config := deps.AIExplanation
	if config == nil || !config.Enabled {
		return nil
	}
	if validationErrors := config.Validate(); len(validationErrors) > 0 {
		return errors.WithCode(code.ErrModuleInitializationFailed, "invalid AI explanation runtime configuration: %v", validationErrors)
	}
	retentionPolicy := mongoAIExplanation.RetentionPolicy{
		Version:                    config.DataLifecycle.PolicyVersion,
		ParticipantRecordRetention: config.DataLifecycle.ParticipantRecordRetention,
		PromptEvaluationRetention:  config.DataLifecycle.PromptEvaluationRetention,
		CapacityLedgerRetention:    config.DataLifecycle.CapacityLedgerRetention,
	}
	profileRepo, err := mongoAIExplanation.NewProfileRepository(deps.MongoDB, mongoOptions)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation profile repository: %v", err)
	}
	evaluationRepo, err := mongoAIExplanation.NewPromptEvaluationRepository(deps.MongoDB, retentionPolicy, mongoOptions)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation Prompt evaluation repository: %v", err)
	}
	evaluationBudgetRepo, err := mongoAIExplanation.NewPromptEvaluationBudgetRepository(deps.MongoDB, retentionPolicy, mongoOptions)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation Prompt evaluation budget repository: %v", err)
	}
	governanceService, err := aiexplanationgovernance.NewService(profileRepo, evaluationRepo, time.Now)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation release governance: %v", err)
	}
	m.aiProfileRepo = profileRepo
	m.aiEvaluationRepo = evaluationRepo
	m.aiEvaluationBudgetRepo = evaluationBudgetRepo
	evidenceService, err := aiexplanationevaluation.NewEvidenceService(evaluationRepo, meta.New, time.Now)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation evaluation evidence service: %v", err)
	}
	m.aiEvidenceService = evidenceService
	reviewService, err := aiexplanationevaluation.NewReviewService(evidenceService)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation evaluation review service: %v", err)
	}
	m.aiReviewService = reviewService
	m.aiGovernanceService = governanceService
	m.aiAdministration = aiexplanationadministration.NewService(reviewService, governanceService, aiAdministrationAccessAdapter{})

	promptCatalog := aiexplanationprompt.NewCatalog()
	routeConfigs := []aiexplanationprovider.Config{{
		Route: apiserveroptions.DefaultAIExplanationProviderRoute, Revision: config.RouteRevision,
		Provider: config.Provider, Model: config.Model, Current: true,
		Capabilities: appport.ProviderCapabilities{StructuredOutput: true},
		Timeout:      config.Timeout, MaxOutputTokens: config.MaxOutputTokens,
	}}
	if config.Evaluation.Enabled {
		routeConfigs = append(routeConfigs, aiexplanationprovider.Config{
			Route: apiserveroptions.DefaultAIExplanationSemanticProviderRoute, Revision: config.Evaluation.RouteRevision,
			Provider: config.Provider, Model: config.Evaluation.Model, Current: true,
			Capabilities: appport.ProviderCapabilities{StructuredOutput: true},
			Timeout:      config.Evaluation.Timeout, MaxOutputTokens: config.Evaluation.MaxOutputTokens,
		})
	}
	routeCatalog, err := aiexplanationprovider.NewCatalog(routeConfigs)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation provider route: %v", err)
	}
	providerAdapter, err := aiexplanationresponsesapi.NewProvider(aiexplanationresponsesapi.Config{
		Provider: config.Provider, Endpoint: config.Endpoint, APIKey: config.APIKey, MaxResponseBytes: config.MaxResponseBytes,
	})
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation Responses API provider: %v", err)
	}
	generationRepo, err := mongoAIExplanation.NewGenerationRepository(deps.MongoDB, retentionPolicy, mongoOptions)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation generation repository: %v", err)
	}
	runRepo, err := mongoAIExplanation.NewRunRepository(deps.MongoDB, retentionPolicy, mongoOptions)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation run repository: %v", err)
	}
	artifactRepo, err := mongoAIExplanation.NewArtifactRepository(deps.MongoDB, retentionPolicy, mongoOptions)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation artifact repository: %v", err)
	}
	participantBudgetRepo, err := mongoAIExplanation.NewParticipantBudgetRepository(deps.MongoDB, retentionPolicy, mongoOptions)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation participant budget repository: %v", err)
	}
	participantActiveRepo, err := mongoAIExplanation.NewParticipantActiveCapacityRepository(deps.MongoDB, mongoOptions)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation participant active capacity repository: %v", err)
	}
	participantCapacityPolicy := domaingeneration.ParticipantCapacityPolicy{
		DailyProviderInvocationBudgetPerOrg:        config.ParticipantCapacity.DailyProviderInvocationBudgetPerOrg,
		DailyProviderInvocationBudgetPerUser:       config.ParticipantCapacity.DailyProviderInvocationBudgetPerUser,
		DailyProviderInvocationBudgetPerAssessment: config.ParticipantCapacity.DailyProviderInvocationBudgetPerAssessment,
		MaxActiveProviderExecutionsPerOrg:          config.ParticipantCapacity.MaxActiveProviderExecutionsPerOrg,
		MaxActiveProviderExecutionsPerUser:         config.ParticipantCapacity.MaxActiveProviderExecutionsPerUser,
		MaxActiveProviderExecutionsPerAssessment:   config.ParticipantCapacity.MaxActiveProviderExecutionsPerAssessment,
	}
	lifecycleTx := modtx.NewMongoRunner(deps.MongoDB, modtx.MongoRunnerOptions{Boundary: "ai_explanation_lifecycle", Limiter: deps.MongoLimiter})
	committer, err := aiexplanationpersistence.NewCommitter(
		lifecycleTx,
		generationRepo, runRepo, artifactRepo, participantBudgetRepo, participantActiveRepo, participantCapacityPolicy,
		aiexplanationevents.Factory{}, deps.OutboxProfile.Stager, deps.OutboxProfile.PostCommit,
	)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation committer: %v", err)
	}
	subjectExport, err := aiexplanationsubjectexport.NewService(artifactRepo, time.Now)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation subject export: %v", err)
	}
	recoveryService, err := aiexplanationrecovery.NewService(generationRepo, runRepo, committer, time.Now)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation participant recovery: %v", err)
	}
	participantLeaseRecoverer, err := aiexplanationrecovery.NewLeaseRecoverer(runRepo, generationRepo, runRepo, committer)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation participant lease recovery: %v", err)
	}
	safetyGate := aiexplanationsafety.NewDeterministicGate()
	executor, err := aiexplanationexecution.NewExecutor(
		generationRepo, runRepo, artifactRepo, profileRepo, promptCatalog, routeCatalog,
		aiexplanationschema.NewCatalog(), providerAdapter, safetyGate, committer, config.RunLeaseDuration,
	)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation executor: %v", err)
	}

	m.aiExplanationEnabled = true
	m.aiExplanationExecutor = executor
	m.aiGenerationRepo = generationRepo
	m.aiRunRepo = runRepo
	m.aiArtifactRepo = artifactRepo
	m.aiParticipantBudgetRepo = participantBudgetRepo
	m.aiParticipantActiveRepo = participantActiveRepo
	m.aiSubjectExport = subjectExport
	m.aiPromptCatalog = promptCatalog
	m.aiRouteCatalog = routeCatalog
	m.aiCommitter = committer
	m.aiParticipantLeaseRecoverer = participantLeaseRecoverer
	m.aiAdministration = aiexplanationadministration.NewService(
		reviewService, governanceService, aiAdministrationAccessAdapter{},
		aiexplanationadministration.WithParticipantCapacity(participantBudgetRepo, participantActiveRepo, participantCapacityPolicy, time.Now),
		aiexplanationadministration.WithParticipantRecovery(recoveryService),
	)
	if config.Evaluation.Enabled {
		evaluationCommitter, err := aiexplanationevaluation.NewDurableCommitter(
			modtx.NewMongoRunner(deps.MongoDB, modtx.MongoRunnerOptions{Boundary: "ai_explanation_prompt_evaluation", Limiter: deps.MongoLimiter}),
			evaluationRepo, aiexplanationevents.Factory{}, deps.OutboxProfile.Stager, deps.OutboxProfile.PostCommit,
			evaluationBudgetRepo, config.Evaluation.Capacity.DailyProviderInvocationBudgetPerOrg, time.Now,
		)
		if err != nil {
			return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation evaluation committer: %v", err)
		}
		evaluationLeaseRecoverer, err := aiexplanationevaluation.NewPreparedLeaseRecoverer(evaluationRepo, evaluationCommitter)
		if err != nil {
			return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation Prompt evaluation lease recovery: %v", err)
		}
		semanticRoute, err := routeCatalog.ResolveProviderRoute(context.Background(), apiserveroptions.DefaultAIExplanationSemanticProviderRoute)
		if err != nil {
			return errors.WithCode(code.ErrModuleInitializationFailed, "failed to resolve AI explanation semantic evaluator route: %v", err)
		}
		semanticEvaluator, err := aiexplanationsemantic.NewEvaluator(providerAdapter, semanticRoute)
		if err != nil {
			return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation semantic evaluator: %v", err)
		}
		onlineRunner, err := aiexplanationevaluation.NewOnlineRunner(aiexplanationevaluation.OnlineRunnerDependencies{
			Prompts: promptCatalog, Schemas: aiexplanationschema.NewCatalog(), Routes: routeCatalog,
			Provider: providerAdapter, Safety: safetyGate, Semantic: semanticEvaluator,
			SemanticTimeout: config.Evaluation.Timeout, Evidence: evidenceService,
			DurableCommitter: evaluationCommitter, Now: time.Now,
		})
		if err != nil {
			return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation online evaluation runner: %v", err)
		}
		m.aiOnlineEvalRunner = onlineRunner
		m.aiEvaluationCommitter = evaluationCommitter
		m.aiEvaluationLeaseRecoverer = evaluationLeaseRecoverer
		m.aiAdministration = aiexplanationadministration.NewService(
			reviewService, governanceService, aiAdministrationAccessAdapter{},
			aiexplanationadministration.WithParticipantCapacity(participantBudgetRepo, participantActiveRepo, participantCapacityPolicy, time.Now),
			aiexplanationadministration.WithParticipantRecovery(recoveryService),
			aiexplanationadministration.WithEvaluationExecution(onlineRunner, evaluationCommitter, meta.New),
			aiexplanationadministration.WithEvaluationCapacity(
				evaluationBudgetRepo, config.Evaluation.Capacity.MaxActiveRunsPerOrg,
				config.Evaluation.Capacity.DailyProviderInvocationBudgetPerOrg, time.Now,
			),
		)
	}
	return nil
}

// AIExplanationGovernance exposes the operator-only release service when the
// AI explanation feature is enabled. Online evaluation execution may remain
// disabled; existing evaluation evidence can still be reviewed and used by
// the authenticated administration transport.
func (m *Module) AIExplanationGovernance() *aiexplanationgovernance.Service {
	if m == nil {
		return nil
	}
	return m.aiGovernanceService
}

// AIExplanationEvaluationEvidence exposes the operator-only evidence recorder.
// It does not invoke a Provider; the online runner and administration
// transport use this CAS-protected boundary.
func (m *Module) AIExplanationEvaluationEvidence() *aiexplanationevaluation.EvidenceService {
	if m == nil {
		return nil
	}
	return m.aiEvidenceService
}

// AIExplanationEvaluationReview exposes the operator-only synthetic review
// workflow. The caller must authenticate the reviewer, authorize the selected
// review role and keep this boundary out of participant-facing transports.
func (m *Module) AIExplanationEvaluationReview() *aiexplanationevaluation.ReviewService {
	if m == nil {
		return nil
	}
	return m.aiReviewService
}

// AIExplanationAdministration exposes the authenticated operator workflow for
// synthetic evidence review and Profile lifecycle governance. Provider
// execution remains a separate durable-operation concern.
func (m *Module) AIExplanationAdministration() aiexplanationadministration.Service {
	if m == nil {
		return nil
	}
	return m.aiAdministration
}

// AIExplanationSubjectExport exposes the participant-authorized, paginated
// projection of immutable final artifacts. The transport must authorize the
// Testee before invoking this boundary.
func (m *Module) AIExplanationSubjectExport() *aiexplanationsubjectexport.Service {
	if m == nil {
		return nil
	}
	return m.aiSubjectExport
}

// AIExplanationOnlineEvaluation exposes the operator-only 35-attempt release
// runner when the independent evaluator Route is explicitly enabled. It never
// finalizes evidence or publishes a Profile.
func (m *Module) AIExplanationOnlineEvaluation() *aiexplanationevaluation.OnlineRunner {
	if m == nil {
		return nil
	}
	return m.aiOnlineEvalRunner
}

// AIExplanationPromptEvaluationLeaseRecoverer exposes the scanner boundary
// only when the operator evaluation runtime is explicitly enabled.
func (m *Module) AIExplanationPromptEvaluationLeaseRecoverer() *aiexplanationevaluation.PreparedLeaseRecoverer {
	if m == nil {
		return nil
	}
	return m.aiEvaluationLeaseRecoverer
}

// AIExplanationParticipantLeaseRecoverer exposes the default-off scheduler
// boundary that persists exact lease wake-ups without invoking a Provider.
func (m *Module) AIExplanationParticipantLeaseRecoverer() *aiexplanationrecovery.LeaseRecoverer {
	if m == nil {
		return nil
	}
	return m.aiParticipantLeaseRecoverer
}

// BindOutcomeRepository completes the cross-storage Interpretation use case
// after Evaluation has installed its canonical outcome repository.
func (m *Module) BindOutcomeRepository(repo domainoutcome.Repository) error {
	if m == nil || repo == nil || m.executionExecutor == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "interpretation outcome service dependencies are not configured")
	}
	automationService, err := interpretationautomation.NewService(repo, m.executionExecutor, m.admissionRepo)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize interpretation automation service: %v", err)
	}
	m.automationService = automationService
	m.readmissionService = interpretationreadmission.NewService(m.admissionRepo, repo, automationService)
	m.operationsService = interpretationoperations.NewService(
		outcomeCorrelationAdapter{repo: repo},
		m.generationRepo,
		m.runRepo,
		m.reportRepo,
		operationsAccessAdapter{},
		m.admissionRepo,
	)
	m.governedRetryService = interpretationautomation.NewGovernedRetryService(m.generationRepo, m.runRepo, repo, m.txRunner, m.eventStager)
	m.leaseRecoverer = interpretationautomation.NewLeaseRecoverer(m.runRepo, m.generationRepo, m.automationService)
	m.aiOutcomeRepo = repo
	if err := m.tryBindAIExplanationParticipant(); err != nil {
		return err
	}
	return nil
}

func (m *Module) ReadmissionService() interpretationreadmission.Service {
	if m == nil {
		return nil
	}
	return m.readmissionService
}

func (m *Module) LeaseRecoverer() interpretationautomation.LeaseRecoverer {
	if m == nil {
		return nil
	}
	return m.leaseRecoverer
}

func (m *Module) GovernedRetryService() interpretationautomation.GovernedRetryService {
	if m == nil {
		return nil
	}
	return m.governedRetryService
}

func (m *Module) OperationsService() interpretationoperations.Service {
	if m == nil {
		return nil
	}
	return m.operationsService
}

func (m *Module) CatalogReconcileService() interpretationcatalog.Service {
	if m == nil {
		return nil
	}
	return m.catalogReconcile
}

func (m *Module) CatalogAuditService() interpretationcatalog.RunnerService {
	if m == nil {
		return nil
	}
	return m.catalogAudit
}

type catalogReconcileStoreAdapter struct {
	store      *mongoEval.CatalogReconcileStore
	checkpoint auditCheckpointStore
}

type auditCheckpointStore interface {
	LoadAuditCheckpoint(context.Context) (interpretationcatalog.AuditCheckpoint, error)
	SaveAuditCheckpoint(context.Context, int64, interpretationcatalog.AuditCheckpoint) error
}

func (a catalogReconcileStoreAdapter) ListDrifts(
	ctx context.Context,
	filter interpretationcatalog.Filter,
	cursor string,
	limit int,
) (interpretationcatalog.DriftPage, error) {
	if a.store == nil {
		return interpretationcatalog.DriftPage{}, fmt.Errorf("catalog reconcile store is not configured")
	}
	page, err := a.store.ListDrifts(ctx, mongoEval.CatalogReconcileFilter{
		OrgID: filter.OrgID, AssessmentID: filter.AssessmentID, Kind: filter.Kind,
		SortAtAfter: filter.SortAtAfter, SortAtBefore: filter.SortAtBefore,
	}, cursor, limit)
	if err != nil {
		return interpretationcatalog.DriftPage{}, err
	}
	items := make([]interpretationcatalog.DriftItem, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, interpretationcatalog.DriftItem{
			CatalogID: item.CatalogID, ReportID: item.ReportID, AssessmentID: item.AssessmentID,
			Source: item.Source, Kind: item.Kind, Fields: item.Fields,
			ObservedState: item.ObservedState, Version: item.Version,
		})
	}
	return interpretationcatalog.DriftPage{Items: items, NextCursor: page.NextCursor}, nil
}

func (a catalogReconcileStoreAdapter) VerifyAuditIndexes(ctx context.Context) error {
	if a.store == nil {
		return fmt.Errorf("catalog audit store is not configured")
	}
	return a.store.VerifyAuditIndexes(ctx)
}

func (a catalogReconcileStoreAdapter) LoadAuditCheckpoint(ctx context.Context) (interpretationcatalog.AuditCheckpoint, error) {
	if a.checkpoint == nil {
		return interpretationcatalog.AuditCheckpoint{}, fmt.Errorf("catalog audit checkpoint store is not configured")
	}
	return a.checkpoint.LoadAuditCheckpoint(ctx)
}

func (a catalogReconcileStoreAdapter) SaveAuditCheckpoint(ctx context.Context, expectedRevision int64, checkpoint interpretationcatalog.AuditCheckpoint) error {
	if a.checkpoint == nil {
		return fmt.Errorf("catalog audit checkpoint store is not configured")
	}
	return a.checkpoint.SaveAuditCheckpoint(ctx, expectedRevision, checkpoint)
}

func (a catalogReconcileStoreAdapter) LoadAuditUpperBounds(ctx context.Context, maxTime time.Duration) (interpretationcatalog.AuditUpperBounds, error) {
	bounds, err := a.store.LoadAuditUpperBounds(ctx, maxTime)
	return interpretationcatalog.AuditUpperBounds{SourceAssessmentID: bounds.SourceAssessmentID, CatalogAssessmentID: bounds.CatalogAssessmentID}, err
}

func (a catalogReconcileStoreAdapter) ScanAuditBatch(ctx context.Context, request interpretationcatalog.AuditBatchRequest) (interpretationcatalog.AuditBatchResult, error) {
	result, err := a.store.ScanAuditBatch(ctx, mongoEval.CatalogAuditBatchRequest{
		Phase: request.Phase, AfterAssessmentID: request.AfterAssessmentID,
		UpperAssessmentID: request.UpperAssessmentID, Limit: request.Limit, MaxTime: request.MaxTime,
	})
	if err != nil {
		return interpretationcatalog.AuditBatchResult{}, err
	}
	return interpretationcatalog.AuditBatchResult{
		NextAssessmentID: result.NextAssessmentID, Scanned: result.Scanned, Exhausted: result.Exhausted,
		Counts: driftCountsFromMongo(result.Counts), OrgCounts: orgDriftCountsFromMongo(result.OrgCounts),
	}, nil
}

func driftCountsFromMongo(source mongoEval.CatalogDriftCounts) interpretationcatalog.DriftCounts {
	return interpretationcatalog.DriftCounts{Missing: source.Missing, Dangling: source.Dangling, AssociationMismatch: source.AssociationMismatch, WrongWinner: source.WrongWinner}
}

func orgDriftCountsFromMongo(source map[int64]mongoEval.CatalogDriftCounts) map[int64]interpretationcatalog.DriftCounts {
	result := make(map[int64]interpretationcatalog.DriftCounts, len(source))
	for orgID, counts := range source {
		result[orgID] = driftCountsFromMongo(counts)
	}
	return result
}

func (a catalogReconcileStoreAdapter) SaveRepairPlan(ctx context.Context, plan interpretationcatalog.RepairPlan) error {
	return a.store.SaveRepairPlan(ctx, mongoEval.CatalogRepairPlan{
		DryRunID: plan.DryRunID, OrgID: plan.OrgID,
		Item: mongoEval.CatalogDriftItem{
			CatalogID: plan.Item.CatalogID, ReportID: plan.Item.ReportID, AssessmentID: plan.Item.AssessmentID,
			Source: plan.Item.Source, Kind: plan.Item.Kind, Fields: plan.Item.Fields,
			ObservedState: plan.Item.ObservedState, Version: plan.Item.Version,
		},
		CreatedAt: plan.CreatedAt, ExpiresAt: plan.ExpiresAt,
	})
}

func (a catalogReconcileStoreAdapter) FindRepairPlan(ctx context.Context, dryRunID string) (interpretationcatalog.RepairPlan, error) {
	plan, err := a.store.FindRepairPlan(ctx, dryRunID)
	if err != nil {
		return interpretationcatalog.RepairPlan{}, err
	}
	return interpretationcatalog.RepairPlan{
		DryRunID: plan.DryRunID, OrgID: plan.OrgID,
		Item: interpretationcatalog.DriftItem{
			CatalogID: plan.Item.CatalogID, ReportID: plan.Item.ReportID, AssessmentID: plan.Item.AssessmentID,
			Source: plan.Item.Source, Kind: plan.Item.Kind, Fields: plan.Item.Fields,
			ObservedState: plan.Item.ObservedState, Version: plan.Item.Version,
		},
		CreatedAt: plan.CreatedAt, ExpiresAt: plan.ExpiresAt,
	}, nil
}

func (a catalogReconcileStoreAdapter) ApplyRepair(ctx context.Context, plan interpretationcatalog.RepairPlan) (string, error) {
	return a.store.ApplyRepair(ctx, mongoEval.CatalogRepairPlan{
		DryRunID: plan.DryRunID, OrgID: plan.OrgID,
		Item: mongoEval.CatalogDriftItem{
			CatalogID: plan.Item.CatalogID, ReportID: plan.Item.ReportID, AssessmentID: plan.Item.AssessmentID,
			Source: plan.Item.Source, Kind: plan.Item.Kind, Fields: plan.Item.Fields,
			ObservedState: plan.Item.ObservedState, Version: plan.Item.Version,
		},
		CreatedAt: plan.CreatedAt, ExpiresAt: plan.ExpiresAt,
	})
}

func (m *Module) ReportReader() evaluationreadmodel.ReportReader {
	if m == nil {
		return nil
	}
	return m.reader
}

// outcomeCorrelationAdapter keeps Evaluation outcome types inside the
// composition root so application/interpretation does not import them.
type outcomeCorrelationAdapter struct {
	repo domainoutcome.Repository
}

func (a outcomeCorrelationAdapter) FindOutcomeByAssessmentID(ctx context.Context, assessmentID meta.ID) (interpretationoperations.OutcomeRef, error) {
	if a.repo == nil {
		return interpretationoperations.OutcomeRef{}, fmt.Errorf("evaluation outcome repository is not configured")
	}
	record, err := a.repo.FindByAssessmentID(ctx, assessmentID)
	if err != nil {
		return interpretationoperations.OutcomeRef{}, err
	}
	if record == nil {
		return interpretationoperations.OutcomeRef{}, fmt.Errorf("evaluation outcome not found for assessment %d", assessmentID.Uint64())
	}
	return interpretationoperations.OutcomeRef{ID: record.ID(), AssessmentID: record.AssessmentID(), OrgID: record.OrgID()}, nil
}

func (a outcomeCorrelationAdapter) FindOutcomeByID(ctx context.Context, id meta.ID) (interpretationoperations.OutcomeRef, error) {
	if a.repo == nil {
		return interpretationoperations.OutcomeRef{}, fmt.Errorf("evaluation outcome repository is not configured")
	}
	record, err := a.repo.FindByID(ctx, id)
	if err != nil {
		return interpretationoperations.OutcomeRef{}, err
	}
	return interpretationoperations.OutcomeRef{ID: record.ID(), AssessmentID: record.AssessmentID(), OrgID: record.OrgID()}, nil
}

type operationsAccessAdapter struct{}

func (operationsAccessAdapter) AuthorizeAudit(ctx context.Context, actor interpretationoperations.Actor, resourceOrgID int64) error {
	if actor.OrgID != resourceOrgID {
		return errors.WithCode(code.ErrPermissionDenied, "interpretation resource is outside current organization")
	}
	snapshot, ok := authzapp.FromContext(ctx)
	if !ok || !authzapp.DecideCapability(snapshot, authzapp.CapabilityAuditInterpretation).Allowed {
		return errors.WithCode(code.ErrPermissionDenied, "interpretation audit permission is required")
	}
	return nil
}

type aiAdministrationAccessAdapter struct{}

func (aiAdministrationAccessAdapter) AuthorizeRead(ctx context.Context, actor aiexplanationadministration.Actor) error {
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 {
		return errors.WithCode(code.ErrPermissionDenied, "AI explanation administrator identity is required")
	}
	snapshot, ok := authzapp.FromContext(ctx)
	if !ok || !authzapp.DecideCapability(snapshot, authzapp.CapabilityAuditInterpretation).Allowed {
		return errors.WithCode(code.ErrPermissionDenied, "AI explanation evaluation audit permission is required")
	}
	return nil
}

func (aiAdministrationAccessAdapter) AuthorizeReview(ctx context.Context, actor aiexplanationadministration.Actor, role domainevaluation.ReviewRole) error {
	if role != domainevaluation.ReviewRoleAssessmentSemantics && role != domainevaluation.ReviewRoleSafetyProduct {
		return errors.WithCode(code.ErrPermissionDenied, "AI explanation review role is invalid")
	}
	return aiAdministrationAccessAdapter{}.AuthorizeGovernance(ctx, actor)
}

func (aiAdministrationAccessAdapter) AuthorizeGovernance(ctx context.Context, actor aiexplanationadministration.Actor) error {
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 {
		return errors.WithCode(code.ErrPermissionDenied, "AI explanation administrator identity is required")
	}
	snapshot, ok := authzapp.FromContext(ctx)
	if !ok || !authzapp.DecideCapability(snapshot, authzapp.CapabilityOrgAdmin).Allowed {
		return errors.WithCode(code.ErrPermissionDenied, "AI explanation release governance permission is required")
	}
	return nil
}

func (m *Module) AutomationService() interpretationautomation.Service {
	if m == nil {
		return nil
	}
	return m.automationService
}

// BindParticipantAccess installs the participant-owned read use cases after
// Evaluation has exposed its ownership-checking query service.
func (m *Module) BindReportProjection(projection reportprojection.Mapper) {
	if m == nil {
		return
	}
	m.projectionMapper = projection
}

func (m *Module) BindParticipantAccess(access interpretationparticipant.Access) error {
	if m == nil || access == nil || m.reader == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "interpretation participant service dependencies are not configured")
	}
	m.participantService = interpretationparticipant.NewService(m.reader, access, m.projectionMapper)
	m.participantAccess = access
	return m.tryBindAIExplanationParticipant()
}

func (m *Module) tryBindAIExplanationParticipant() error {
	if m == nil || !m.aiExplanationEnabled || m.aiExplanationService != nil || m.aiOutcomeRepo == nil || m.participantAccess == nil {
		return nil
	}
	sourceResolver, err := aiexplanationsource.NewResolver(m.reportCatalog, m.reportRepo, m.aiOutcomeRepo)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize AI explanation source resolver: %v", err)
	}
	service, err := aiexplanationparticipant.NewService(
		m.participantAccess, sourceResolver, m.aiProfileRepo, m.aiPromptCatalog, m.aiRouteCatalog, m.aiCommitter,
		m.aiGenerationRepo, m.aiRunRepo, m.aiArtifactRepo, m.reportCatalog,
	)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize participant AI explanation service: %v", err)
	}
	m.aiExplanationService = service
	return nil
}

func (m *Module) BindAdministrationAccess(access interpretationadmin.Access) error {
	if m == nil || access == nil || m.reader == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "interpretation administration service dependencies are not configured")
	}
	m.administrationService = interpretationadmin.NewService(m.reader, access, m.projectionMapper)
	return nil
}

func (m *Module) AdministrationService() interpretationadmin.Service {
	if m == nil {
		return nil
	}
	return m.administrationService
}

func (m *Module) BindClinicianAccess(access interpretationclinician.Access) error {
	if m == nil || access == nil || m.reader == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "interpretation clinician service dependencies are not configured")
	}
	m.clinicianService = interpretationclinician.NewService(m.reader, access, m.projectionMapper)
	return nil
}
func (m *Module) ClinicianService() interpretationclinician.Service {
	if m == nil {
		return nil
	}
	return m.clinicianService
}

func (m *Module) ParticipantService() interpretationparticipant.Service {
	if m == nil {
		return nil
	}
	return m.participantService
}

func (m *Module) ReportTemplateService() appreporttemplate.Service {
	if m == nil {
		return nil
	}
	return m.reportTemplateService
}

func (m *Module) ReportTemplateCatalog() domainreporttemplate.Catalog {
	if m == nil {
		return nil
	}
	return m.reportTemplateRepo
}

func buildReportBuilderRegistry() (rendering.Registry, error) {
	registry, err := rendering.NewDefaultRegistry(interpretationbuilder.NewDefaultReportBuilder())
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report builder registry: %v", err)
	}
	return registry, nil
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
		Description: "解读报告模块",
	}
}
