package container

import (
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	questionnaireApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"
	resttransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest"
	grpcpkg "github.com/FangcunMount/qs-server/internal/pkg/grpc"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

func (c *Container) BuildRESTDeps(rateCfg *options.RateLimitOptions) resttransport.Deps {
	deps := resttransport.Deps{RateLimit: rateCfg}
	if c == nil {
		return deps
	}
	deps.CodesService = c.CodesService
	deps.QRCodeObjectStore = c.QRCodeObjectStore
	deps.QRCodeObjectKeyPrefix = c.QRCodeObjectKeyPrefix
	deps.GovernanceStatusService = c.CacheGovernanceStatusService()
	deps.EventStatusService = c.buildEventStatusService()
	deps.Backpressure = c.buildBackpressureSnapshots()

	if c.SurveyModule != nil {
		if c.SurveyModule.Questionnaire != nil {
			deps.Survey.QuestionnaireLifecycleService = c.SurveyModule.Questionnaire.LifecycleService
			deps.Survey.QuestionnaireContentService = c.SurveyModule.Questionnaire.ContentService
			deps.Survey.QuestionnaireQueryService = c.SurveyModule.Questionnaire.QueryService
			deps.Survey.QuestionnaireQRCodeService = questionnaireApp.NewQRCodeQueryService(c.SurveyModule.Questionnaire.QueryService, c.QRCodeService)
		}
		if c.SurveyModule.AnswerSheet != nil {
			deps.Survey.AnswerSheetManagementService = c.SurveyModule.AnswerSheet.ManagementService
			deps.Survey.AnswerSheetSubmissionService = c.SurveyModule.AnswerSheet.SubmissionService
		}
	}
	if c.ScaleModule != nil {
		deps.Scale.LifecycleService = c.ScaleModule.LifecycleService
		deps.Scale.FactorService = c.ScaleModule.FactorService
		deps.Scale.QueryService = c.ScaleModule.QueryService
		deps.Scale.CategoryService = c.ScaleModule.CategoryService
		deps.Scale.QRCodeService = scaleApp.NewQRCodeQueryService(c.QRCodeService)
	}
	if c.ActorModule != nil {
		deps.Actor.TesteeManagementService = c.ActorModule.TesteeManagementService
		deps.Actor.TesteeQueryService = c.ActorModule.TesteeQueryService
		deps.Actor.TesteeBackendQueryService = c.ActorModule.TesteeBackendQueryService
		deps.Actor.TesteeAccessService = c.ActorModule.TesteeAccessService
		deps.Actor.OperatorLifecycleService = c.ActorModule.OperatorLifecycleService
		deps.Actor.OperatorAuthorizationService = c.ActorModule.OperatorAuthorizationService
		deps.Actor.OperatorQueryService = c.ActorModule.OperatorQueryService
		deps.Actor.ClinicianLifecycleService = c.ActorModule.ClinicianLifecycleService
		deps.Actor.ClinicianQueryService = c.ActorModule.ClinicianQueryService
		deps.Actor.ClinicianRelationshipService = c.ActorModule.ClinicianRelationshipService
		deps.Actor.AssessmentEntryService = c.ActorModule.AssessmentEntryService
		deps.Actor.QRCodeService = c.QRCodeService
		deps.Actor.ActiveOperatorChecker = c.ActorModule.ActiveOperatorChecker
		deps.Actor.OperatorRoleProjectionUpdater = c.ActorModule.OperatorRoleProjectionUpdater
	}
	if c.EvaluationModule != nil {
		deps.Evaluation.ManagementService = c.EvaluationModule.ManagementService
		deps.Evaluation.ReportQueryService = c.EvaluationModule.ReportQueryService
		deps.Evaluation.ScoreQueryService = c.EvaluationModule.ScoreQueryService
		deps.Evaluation.EvaluationService = c.EvaluationModule.EvaluationService
		deps.Evaluation.WaitService = c.EvaluationModule.WaitService
		deps.Evaluation.AccessQueryService = c.EvaluationModule.AccessQueryService
		if c.ActorModule != nil {
			deps.Actor.TesteeScaleAnalysisService = testeeApp.NewScaleAnalysisQueryService(
				c.EvaluationModule.ManagementService,
				c.EvaluationModule.ScoreQueryService,
			)
		}
	}
	if c.PlanModule != nil {
		deps.Plan.Handler = c.PlanModule.Handler
	}
	if c.StatisticsModule != nil {
		deps.Statistics.Handler = c.StatisticsModule.Handler
	}
	if c.IAMModule != nil {
		deps.IAM.Enabled = c.IAMModule.IsEnabled()
		deps.IAM.TokenVerifier = c.IAMModule.SDKTokenVerifier()
		deps.IAM.SnapshotLoader = c.IAMModule.AuthzSnapshotLoader()
		if client := c.IAMModule.Client(); client != nil && client.Config() != nil && client.Config().JWT != nil {
			deps.IAM.ForceRemoteVerification = client.Config().JWT.ForceRemoteVerification
		}
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

func (c *Container) buildEventStatusService() appEventing.StatusService {
	if c == nil {
		return appEventing.NewStatusService(appEventing.StatusServiceOptions{})
	}
	outboxes := make([]appEventing.NamedOutboxStatusReader, 0, 2)
	if c.SurveyModule != nil && c.SurveyModule.AnswerSheet != nil && c.SurveyModule.AnswerSheet.SubmittedEventStatusReader.Reader != nil {
		outboxes = append(outboxes, c.SurveyModule.AnswerSheet.SubmittedEventStatusReader)
	}
	if c.EvaluationModule != nil && c.EvaluationModule.AssessmentOutboxStatusReader.Reader != nil {
		outboxes = append(outboxes, c.EvaluationModule.AssessmentOutboxStatusReader)
	}
	return appEventing.NewStatusService(appEventing.StatusServiceOptions{
		Catalog:  c.eventCatalog,
		Outboxes: outboxes,
	})
}

func (c *Container) BuildGRPCDeps(server *grpcpkg.Server) grpctransport.Deps {
	deps := grpctransport.Deps{Server: server}
	if c == nil {
		return deps
	}
	deps.WarmupCoordinator = c.WarmupCoordinator()
	deps.QRCodeService = c.QRCodeService
	deps.MiniProgramTaskNotificationService = c.MiniProgramTaskNotificationService

	if c.SurveyModule != nil {
		if c.SurveyModule.AnswerSheet != nil {
			deps.Survey.AnswerSheetSubmissionService = c.SurveyModule.AnswerSheet.SubmissionService
			deps.Survey.AnswerSheetManagementService = c.SurveyModule.AnswerSheet.ManagementService
			deps.Survey.AnswerSheetScoringService = c.SurveyModule.AnswerSheet.ScoringService
		}
		if c.SurveyModule.Questionnaire != nil {
			deps.Survey.QuestionnaireQueryService = c.SurveyModule.Questionnaire.QueryService
		}
	}
	if c.ActorModule != nil {
		deps.Actor.TesteeRegistrationService = c.ActorModule.TesteeRegistrationService
		deps.Actor.TesteeManagementService = c.ActorModule.TesteeManagementService
		deps.Actor.TesteeQueryService = c.ActorModule.TesteeQueryService
		deps.Actor.ClinicianRelationshipService = c.ActorModule.ClinicianRelationshipService
		deps.Actor.TesteeTaggingService = c.ActorModule.TesteeTaggingService
		deps.Actor.OperatorLifecycleService = c.ActorModule.OperatorLifecycleService
		deps.Actor.OperatorAuthorizationService = c.ActorModule.OperatorAuthorizationService
		deps.Actor.OperatorQueryService = c.ActorModule.OperatorQueryService
		deps.Actor.OperatorRoleProjectionUpdater = c.ActorModule.OperatorRoleProjectionUpdater
	}
	if c.EvaluationModule != nil {
		deps.Evaluation.SubmissionService = c.EvaluationModule.SubmissionService
		deps.Evaluation.ManagementService = c.EvaluationModule.ManagementService
		deps.Evaluation.ReportQueryService = c.EvaluationModule.ReportQueryService
		deps.Evaluation.ScoreQueryService = c.EvaluationModule.ScoreQueryService
		deps.Evaluation.EvaluationService = c.EvaluationModule.EvaluationService
	}
	if c.ScaleModule != nil {
		deps.Scale.QueryService = c.ScaleModule.QueryService
		deps.Scale.CategoryService = c.ScaleModule.CategoryService
	}
	if c.PlanModule != nil {
		deps.Plan.CommandService = c.PlanModule.CommandService
		deps.Plan.TaskRepo = c.PlanModule.TaskRepo
	}
	if c.StatisticsModule != nil {
		deps.Statistics.BehaviorProjectorService = c.StatisticsModule.BehaviorProjectorService
	}
	if c.IAMModule != nil {
		deps.IAM.AuthzSnapshotLoader = c.IAMModule.AuthzSnapshotLoader()
	}

	return deps
}
