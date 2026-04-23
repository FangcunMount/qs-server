package grpcclient

import (
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/container"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"google.golang.org/grpc/credentials"
)

// GRPCClientRegistry gRPC 客户端注册器
type GRPCClientRegistry struct {
	manager   *grpcclient.Manager
	container *container.Container
}

// NewGRPCClientRegistry 创建 gRPC 客户端注册器
func NewGRPCClientRegistry(manager *grpcclient.Manager, container *container.Container) *GRPCClientRegistry {
	return &GRPCClientRegistry{
		manager:   manager,
		container: container,
	}
}

func NewRegistry(manager *grpcclient.Manager, container *container.Container) *GRPCClientRegistry {
	return NewGRPCClientRegistry(manager, container)
}

// RegisterClients 注册所有 gRPC 客户端到容器
func (r *GRPCClientRegistry) RegisterClients() error {
	log.Info("🔧 Registering gRPC clients to container...")

	// 注册答卷客户端
	if err := r.registerAnswerSheetClient(); err != nil {
		return err
	}

	// 注册问卷客户端
	if err := r.registerQuestionnaireClient(); err != nil {
		return err
	}

	// 注册测评客户端
	if err := r.registerEvaluationClient(); err != nil {
		return err
	}

	// 注册 Actor 客户端
	if err := r.registerActorClient(); err != nil {
		return err
	}

	// 注册 Scale 客户端
	if err := r.registerScaleClient(); err != nil {
		return err
	}

	log.Info("✅ All gRPC clients registered to container")
	return nil
}

// registerAnswerSheetClient 注册答卷客户端
func (r *GRPCClientRegistry) registerAnswerSheetClient() error {
	client := r.manager.AnswerSheetClient()
	if client == nil {
		log.Warn("AnswerSheet client is not initialized, skipping registration")
		return nil
	}

	r.container.SetAnswerSheetClient(client)
	log.Info("   📋 AnswerSheet client injected to container")
	return nil
}

// registerQuestionnaireClient 注册问卷客户端
func (r *GRPCClientRegistry) registerQuestionnaireClient() error {
	client := r.manager.QuestionnaireClient()
	if client == nil {
		log.Warn("Questionnaire client is not initialized, skipping registration")
		return nil
	}

	r.container.SetQuestionnaireClient(client)
	log.Info("   📝 Questionnaire client injected to container")
	return nil
}

// registerEvaluationClient 注册测评客户端
func (r *GRPCClientRegistry) registerEvaluationClient() error {
	client := r.manager.EvaluationClient()
	if client == nil {
		log.Warn("Evaluation client is not initialized, skipping registration")
		return nil
	}

	r.container.SetEvaluationClient(client)
	log.Info("   📊 Evaluation client injected to container")
	return nil
}

// registerActorClient 注册 Actor 客户端
func (r *GRPCClientRegistry) registerActorClient() error {
	client := r.manager.ActorClient()
	if client == nil {
		log.Warn("Actor client is not initialized, skipping registration")
		return nil
	}

	r.container.SetActorClient(client)
	log.Info("   👤 Actor client injected to container")
	return nil
}

// registerScaleClient 注册量表客户端
func (r *GRPCClientRegistry) registerScaleClient() error {
	client := r.manager.ScaleClient()
	if client == nil {
		log.Warn("Scale client is not initialized, skipping registration")
		return nil
	}

	r.container.SetScaleClient(client)
	log.Info("   📊 Scale client injected to container")
	return nil
}

// CreateGRPCClientManager 创建 gRPC 客户端管理器。
// perRPC 非 nil 时（通常为 IAM ServiceAuthHelper）对 apiserver 的每次 RPC 附加服务 JWT metadata。
func CreateGRPCClientManager(endpoint string, timeout int, insecure bool, tlsCertFile, tlsKeyFile, tlsCAFile, tlsServerName string, maxInflight int, perRPC credentials.PerRPCCredentials) (*grpcclient.Manager, error) {
	manager, err := grpcclient.NewManager(&grpcclient.ManagerConfig{
		Endpoint:          endpoint,
		Timeout:           time.Duration(timeout) * time.Second,
		Insecure:          insecure,
		PoolSize:          1,
		MaxInflight:       maxInflight,
		TLSCertFile:       tlsCertFile,
		TLSKeyFile:        tlsKeyFile,
		TLSCAFile:         tlsCAFile,
		TLSServerName:     tlsServerName,
		PerRPCCredentials: perRPC,
	})
	if err != nil {
		return nil, err
	}

	// 注册所有客户端
	if err := manager.RegisterClients(); err != nil {
		if closeErr := manager.Close(); closeErr != nil {
			log.Warnf("Failed to close gRPC client manager after register error: %v", closeErr)
		}
		return nil, err
	}

	return manager, nil
}
