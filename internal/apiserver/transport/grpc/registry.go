package grpc

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	clinicianApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	operatorApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	runqueryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runquery"
	interpretationApp "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation"
	modelcatalogApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	notificationApp "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	answerSheetApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	appQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	rulesetInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	grpcpkg "github.com/FangcunMount/qs-server/internal/pkg/grpc"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
)

type Registry struct {
	server *grpcpkg.Server
	deps   Deps
}

type Deps struct {
	Server *grpcpkg.Server

	Survey                 SurveyDeps
	Actor                  ActorDeps
	Evaluation             EvaluationDeps
	Interpretation         InterpretationDeps
	AssessmentModelCatalog AssessmentModelCatalogDeps
	Plan                   PlanDeps
	Statistics             StatisticsDeps
	IAM                    IAMDeps
	PublishedModelCatalog  rulesetport.Catalog

	WarmupCoordinator                  cachegov.Coordinator
	QRCodeService                      SurveyScaleQRCodeGenerator
	MiniProgramTaskNotificationService notificationApp.MiniProgramTaskNotificationService
}

type SurveyScaleQRCodeGenerator interface {
	GenerateQuestionnaireQRCode(ctx context.Context, code, version string) (string, error)
	GenerateScaleQRCode(ctx context.Context, code string) (string, error)
}

type SurveyDeps struct {
	AnswerSheetSubmissionService answerSheetApp.AnswerSheetSubmissionService
	AnswerSheetManagementService answerSheetApp.AnswerSheetManagementService
	AnswerSheetScoringService    answerSheetApp.AnswerSheetScoringService
	QuestionnaireQueryService    appQuestionnaire.QuestionnaireQueryService
}

type ActorDeps struct {
	TesteeRegistrationService        testeeApp.TesteeRegistrationService
	TesteeManagementService          testeeApp.TesteeManagementService
	TesteeQueryService               testeeApp.TesteeQueryService
	ClinicianRelationshipService     clinicianApp.ClinicianRelationshipService
	TesteeAssessmentAttentionService testeeApp.TesteeAssessmentAttentionService
	OperatorLifecycleService         operatorApp.OperatorLifecycleService
	OperatorAuthorizationService     operatorApp.OperatorAuthorizationService
	OperatorQueryService             operatorApp.OperatorQueryService
	OperatorRoleProjectionUpdater    operatorApp.OperatorRoleProjectionUpdater
}

type EvaluationDeps struct {
	IntakeService          assessmentApp.AnswerSheetAssessmentIntakeService
	TesteeQueryService     assessmentApp.TesteeAssessmentQueryService
	WorkerResultReader     assessmentApp.AssessmentResultReader
	ScoreQueryService      assessmentApp.ScoreQueryService
	AssessmentReader       evaluationreadmodel.AssessmentReader
	WorkerExecutionService execute.WorkerExecutionService
	RunQueryService        runqueryApp.Service
	ReportStatusReporter   *reportstatus.Reporter
}

type InterpretationDeps struct {
	OutcomeReportService interpretationApp.OutcomeReportService
	ReportQueryService   interpretationApp.ReportQueryService
}

type AssessmentModelCatalogDeps struct {
	QueryService modelcatalogApp.CatalogQueryService
}

type PlanDeps struct {
	CommandService         planApp.PlanCommandService
	TaskAssessmentResolver planApp.TaskAssessmentResolver
}

type StatisticsDeps struct {
	BehaviorProjectorService statisticsApp.BehaviorProjectorService
}

type IAMDeps struct {
	AuthzSnapshotLoader *iaminfra.AuthzSnapshotLoader
}

func NewRegistry(deps Deps) *Registry {
	return &Registry{
		server: deps.Server,
		deps:   deps,
	}
}

// RegisterServices 注册所有 GRPC 服务。
func (r *Registry) RegisterServices() error {
	logger.L(context.Background()).Infow("Registering GRPC services",
		"component", "grpc",
		"action", "register_services",
	)

	if err := r.registerAnswerSheetService(); err != nil {
		return err
	}
	if err := r.registerQuestionnaireService(); err != nil {
		return err
	}
	if err := r.registerActorService(); err != nil {
		return err
	}
	if err := r.registerEvaluationService(); err != nil {
		return err
	}
	if err := r.registerAssessmentModelCatalogService(); err != nil {
		return err
	}
	if err := r.registerInternalService(); err != nil {
		return err
	}
	if err := r.registerPlanCommandService(); err != nil {
		return err
	}

	logger.L(context.Background()).Infow("All GRPC services registered successfully",
		"component", "grpc",
		"result", "success",
	)
	return nil
}

func (r *Registry) registerAnswerSheetService() error {
	if r.deps.Survey.AnswerSheetSubmissionService == nil || r.deps.Survey.AnswerSheetManagementService == nil {
		log.Warn("SurveyModule is not initialized, skipping answersheet service registration")
		return nil
	}

	answerSheetService := service.NewAnswerSheetService(
		r.deps.Survey.AnswerSheetSubmissionService,
		r.deps.Survey.AnswerSheetManagementService,
	)
	r.server.RegisterService(answerSheetService)
	log.Info("   📋 AnswerSheet service registered")
	return nil
}

func (r *Registry) registerQuestionnaireService() error {
	if r.deps.Survey.QuestionnaireQueryService == nil {
		log.Warn("SurveyModule is not initialized, skipping questionnaire service registration")
		return nil
	}

	questionnaireService := service.NewQuestionnaireService(
		r.deps.Survey.QuestionnaireQueryService,
		r.deps.PublishedModelCatalog,
	)
	r.server.RegisterService(questionnaireService)
	log.Info("   📝 Questionnaire service registered (read-only)")
	return nil
}

func (r *Registry) registerActorService() error {
	if r.deps.Actor.TesteeRegistrationService == nil ||
		r.deps.Actor.TesteeManagementService == nil ||
		r.deps.Actor.TesteeQueryService == nil ||
		r.deps.Actor.ClinicianRelationshipService == nil {
		log.Warn("ActorModule is not initialized, skipping actor service registration")
		return nil
	}

	actorService := service.NewActorService(
		r.deps.Actor.TesteeRegistrationService,
		r.deps.Actor.TesteeManagementService,
		r.deps.Actor.TesteeQueryService,
		r.deps.Actor.ClinicianRelationshipService,
	)
	r.server.RegisterService(actorService)
	log.Info("   👥 Actor service registered")
	return nil
}

func (r *Registry) registerEvaluationService() error {
	if r.deps.Evaluation.IntakeService == nil ||
		r.deps.Evaluation.TesteeQueryService == nil ||
		r.deps.Interpretation.ReportQueryService == nil ||
		r.deps.Evaluation.ScoreQueryService == nil {
		log.Warn("EvaluationModule is not initialized, skipping evaluation service registration")
		return nil
	}

	evaluationService := service.NewEvaluationService(
		r.deps.Evaluation.IntakeService,
		r.deps.Evaluation.TesteeQueryService,
		r.deps.Interpretation.ReportQueryService,
		r.deps.Evaluation.ScoreQueryService,
		r.deps.Evaluation.AssessmentReader,
	)
	r.server.RegisterService(evaluationService)
	log.Info("   📊 Evaluation service registered")
	return nil
}

func (r *Registry) registerAssessmentModelCatalogService() error {
	if r.deps.AssessmentModelCatalog.QueryService == nil {
		log.Warn("AssessmentModelCatalog is not initialized, skipping published catalogue service registration")
		return nil
	}
	r.server.RegisterService(service.NewAssessmentModelCatalogService(r.deps.AssessmentModelCatalog.QueryService))
	log.Info("   📚 AssessmentModel catalog service registered (published-only)")
	return nil
}

func (r *Registry) registerInternalService() error {
	if r.deps.Evaluation.IntakeService == nil || r.deps.Evaluation.WorkerResultReader == nil || r.deps.Evaluation.WorkerExecutionService == nil {
		log.Warn("EvaluationModule is not initialized, skipping internal service registration")
		return nil
	}
	if r.deps.Survey.AnswerSheetScoringService == nil {
		log.Warn("SurveyModule is not initialized, skipping internal service registration")
		return nil
	}
	if r.deps.Actor.TesteeAssessmentAttentionService == nil ||
		r.deps.Actor.OperatorLifecycleService == nil ||
		r.deps.Actor.OperatorAuthorizationService == nil ||
		r.deps.Actor.OperatorQueryService == nil {
		log.Warn("ActorModule is not initialized, skipping internal service registration")
		return nil
	}
	if r.deps.Plan.TaskAssessmentResolver == nil || r.deps.Plan.CommandService == nil {
		log.Warn("PlanModule is not initialized, skipping internal service registration")
		return nil
	}
	if r.deps.Statistics.BehaviorProjectorService == nil {
		log.Warn("StatisticsModule is not initialized, skipping internal service registration")
		return nil
	}

	internalService := service.NewInternalService(
		r.deps.Survey.AnswerSheetScoringService,
		r.deps.Evaluation.IntakeService,
		r.deps.Evaluation.WorkerResultReader,
		r.deps.Evaluation.WorkerExecutionService,
		r.deps.Interpretation.OutcomeReportService,
		r.deps.Evaluation.RunQueryService,
		rulesetInfra.NewAssessmentBindingResolver(r.deps.PublishedModelCatalog),
		r.deps.Actor.TesteeAssessmentAttentionService,
		r.deps.Plan.TaskAssessmentResolver,
		r.deps.Plan.CommandService,
		r.deps.Actor.OperatorLifecycleService,
		r.deps.Actor.OperatorAuthorizationService,
		r.deps.Actor.OperatorQueryService,
		r.deps.Actor.OperatorRoleProjectionUpdater,
		r.deps.Statistics.BehaviorProjectorService,
		r.deps.WarmupCoordinator,
		r.deps.QRCodeService,
		r.deps.MiniProgramTaskNotificationService,
		r.deps.Evaluation.ReportStatusReporter,
	)
	r.server.RegisterService(internalService)
	log.Info("   🔧 Internal service registered (for Worker)")
	return nil
}

func (r *Registry) registerPlanCommandService() error {
	if r.deps.Plan.CommandService == nil {
		log.Warn("PlanModule command service is not initialized, skipping plan command service registration")
		return nil
	}

	planCommandService := service.NewPlanCommandService(r.deps.Plan.CommandService)
	r.server.RegisterService(planCommandService)
	log.Info("   🗂️  PlanCommand service registered (write-side)")
	return nil
}

// GetRegisteredServices 获取已注册的服务列表。
func (r *Registry) GetRegisteredServices() []string {
	services := make([]string, 0)

	if r.deps.Survey.AnswerSheetSubmissionService != nil && r.deps.Survey.AnswerSheetManagementService != nil {
		services = append(services, "AnswerSheetService", "QuestionnaireService")
	}
	if r.deps.AssessmentModelCatalog.QueryService != nil {
		services = append(services, "AssessmentModelCatalogService")
	}
	if r.deps.Actor.TesteeRegistrationService != nil &&
		r.deps.Actor.TesteeManagementService != nil &&
		r.deps.Actor.TesteeQueryService != nil &&
		r.deps.Actor.ClinicianRelationshipService != nil {
		services = append(services, "ActorService")
	}
	if r.deps.Evaluation.IntakeService != nil &&
		r.deps.Evaluation.TesteeQueryService != nil &&
		r.deps.Interpretation.ReportQueryService != nil &&
		r.deps.Evaluation.ScoreQueryService != nil {
		services = append(services, "EvaluationService")
	}
	if r.deps.Plan.CommandService != nil {
		services = append(services, "PlanCommandService")
	}

	return services
}
