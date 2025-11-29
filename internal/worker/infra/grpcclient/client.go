package grpcclient

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// ClientConfig gRPC 客户端配置
type ClientConfig struct {
	// Apiserver gRPC 地址
	ApiserverAddr string
	// 连接超时
	DialTimeout time.Duration
	// 请求超时
	RequestTimeout time.Duration
}

// DefaultClientConfig 默认配置
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		ApiserverAddr:  "127.0.0.1:9090",
		DialTimeout:    5 * time.Second,
		RequestTimeout: 10 * time.Second,
	}
}

// ClientManager gRPC 客户端管理器
type ClientManager struct {
	config *ClientConfig
	conn   *grpc.ClientConn
}

// NewClientManager 创建客户端管理器
func NewClientManager(config *ClientConfig) *ClientManager {
	if config == nil {
		config = DefaultClientConfig()
	}
	return &ClientManager{
		config: config,
	}
}

// Connect 建立 gRPC 连接
func (m *ClientManager) Connect(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, m.config.DialTimeout)
	defer cancel()

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second,
			Timeout:             3 * time.Second,
			PermitWithoutStream: true,
		}),
	}

	conn, err := grpc.DialContext(ctx, m.config.ApiserverAddr, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to apiserver: %w", err)
	}

	m.conn = conn
	return nil
}

// Connection 获取 gRPC 连接
func (m *ClientManager) Connection() *grpc.ClientConn {
	return m.conn
}

// Close 关闭连接
func (m *ClientManager) Close() error {
	if m.conn != nil {
		return m.conn.Close()
	}
	return nil
}

// RequestTimeout 获取请求超时时间
func (m *ClientManager) RequestTimeout() time.Duration {
	return m.config.RequestTimeout
}
