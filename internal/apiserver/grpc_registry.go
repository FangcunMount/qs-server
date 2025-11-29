package apiserver

import (
	"github.com/FangcunMount/iam-contracts/pkg/log"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/service"
	"github.com/FangcunMount/qs-server/internal/pkg/grpcserver"
)

// GRPCRegistry GRPC 服务注册器
type GRPCRegistry struct {
	server    *grpcserver.Server
	container *container.Container
}

// NewGRPCRegistry 创建 GRPC 服务注册器
func NewGRPCRegistry(server *grpcserver.Server, container *container.Container) *GRPCRegistry {
	return &GRPCRegistry{
		server:    server,
		container: container,
	}
}

// RegisterServices 注册所有 GRPC 服务
func (r *GRPCRegistry) RegisterServices() error {
	log.Info("🔧 Registering GRPC services...")

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

	log.Info("✅ All GRPC services registered successfully")
	return nil
}

// registerAnswerSheetService 注册答卷服务
func (r *GRPCRegistry) registerAnswerSheetService() error {
	if r.container.SurveyModule == nil {
		log.Warn("SurveyModule is not initialized, skipping answersheet service registration")
		return nil
	}

	// 使用 SurveyModule 中的 SubmissionService
	answerSheetService := service.NewAnswerSheetService(
		r.container.SurveyModule.AnswerSheet.SubmissionService,
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
	)
	r.server.RegisterService(actorService)
	log.Info("   👥 Actor service registered")
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

	return services
}
