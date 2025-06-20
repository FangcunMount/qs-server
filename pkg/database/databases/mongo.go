package databases

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/vinllen/mgo"
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
	client *mgo.Session
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
	dialInfo, err := mgo.ParseURL(m.config.URL)
	if err != nil {
		return fmt.Errorf("failed to parse MongoDB URL: %w", err)
	}

	dialInfo.Timeout = 5 * time.Second

	session, err := mgo.DialWithInfo(dialInfo)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// 测试连接
	if err := session.Ping(); err != nil {
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	m.client = session
	log.Printf("MongoDB connected successfully")
	return nil
}

// Close 关闭 MongoDB 连接
func (m *MongoDBConnection) Close() error {
	if m.client != nil {
		m.client.Close()
	}
	return nil
}

// HealthCheck 检查 MongoDB 连接是否健康
func (m *MongoDBConnection) HealthCheck(ctx context.Context) error {
	if m.client == nil {
		return fmt.Errorf("MongoDB client is nil")
	}

	return m.client.Ping()
}

// GetClient 获取 MongoDB 客户端
func (m *MongoDBConnection) GetClient() interface{} {
	return m.client
}
