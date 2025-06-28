package databases

import (
	"context"
	"fmt"
	"log"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// MySQLConfig MySQL 数据库配置
type MySQLConfig struct {
	Host                  string        `json:"host" mapstructure:"host"`
	Username              string        `json:"username" mapstructure:"username"`
	Password              string        `json:"password" mapstructure:"password"`
	Database              string        `json:"database" mapstructure:"database"`
	MaxIdleConnections    int           `json:"max-idle-connections" mapstructure:"max-idle-connections"`
	MaxOpenConnections    int           `json:"max-open-connections" mapstructure:"max-open-connections"`
	MaxConnectionLifeTime time.Duration `json:"max-connection-life-time" mapstructure:"max-connection-life-time"`
	LogLevel              int           `json:"log-level" mapstructure:"log-level"`
	Logger                logger.Interface
}

// MySQLConnection MySQL 连接实现
type MySQLConnection struct {
	config *MySQLConfig
	client *gorm.DB
}

// NewMySQLConnection 创建 MySQL 连接
func NewMySQLConnection(config *MySQLConfig) *MySQLConnection {
	return &MySQLConnection{
		config: config,
	}
}

// Type 返回数据库类型
func (m *MySQLConnection) Type() DatabaseType {
	return MySQL
}

// Connect 连接 MySQL 数据库
func (m *MySQLConnection) Connect() error {
	dsn := fmt.Sprintf(`%s:%s@tcp(%s)/%s?charset=utf8&parseTime=%t&loc=%s`,
		m.config.Username,
		m.config.Password,
		m.config.Host,
		m.config.Database,
		true,
		"Local")

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: m.config.Logger,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to MySQL: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// 设置连接池参数
	sqlDB.SetMaxOpenConns(m.config.MaxOpenConnections)
	sqlDB.SetConnMaxLifetime(m.config.MaxConnectionLifeTime)
	sqlDB.SetMaxIdleConns(m.config.MaxIdleConnections)

	m.client = db
	log.Printf("MySQL connected successfully to %s/%s", m.config.Host, m.config.Database)
	return nil
}

// Close 关闭 MySQL 连接
func (m *MySQLConnection) Close() error {
	if m.client != nil {
		if sqlDB, err := m.client.DB(); err == nil {
			return sqlDB.Close()
		}
	}
	return nil
}

// HealthCheck 检查 MySQL 连接是否健康
func (m *MySQLConnection) HealthCheck(ctx context.Context) error {
	if m.client == nil {
		return fmt.Errorf("MySQL client is nil")
	}

	if sqlDB, err := m.client.DB(); err == nil {
		return sqlDB.PingContext(ctx)
	}

	return fmt.Errorf("failed to get MySQL sql.DB for health check")
}

// GetClient 获取 MySQL 客户端
func (m *MySQLConnection) GetClient() interface{} {
	return m.client
}
