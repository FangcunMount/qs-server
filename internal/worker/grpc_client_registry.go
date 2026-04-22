package worker

import (
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/worker/config"
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

	// 注册内部服务客户端
	if err := r.registerInternalClient(); err != nil {
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

// registerInternalClient 注册内部服务客户端
func (r *GRPCClientRegistry) registerInternalClient() error {
	client := r.manager.InternalClient()
	if client == nil {
		log.Warn("Internal client is not initialized, skipping registration")
		return nil
	}

	r.container.SetInternalClient(client)
	log.Info("   🔧 Internal client injected to container")
	return nil
}

// CreateGRPCClientManager 创建 gRPC 客户端管理器
func CreateGRPCClientManager(cfg *config.GRPCConfig, timeout int) (*grpcclient.Manager, error) {
	manager, err := grpcclient.NewManager(&grpcclient.ManagerConfig{
		Endpoint: cfg.ApiserverAddr,
		Timeout:  time.Duration(timeout) * time.Second,
		PoolSize: 1,
		Insecure: cfg.Insecure,
		TLS: grpcclient.TLSConfig{
			CAFile:     cfg.TLSCAFile,
			CertFile:   cfg.TLSCertFile,
			KeyFile:    cfg.TLSKeyFile,
			ServerName: cfg.TLSServerName,
		},
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
