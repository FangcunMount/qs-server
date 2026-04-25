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
	log.Info("🔧 Building collection gRPC client bundle...")
	bundle := container.ClientBundle{
		AnswerSheet:   r.answerSheetClient(),
		Questionnaire: r.questionnaireClient(),
		Evaluation:    r.evaluationClient(),
		Actor:         r.actorClient(),
		Scale:         r.scaleClient(),
	}
	log.Info("✅ Collection gRPC client bundle built")
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

func (r *GRPCClientRegistry) questionnaireClient() *grpcclient.QuestionnaireClient {
	client := r.manager.QuestionnaireClient()
	if client == nil {
		log.Warn("Questionnaire client is not initialized, skipping registration")
		return nil
	}
	log.Info("   📝 Questionnaire client added to bundle")
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

func (r *GRPCClientRegistry) actorClient() *grpcclient.ActorClient {
	client := r.manager.ActorClient()
	if client == nil {
		log.Warn("Actor client is not initialized, skipping registration")
		return nil
	}
	log.Info("   👤 Actor client added to bundle")
	return client
}

func (r *GRPCClientRegistry) scaleClient() *grpcclient.ScaleClient {
	client := r.manager.ScaleClient()
	if client == nil {
		log.Warn("Scale client is not initialized, skipping registration")
		return nil
	}
	log.Info("   📊 Scale client added to bundle")
	return client
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
