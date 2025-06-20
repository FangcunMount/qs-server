package databases

import "context"

// DatabaseType 数据库类型
type DatabaseType string

const (
	MySQL   DatabaseType = "mysql"
	Redis   DatabaseType = "redis"
	MongoDB DatabaseType = "mongodb"
)

// Connection 数据库连接接口
type Connection interface {
	// Type 返回数据库类型
	Type() DatabaseType

	// Connect 建立连接
	Connect() error

	// Close 关闭连接
	Close() error

	// HealthCheck 健康检查
	HealthCheck(ctx context.Context) error

	// GetClient 获取原始客户端
	GetClient() interface{}
}
