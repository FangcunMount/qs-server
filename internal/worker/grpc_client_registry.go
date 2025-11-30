package worker

import (
	"time"

	"github.com/FangcunMount/iam-contracts/pkg/log"
	"github.com/FangcunMount/qs-server/internal/worker/container"
	"github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
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
func CreateGRPCClientManager(endpoint string, timeout int) (*grpcclient.Manager, error) {
	manager, err := grpcclient.NewManager(&grpcclient.ManagerConfig{
		Endpoint: endpoint,
		Timeout:  time.Duration(timeout) * time.Second,
		PoolSize: 1,
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
