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

	// 注册医学量表服务
	if err := r.registerMedicalScaleService(); err != nil {
		return err
	}

	// 注册解读报告服务
	if err := r.registerInterpretReportService(); err != nil {
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
	if r.container.AnswersheetModule == nil {
		log.Warn("AnswersheetModule is not initialized, skipping answersheet service registration")
		return nil
	}

	answerSheetService := service.NewAnswerSheetService(
		r.container.AnswersheetModule.AnswersheetSaver,
		r.container.AnswersheetModule.AnswersheetQueryer,
	)

	r.server.RegisterService(answerSheetService)
	log.Info("   📋 AnswerSheet service registered")
	return nil
}

// registerQuestionnaireService 注册问卷服务
func (r *GRPCRegistry) registerQuestionnaireService() error {
	if r.container.QuestionnaireModule == nil {
		log.Warn("QuestionnaireModule is not initialized, skipping questionnaire service registration")
		return nil
	}

	// 只需要查询服务
	questionnaireService := service.NewQuestionnaireService(
		r.container.QuestionnaireModule.QuesQueryer,
	)

	r.server.RegisterService(questionnaireService)
	log.Info("   📝 Questionnaire service registered (read-only)")
	return nil
}

// registerMedicalScaleService 注册医学量表服务
func (r *GRPCRegistry) registerMedicalScaleService() error {
	if r.container.MedicalScaleModule == nil {
		log.Warn("MedicalScaleModule is not initialized, skipping medical scale service registration")
		return nil
	}

	// 创建并注册医学量表服务
	medicalScaleService := service.NewMedicalScaleService(r.container.MedicalScaleModule.MSQueryer)
	r.server.RegisterService(medicalScaleService)
	log.Info("   🏥 MedicalScale service registered (read-only)")
	return nil
}

// registerInterpretReportService 注册解读报告服务
func (r *GRPCRegistry) registerInterpretReportService() error {
	if r.container.InterpretReportModule == nil {
		log.Warn("InterpretReportModule is not initialized, skipping interpret report service registration")
		return nil
	}

	// 创建并注册解读报告服务
	interpretReportService := service.NewInterpretReportService(
		r.container.InterpretReportModule.IRCreator,
		r.container.InterpretReportModule.IRQueryer,
	)
	r.server.RegisterService(interpretReportService)
	log.Info("   📊 InterpretReport service registered")
	return nil
}

// registerActorService 注册 Actor 服务
func (r *GRPCRegistry) registerActorService() error {
	if r.container.ActorModule == nil {
		log.Warn("ActorModule is not initialized, skipping actor service registration")
		return nil
	}

	// 创建并注册 Actor 服务
	actorService := service.NewActorService(r.container.ActorModule.TesteeService)
	r.server.RegisterService(actorService)
	log.Info("   👤 Actor service registered")
	return nil
}

// GetRegisteredServices 获取已注册的服务列表
func (r *GRPCRegistry) GetRegisteredServices() []string {
	services := make([]string, 0)

	if r.container.AnswersheetModule != nil {
		services = append(services, "AnswerSheetService")
	}

	if r.container.QuestionnaireModule != nil {
		services = append(services, "QuestionnaireService")
	}

	if r.container.MedicalScaleModule != nil {
		services = append(services, "MedicalScaleService")
	}

	if r.container.InterpretReportModule != nil {
		services = append(services, "InterpretReportService")
	}

	if r.container.ActorModule != nil {
		services = append(services, "ActorService")
	}

	return services
}
