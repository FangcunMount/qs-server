package collection

import (
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/container"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
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

// CreateGRPCClientManager 创建 gRPC 客户端管理器
func CreateGRPCClientManager(endpoint string, timeout int, insecure bool, tlsCertFile, tlsKeyFile, tlsCAFile, tlsServerName string) (*grpcclient.Manager, error) {
	manager, err := grpcclient.NewManager(&grpcclient.ManagerConfig{
		Endpoint:      endpoint,
		Timeout:       time.Duration(timeout) * time.Second,
		Insecure:      insecure,
		PoolSize:      1,
		TLSCertFile:   tlsCertFile,
		TLSKeyFile:    tlsKeyFile,
		TLSCAFile:     tlsCAFile,
		TLSServerName: tlsServerName,
	})
	if err != nil {
		return nil, err
	}

	// 注册所有客户端
	if err := manager.RegisterClients(); err != nil {
		manager.Close()
		return nil, err
	}

	return manager, nil
}
