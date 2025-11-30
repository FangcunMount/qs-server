package grpcclient

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ClientConfig gRPC 客户端配置
type ClientConfig struct {
	Endpoint string        // apiserver 地址，如 "localhost:9090"
	Timeout  time.Duration // 请求超时时间
	Insecure bool          // 是否使用不安全连接（开发环境）
}

// Client gRPC 客户端管理器
type Client struct {
	conn   *grpc.ClientConn
	config *ClientConfig
}

// NewClient 创建 gRPC 客户端
func NewClient(cfg *ClientConfig, dialOpts ...grpc.DialOption) (*Client, error) {
	opts := []grpc.DialOption{
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(10*1024*1024), // 10MB
			grpc.MaxCallSendMsgSize(10*1024*1024), // 10MB
		),
	}

	if cfg.Insecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	opts = append(opts, dialOpts...)

	// 创建连接
	conn, err := grpc.NewClient(cfg.Endpoint, opts...)
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:   conn,
		config: cfg,
	}, nil
}

// Conn 获取 gRPC 连接
func (c *Client) Conn() *grpc.ClientConn {
	return c.conn
}

// Close 关闭连接
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// ContextWithTimeout 创建带超时的 context
func (c *Client) ContextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.config.Timeout > 0 {
		return context.WithTimeout(ctx, c.config.Timeout)
	}
	return context.WithTimeout(ctx, 30*time.Second)
}
