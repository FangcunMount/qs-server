package apiserver

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	appQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/service"
	grpcpkg "github.com/FangcunMount/qs-server/internal/pkg/grpc"
)

// GRPCRegistry GRPC 服务注册器
type GRPCRegistry struct {
	server    *grpcpkg.Server
	container *container.Container
}

// NewGRPCRegistry 创建 GRPC 服务注册器
func NewGRPCRegistry(server *grpcpkg.Server, container *container.Container) *GRPCRegistry {
	return &GRPCRegistry{
		server:    server,
		container: container,
	}
}

// RegisterServices 注册所有 GRPC 服务
func (r *GRPCRegistry) RegisterServices() error {
	logger.L(context.Background()).Infow("Registering GRPC services",
		"component", "grpc",
		"action", "register_services",
	)

	// 注册答卷服务
	if err := r.registerAnswerSheetService(); err != nil {
		return err
	}

	// 注册问卷服务
	if err := r.registerQuestionnaireService(); err != nil {
		return err
	}

	// 注册 Actor 服务
	if err := r.registerActorService(); err != nil {
		return err
	}

	// 注册 Evaluation 服务
	if err := r.registerEvaluationService(); err != nil {
		return err
	}

	// 注册 Scale 服务
	if err := r.registerScaleService(); err != nil {
		return err
	}

	// 注册 Internal 服务（供 Worker 调用）
	if err := r.registerInternalService(); err != nil {
		return err
	}

	// 注册 PlanCommand 服务（供 Worker 和内部系统动作调用）
	if err := r.registerPlanCommandService(); err != nil {
		return err
	}

	logger.L(context.Background()).Infow("All GRPC services registered successfully",
		"component", "grpc",
		"result", "success",
	)
	return nil
}

// registerAnswerSheetService 注册答卷服务
func (r *GRPCRegistry) registerAnswerSheetService() error {
	if r.container.SurveyModule == nil {
		log.Warn("SurveyModule is not initialized, skipping answersheet service registration")
		return nil
	}

	// 使用 SurveyModule 中的 SubmissionService 和 ManagementService
	answerSheetService := service.NewAnswerSheetService(
		r.container.SurveyModule.AnswerSheet.SubmissionService,
		r.container.SurveyModule.AnswerSheet.ManagementService,
	)
	r.server.RegisterService(answerSheetService)
	log.Info("   📋 AnswerSheet service registered")
	return nil
}

// registerQuestionnaireService 注册问卷服务
func (r *GRPCRegistry) registerQuestionnaireService() error {
	if r.container.SurveyModule == nil {
		log.Warn("SurveyModule is not initialized, skipping questionnaire service registration")
		return nil
	}

	// 使用 QueryService
	questionnaireService := service.NewQuestionnaireService(
		r.container.SurveyModule.Questionnaire.QueryService,
	)

	r.server.RegisterService(questionnaireService)
	log.Info("   📝 Questionnaire service registered (read-only)")
	return nil
}

// registerActorService 注册 Actor 服务
func (r *GRPCRegistry) registerActorService() error {
	if r.container.ActorModule == nil {
		log.Warn("ActorModule is not initialized, skipping actor service registration")
		return nil
	}

	// 使用按行为者组织的服务
	actorService := service.NewActorService(
		r.container.ActorModule.TesteeRegistrationService,
		r.container.ActorModule.TesteeManagementService,
		r.container.ActorModule.TesteeQueryService,
		r.container.ActorModule.ClinicianRelationshipService,
		r.container.ActorModule.AssessmentEntryRepo,
	)
	r.server.RegisterService(actorService)
	log.Info("   👥 Actor service registered")
	return nil
}

// registerEvaluationService 注册测评服务
func (r *GRPCRegistry) registerEvaluationService() error {
	if r.container.EvaluationModule == nil {
		log.Warn("EvaluationModule is not initialized, skipping evaluation service registration")
		return nil
	}

	// 使用 EvaluationModule 中的服务
	evaluationService := service.NewEvaluationService(
		r.container.EvaluationModule.SubmissionService,
		r.container.EvaluationModule.ReportQueryService,
		r.container.EvaluationModule.ScoreQueryService,
		r.container.ActorModule.TesteeRepo,
		r.container.EvaluationModule.AssessmentRepo,
	)
	r.server.RegisterService(evaluationService)
	log.Info("   📊 Evaluation service registered")
	return nil
}

// registerScaleService 注册量表服务
func (r *GRPCRegistry) registerScaleService() error {
	if r.container.ScaleModule == nil {
		log.Warn("ScaleModule is not initialized, skipping scale service registration")
		return nil
	}

	// 获取问卷查询服务（如果 SurveyModule 已初始化）
	var questionnaireQueryService appQuestionnaire.QuestionnaireQueryService
	if r.container.SurveyModule != nil && r.container.SurveyModule.Questionnaire != nil {
		questionnaireQueryService = r.container.SurveyModule.Questionnaire.QueryService
	}

	// 使用 QueryService、CategoryService 和 QuestionnaireQueryService
	scaleService := service.NewScaleService(
		r.container.ScaleModule.QueryService,
		r.container.ScaleModule.CategoryService,
		questionnaireQueryService,
	)

	r.server.RegisterService(scaleService)
	log.Info("   📊 Scale service registered (read-only)")
	return nil
}

// registerInternalService 注册内部服务（供 Worker 调用）
func (r *GRPCRegistry) registerInternalService() error {
	if r.container.EvaluationModule == nil {
		log.Warn("EvaluationModule is not initialized, skipping internal service registration")
		return nil
	}

	if r.container.ScaleModule == nil {
		log.Warn("ScaleModule is not initialized, skipping internal service registration")
		return nil
	}

	if r.container.SurveyModule == nil {
		log.Warn("SurveyModule is not initialized, skipping internal service registration")
		return nil
	}

	if r.container.ActorModule == nil {
		log.Warn("ActorModule is not initialized, skipping internal service registration")
		return nil
	}
	if r.container.PlanModule == nil {
		log.Warn("PlanModule is not initialized, skipping internal service registration")
		return nil
	}
	if r.container.StatisticsModule == nil {
		log.Warn("StatisticsModule is not initialized, skipping internal service registration")
		return nil
	}

	// 使用 SurveyModule、EvaluationModule、ScaleModule、ActorModule 中的服务
	var authzSnapshot *iaminfra.AuthzSnapshotLoader
	if r.container.IAMModule != nil {
		authzSnapshot = r.container.IAMModule.AuthzSnapshotLoader()
	}

	internalService := service.NewInternalService(
		r.container.SurveyModule.AnswerSheet.ScoringService,
		r.container.EvaluationModule.SubmissionService,
		r.container.EvaluationModule.ManagementService,
		r.container.EvaluationModule.EvaluationService,
		r.container.ScaleModule.Repo,
		r.container.ActorModule.TesteeTaggingService,
		r.container.PlanModule.TaskRepo,
		r.container.PlanModule.CommandService,
		r.container.ActorModule.OperatorLifecycleService,
		r.container.ActorModule.OperatorAuthorizationService,
		r.container.ActorModule.OperatorQueryService,
		r.container.ActorModule.OperatorRepo,
		authzSnapshot,
		r.container.StatisticsModule.BehaviorProjectorService,
		r.container.QRCodeService, // 可能为 nil
		r.container.MiniProgramTaskNotificationService,
	)
	r.server.RegisterService(internalService)
	log.Info("   🔧 Internal service registered (for Worker)")
	return nil
}

func (r *GRPCRegistry) registerPlanCommandService() error {
	if r.container.PlanModule == nil || r.container.PlanModule.CommandService == nil {
		log.Warn("PlanModule command service is not initialized, skipping plan command service registration")
		return nil
	}

	planCommandService := service.NewPlanCommandService(r.container.PlanModule.CommandService)
	r.server.RegisterService(planCommandService)
	log.Info("   🗂️  PlanCommand service registered (write-side)")
	return nil
}

// GetRegisteredServices 获取已注册的服务列表
func (r *GRPCRegistry) GetRegisteredServices() []string {
	services := make([]string, 0)

	if r.container.SurveyModule != nil {
		services = append(services, "AnswerSheetService", "QuestionnaireService")
	}

	if r.container.ScaleModule != nil {
		services = append(services, "ScaleService")
	}

	if r.container.ActorModule != nil {
		services = append(services, "ActorService")
	}

	if r.container.EvaluationModule != nil {
		services = append(services, "EvaluationService")
	}

	if r.container.PlanModule != nil && r.container.PlanModule.CommandService != nil {
		services = append(services, "PlanCommandService")
	}

	return services
}
