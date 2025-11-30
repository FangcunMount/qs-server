package grpcclient

import (
	"sync"
	"time"

	"github.com/FangcunMount/iam-contracts/pkg/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ManagerConfig gRPC 客户端管理器配置
type ManagerConfig struct {
	Endpoint   string        // apiserver 地址，如 "localhost:9090"
	Timeout    time.Duration // 请求超时时间
	Insecure   bool          // 是否使用不安全连接（开发环境）
	PoolSize   int           // 连接池大小（默认 1）
	MaxRetries int           // 最大重试次数
}

// Manager gRPC 客户端管理器，负责连接池管理和客户端缓存
type Manager struct {
	config *ManagerConfig
	conn   *grpc.ClientConn
	mu     sync.RWMutex

	// 客户端缓存
	clients map[string]interface{}

	// 已注册的客户端
	answerSheetClient   *AnswerSheetClient
	questionnaireClient *QuestionnaireClient
	evaluationClient    *EvaluationClient
}

// NewManager 创建 gRPC 客户端管理器
func NewManager(cfg *ManagerConfig) (*Manager, error) {
	if cfg.PoolSize <= 0 {
		cfg.PoolSize = 1
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}

	m := &Manager{
		config:  cfg,
		clients: make(map[string]interface{}),
	}

	// 初始化连接
	if err := m.connect(); err != nil {
		return nil, err
	}

	return m, nil
}

// connect 建立 gRPC 连接
func (m *Manager) connect() error {
	opts := []grpc.DialOption{
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(10*1024*1024), // 10MB
			grpc.MaxCallSendMsgSize(10*1024*1024), // 10MB
		),
	}

	if m.config.Insecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(m.config.Endpoint, opts...)
	if err != nil {
		return err
	}

	m.conn = conn
	return nil
}

// RegisterClients 注册所有 gRPC 客户端
func (m *Manager) RegisterClients() error {
	log.Info("🔧 Registering gRPC clients...")

	// 创建基础 Client
	baseClient := &Client{
		conn: m.conn,
		config: &ClientConfig{
			Endpoint: m.config.Endpoint,
			Timeout:  m.config.Timeout,
			Insecure: m.config.Insecure,
		},
	}

	// 注册 AnswerSheet 客户端
	m.answerSheetClient = NewAnswerSheetClient(baseClient)
	m.clients["answerSheet"] = m.answerSheetClient
	log.Info("   📋 AnswerSheet client registered")

	// 注册 Questionnaire 客户端
	m.questionnaireClient = NewQuestionnaireClient(baseClient)
	m.clients["questionnaire"] = m.questionnaireClient
	log.Info("   📝 Questionnaire client registered")

	// 注册 Evaluation 客户端
	m.evaluationClient = NewEvaluationClient(baseClient)
	m.clients["evaluation"] = m.evaluationClient
	log.Info("   📊 Evaluation client registered")

	log.Infof("✅ All gRPC clients registered (endpoint: %s)", m.config.Endpoint)
	return nil
}

// AnswerSheetClient 获取答卷客户端
func (m *Manager) AnswerSheetClient() *AnswerSheetClient {
	return m.answerSheetClient
}

// QuestionnaireClient 获取问卷客户端
func (m *Manager) QuestionnaireClient() *QuestionnaireClient {
	return m.questionnaireClient
}

// EvaluationClient 获取测评客户端
func (m *Manager) EvaluationClient() *EvaluationClient {
	return m.evaluationClient
}

// GetClient 根据名称获取客户端
func (m *Manager) GetClient(name string) interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clients[name]
}

// Conn 获取底层 gRPC 连接
func (m *Manager) Conn() *grpc.ClientConn {
	return m.conn
}

// Close 关闭所有连接
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn != nil {
		if err := m.conn.Close(); err != nil {
			log.Warnf("Failed to close gRPC connection: %v", err)
			return err
		}
	}

	m.clients = make(map[string]interface{})
	log.Info("🔌 gRPC client manager closed")
	return nil
}

// IsConnected 检查连接状态
func (m *Manager) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.conn != nil
}

// Endpoint 返回连接端点
func (m *Manager) Endpoint() string {
	return m.config.Endpoint
}
