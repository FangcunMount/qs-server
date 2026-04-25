package grpcclient

import (
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/worker/config"
	"github.com/FangcunMount/qs-server/internal/worker/container"
	"github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
)

// GRPCClientRegistry gRPC 客户端注册器
type GRPCClientRegistry struct {
	manager *grpcclient.Manager
}

// NewGRPCClientRegistry 创建 gRPC 客户端注册器
func NewGRPCClientRegistry(manager *grpcclient.Manager) *GRPCClientRegistry {
	return &GRPCClientRegistry{
		manager: manager,
	}
}

func NewRegistry(manager *grpcclient.Manager) *GRPCClientRegistry {
	return NewGRPCClientRegistry(manager)
}

// ClientBundle returns all gRPC clients as one explicit runtime dependency graph.
func (r *GRPCClientRegistry) ClientBundle() container.ClientBundle {
	log.Info("🔧 Building worker gRPC client bundle...")
	bundle := container.ClientBundle{
		AnswerSheet: r.answerSheetClient(),
		Evaluation:  r.evaluationClient(),
		Internal:    r.internalClient(),
	}
	log.Info("✅ Worker gRPC client bundle built")
	return bundle
}

func (r *GRPCClientRegistry) answerSheetClient() *grpcclient.AnswerSheetClient {
	client := r.manager.AnswerSheetClient()
	if client == nil {
		log.Warn("AnswerSheet client is not initialized, skipping registration")
		return nil
	}
	log.Info("   📋 AnswerSheet client added to bundle")
	return client
}

func (r *GRPCClientRegistry) evaluationClient() *grpcclient.EvaluationClient {
	client := r.manager.EvaluationClient()
	if client == nil {
		log.Warn("Evaluation client is not initialized, skipping registration")
		return nil
	}
	log.Info("   📊 Evaluation client added to bundle")
	return client
}

func (r *GRPCClientRegistry) internalClient() *grpcclient.InternalClient {
	client := r.manager.InternalClient()
	if client == nil {
		log.Warn("Internal client is not initialized, skipping registration")
		return nil
	}
	log.Info("   🔧 Internal client added to bundle")
	return client
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
