package databases

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// MongoConfig MongoDB 数据库配置
type MongoConfig struct {
	URL                      string `json:"url" mapstructure:"url"`
	UseSSL                   bool   `json:"use-ssl" mapstructure:"use-ssl"`
	SSLInsecureSkipVerify    bool   `json:"ssl-insecure-skip-verify" mapstructure:"ssl-insecure-skip-verify"`
	SSLAllowInvalidHostnames bool   `json:"ssl-allow-invalid-hostnames" mapstructure:"ssl-allow-invalid-hostnames"`
	SSLCAFile                string `json:"ssl-ca-file" mapstructure:"ssl-ca-file"`
	SSLPEMKeyfile            string `json:"ssl-pem-keyfile" mapstructure:"ssl-pem-keyfile"`
}

// MongoDBConnection MongoDB 连接实现
type MongoDBConnection struct {
	config *MongoConfig
	client *mongo.Client
}

// NewMongoDBConnection 创建 MongoDB 连接
func NewMongoDBConnection(config *MongoConfig) *MongoDBConnection {
	return &MongoDBConnection{
		config: config,
	}
}

// Type 返回数据库类型
func (m *MongoDBConnection) Type() DatabaseType {
	return MongoDB
}

// Connect 连接 MongoDB 数据库
func (m *MongoDBConnection) Connect() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 创建连接选项
	clientOptions := options.Client().ApplyURI(m.config.URL)

	// 设置连接超时
	clientOptions.SetConnectTimeout(5 * time.Second)
	clientOptions.SetServerSelectionTimeout(5 * time.Second)

	// 连接到MongoDB
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// 测试连接
	err = client.Ping(ctx, readpref.Primary())
	if err != nil {
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	m.client = client
	log.Printf("MongoDB connected successfully")
	return nil
}

// Close 关闭 MongoDB 连接
func (m *MongoDBConnection) Close() error {
	if m.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return m.client.Disconnect(ctx)
	}
	return nil
}

// HealthCheck 检查 MongoDB 连接是否健康
func (m *MongoDBConnection) HealthCheck(ctx context.Context) error {
	if m.client == nil {
		return fmt.Errorf("MongoDB client is nil")
	}

	return m.client.Ping(ctx, readpref.Primary())
}

// GetClient 获取 MongoDB 客户端
func (m *MongoDBConnection) GetClient() interface{} {
	return m.client
}
