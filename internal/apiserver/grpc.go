package apiserver

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/container"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/service"
	"github.com/yshujie/questionnaire-scale/internal/pkg/grpcserver"
	"github.com/yshujie/questionnaire-scale/pkg/log"
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

	// 注册用户服务
	if err := r.registerUserService(); err != nil {
		return err
	}

	// 注册认证服务
	if err := r.registerAuthService(); err != nil {
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

	// TODO: 实现问卷 GRPC 服务
	// questionnaireService := service.NewQuestionnaireService(
	//     r.container.QuestionnaireModule.QuesCreator,
	//     r.container.QuestionnaireModule.QuesQueryer,
	//     r.container.QuestionnaireModule.QuesEditor,
	//     r.container.QuestionnaireModule.QuesPublisher,
	// )
	// r.server.RegisterService(questionnaireService)

	log.Info("   📝 Questionnaire service registration skipped (not implemented)")
	return nil
}

// registerUserService 注册用户服务
func (r *GRPCRegistry) registerUserService() error {
	if r.container.UserModule == nil {
		log.Warn("UserModule is not initialized, skipping user service registration")
		return nil
	}

	// TODO: 实现用户 GRPC 服务
	// userService := service.NewUserService(
	//     r.container.UserModule.UserCreator,
	//     r.container.UserModule.UserQueryer,
	//     r.container.UserModule.UserEditor,
	//     r.container.UserModule.UserActivator,
	//     r.container.UserModule.UserPasswordChanger,
	// )
	// r.server.RegisterService(userService)

	log.Info("   👤 User service registration skipped (not implemented)")
	return nil
}

// registerAuthService 注册认证服务
func (r *GRPCRegistry) registerAuthService() error {
	if r.container.AuthModule == nil {
		log.Warn("AuthModule is not initialized, skipping auth service registration")
		return nil
	}

	// TODO: 实现认证 GRPC 服务
	// authService := service.NewAuthService(
	//     r.container.AuthModule.Authenticator,
	// )
	// r.server.RegisterService(authService)

	log.Info("   🔐 Auth service registration skipped (not implemented)")
	return nil
}

// GetRegisteredServices 获取已注册的服务列表
func (r *GRPCRegistry) GetRegisteredServices() []string {
	services := make([]string, 0)

	if r.container.AnswersheetModule != nil {
		services = append(services, "AnswerSheetService")
	}

	// TODO: 添加其他服务
	// if r.container.QuestionnaireModule != nil {
	//     services = append(services, "QuestionnaireService")
	// }
	// if r.container.UserModule != nil {
	//     services = append(services, "UserService")
	// }
	// if r.container.AuthModule != nil {
	//     services = append(services, "AuthService")
	// }

	return services
}
