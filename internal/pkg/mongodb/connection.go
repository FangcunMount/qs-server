package mongodb

import (
	"context"
	"fmt"
	"time"

	componentdb "github.com/FangcunMount/component-base/pkg/database"
	"github.com/FangcunMount/component-base/pkg/logger"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

const (
	defaultConnectTimeout = 5 * time.Second
	defaultPingTimeout    = 10 * time.Second
)

// ConnectionConfig 定义本地 MongoDB 连接配置。
type ConnectionConfig struct {
	URI           string
	EnableLogger  bool
	SlowThreshold time.Duration
}

// Connection 是支持显式 URI 的 MongoDB 连接实现。
type Connection struct {
	config *ConnectionConfig
	client *mongo.Client
}

// NewConnection 创建 MongoDB 连接。
func NewConnection(config *ConnectionConfig) *Connection {
	return &Connection{config: config}
}

// Type 返回数据库类型。
func (c *Connection) Type() componentdb.DatabaseType {
	return componentdb.MongoDB
}

// Connect 建立 MongoDB 连接。
func (c *Connection) Connect() error {
	if c.config == nil || c.config.URI == "" {
		return fmt.Errorf("mongodb uri is empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultPingTimeout)
	defer cancel()

	clientOptions := options.Client().ApplyURI(c.config.URI)
	clientOptions.SetConnectTimeout(defaultConnectTimeout)
	clientOptions.SetServerSelectionTimeout(defaultConnectTimeout)

	if c.config.EnableLogger {
		slowThreshold := c.config.SlowThreshold
		if slowThreshold <= 0 {
			slowThreshold = 200 * time.Millisecond
		}

		mongoHook := logger.NewMongoHook(true, slowThreshold)
		clientOptions.SetMonitor(mongoHook.CommandMonitor())
		clientOptions.SetPoolMonitor(mongoHook.PoolMonitor())
		clientOptions.SetServerMonitor(mongoHook.ServerMonitor())
	}

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	c.client = client
	return nil
}

// Close 关闭 MongoDB 连接。
func (c *Connection) Close() error {
	if c.client == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.client.Disconnect(ctx)
}

// HealthCheck 检查 MongoDB 连接是否健康。
func (c *Connection) HealthCheck(ctx context.Context) error {
	if c.client == nil {
		return fmt.Errorf("mongodb client is nil")
	}
	return c.client.Ping(ctx, readpref.Primary())
}

// GetClient 返回原始 MongoDB 客户端。
func (c *Connection) GetClient() interface{} {
	return c.client
}
