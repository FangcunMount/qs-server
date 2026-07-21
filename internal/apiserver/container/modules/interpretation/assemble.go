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
	interpretationautomation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation"
	interpretationexecution "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	interpretationclinician "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/clinician"
	interpretationoperations "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/operations"
	interpretationparticipant "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/participant"
	appreporttemplate "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporttemplate"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reportprojection"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	interpretationbuilder "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/builder"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rendering"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongoEval "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	evaluationreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
)

// Module assembles report read/query, builder-registry, and durable write capabilities.
type Module struct {
	reader                evaluationreadmodel.ReportReader
	executionExecutor     interpretationexecution.Executor
	generationRepo        *mongoEval.GenerationRepository
	runRepo               *mongoEval.RunRepository
	reportRepo            *mongoEval.ReportRepository
	admissionRepo         *mongoEval.AdmissionFailureRepository
	reportTemplateRepo    *mongoEval.ReportTemplateRepository
	reportTemplateService appreporttemplate.Service
	automationService     interpretationautomation.Service
	projectionMapper      reportprojection.Mapper
	participantService    interpretationparticipant.Service
	administrationService interpretationadmin.Service
	clinicianService      interpretationclinician.Service
	operationsService     interpretationoperations.Service
	governedRetryService  interpretationautomation.GovernedRetryService
	leaseRecoverer        interpretationautomation.LeaseRecoverer
	txRunner              apptransaction.Runner
	eventStager           appEventing.EventStager
	ReportStatusReporter  *reportstatus.Reporter
}

// Deps defines explicit constructor dependencies for the report module.
type Deps struct {
	MongoDB            *mongo.Database
	MongoLimiter       backpressure.Acquirer
	OpsHandle          *redisruntime.Handle
	ReportStatusConfig reportstatus.Config
	OutboxProfile      appEventing.ProfileBinding
	RunLeaseDuration   time.Duration
}

// New assembles the report module.
func New(deps Deps) (*Module, error) {
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
	module.reader = mongoEval.NewReportReadModel(deps.MongoDB, mongoOptions)
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
	admissionRepo, err := mongoEval.NewAdmissionFailureRepository(deps.MongoDB, mongoOptions)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize interpretation admission failure repository: %v", err)
	}
	module.admissionRepo = admissionRepo
	reportTemplateRepo, err := mongoEval.NewReportTemplateRepository(deps.MongoDB, mongoOptions)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report template repository: %v", err)
	}
	module.reportTemplateRepo = reportTemplateRepo
	module.reportTemplateService = appreporttemplate.NewService(reportTemplateRepo)
	catalogProjector, err := mongoEval.NewReportCatalogProjector(deps.MongoDB, mongoOptions)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report catalog projector: %v", err)
	}

	if deps.OutboxProfile.Stager == nil || deps.OutboxProfile.PostCommit == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "mongo domain event profile is required")
	}
	mongoTxRunner := modtx.NewMongoRunner(deps.MongoDB)
	module.txRunner = mongoTxRunner
	module.eventStager = deps.OutboxProfile.Stager
	{
		registry, err := buildReportBuilderRegistry()
		if err != nil {
			return nil, err
		}
		starter, err := interpretationexecution.NewStarter(mongoTxRunner, module.generationRepo, module.runRepo, module.reportRepo, deps.RunLeaseDuration)
		if err != nil {
			return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report generation starter: %v", err)
		}
		committer, err := interpretationexecution.NewInterpretationCommitter(mongoTxRunner, module.generationRepo, module.runRepo, module.reportRepo, deps.OutboxProfile.Stager, deps.OutboxProfile.PostCommit, catalogProjector)
		if err != nil {
			return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize interpretation committer: %v", err)
		}
		executor, err := interpretationexecution.NewExecutor(starter, registry, committer)
		if err != nil {
			return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize interpretation execution: %v", err)
		}
		module.executionExecutor = executor
	}

	return module, nil
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
	return nil
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

func buildReportBuilderRegistry() (rendering.Registry, error) {
	builders := rendering.DefaultBuilders(interpretationbuilder.NewDefaultReportBuilder())
	registry, err := rendering.NewRegistry(builders...)
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
