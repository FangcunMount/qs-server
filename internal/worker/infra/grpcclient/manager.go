package grpcclient

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// ManagerConfig gRPC 客户端管理器配置
type ManagerConfig struct {
	Endpoint      string        // apiserver gRPC 地址
	Timeout       time.Duration // 请求超时时间
	DialTimeout   time.Duration // 连接超时时间
	PoolSize      int           // 连接池大小（默认 1）
	MaxRetries    int           // 最大重试次数
	KeepaliveTime time.Duration // Keepalive 时间
	Insecure      bool          // 是否使用明文连接
	TLS           TLSConfig     // TLS 配置
}

// TLSConfig TLS/mTLS 配置
type TLSConfig struct {
	CAFile     string // CA 证书
	CertFile   string // 客户端证书（可选，启用 mTLS）
	KeyFile    string // 客户端私钥（可选，启用 mTLS）
	ServerName string // 服务器名称覆盖（可选）
}

// Manager gRPC 客户端管理器，负责连接池管理和客户端缓存
type Manager struct {
	config *ManagerConfig
	conn   *grpc.ClientConn
	mu     sync.RWMutex

	// 客户端缓存
	clients map[string]interface{}

	// 已注册的客户端
	answerSheetClient *AnswerSheetClient
	evaluationClient  *EvaluationClient
	internalClient    *InternalClient
}

// NewManager 创建 gRPC 客户端管理器
func NewManager(cfg *ManagerConfig) (*Manager, error) {
	if cfg.PoolSize <= 0 {
		cfg.PoolSize = 1
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 5 * time.Second
	}
	if cfg.KeepaliveTime <= 0 {
		cfg.KeepaliveTime = 5 * time.Minute // 匹配服务端默认 MinTime，避免 "too_many_pings"
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
		// Keepalive 参数配置，避免 "too_many_pings" 错误
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                m.config.KeepaliveTime,
			Timeout:             20 * time.Second, // ping 响应超时时间
			PermitWithoutStream: false,            // 无活跃流时不发送 ping
		}),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(10*1024*1024), // 10MB
			grpc.MaxCallSendMsgSize(10*1024*1024), // 10MB
		),
	}

	if m.config.Insecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		log.Warn("gRPC client: using insecure connection (not recommended for production)")
	} else {
		creds, err := m.loadTLSCredentials()
		if err != nil {
			return err
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
		log.Info("gRPC client: using secure connection (TLS/mTLS)")
	}

	conn, err := grpc.NewClient(m.config.Endpoint, opts...)
	if err != nil {
		return err
	}

	m.conn = conn
	return nil
}

// loadTLSCredentials 加载 TLS/mTLS 凭证
func (m *Manager) loadTLSCredentials() (credentials.TransportCredentials, error) {
	if m.config.TLS.CAFile == "" {
		return nil, fmt.Errorf("TLS CA file is required for secure gRPC connection")
	}

	caCert, err := os.ReadFile(m.config.TLS.CAFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to add CA certificate to pool")
	}

	tlsCfg := &tls.Config{
		RootCAs: certPool,
	}
	if m.config.TLS.ServerName != "" {
		tlsCfg.ServerName = m.config.TLS.ServerName
	}

	// mTLS（可选）
	if m.config.TLS.CertFile != "" && m.config.TLS.KeyFile != "" {
		clientCert, err := tls.LoadX509KeyPair(m.config.TLS.CertFile, m.config.TLS.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{clientCert}
		log.Info("gRPC client: mTLS enabled (client certificate loaded)")
	}

	return credentials.NewTLS(tlsCfg), nil
}

// RegisterClients 注册所有 gRPC 客户端
func (m *Manager) RegisterClients() error {
	log.Info("🔧 Registering gRPC clients...")

	// 注册 AnswerSheet 客户端
	m.answerSheetClient = NewAnswerSheetClient(m)
	m.clients["answerSheet"] = m.answerSheetClient
	log.Info("   📋 AnswerSheet client registered")

	// 注册 Evaluation 客户端
	m.evaluationClient = NewEvaluationClient(m)
	m.clients["evaluation"] = m.evaluationClient
	log.Info("   📊 Evaluation client registered")

	// 注册 Internal 客户端（用于事件处理）
	m.internalClient = NewInternalClient(m)
	m.clients["internal"] = m.internalClient
	log.Info("   🔧 Internal client registered")

	log.Infof("✅ All gRPC clients registered (endpoint: %s)", m.config.Endpoint)
	return nil
}

// AnswerSheetClient 获取答卷客户端
func (m *Manager) AnswerSheetClient() *AnswerSheetClient {
	return m.answerSheetClient
}

// EvaluationClient 获取测评客户端
func (m *Manager) EvaluationClient() *EvaluationClient {
	return m.evaluationClient
}

// InternalClient 获取内部服务客户端
func (m *Manager) InternalClient() *InternalClient {
	return m.internalClient
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

// Timeout 获取请求超时时间
func (m *Manager) Timeout() time.Duration {
	return m.config.Timeout
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
