package databases

import (
	"context"
	"fmt"
	"log"

	redis "github.com/go-redis/redis/v7"
)

// RedisConfig Redis 数据库配置
type RedisConfig struct {
	Host                  string   `json:"host" mapstructure:"host"`
	Port                  int      `json:"port" mapstructure:"port"`
	Addrs                 []string `json:"addrs" mapstructure:"addrs"`
	Password              string   `json:"password" mapstructure:"password"`
	Database              int      `json:"database" mapstructure:"database"`
	MaxIdle               int      `json:"max-idle" mapstructure:"max-idle"`
	MaxActive             int      `json:"max-active" mapstructure:"max-active"`
	Timeout               int      `json:"timeout" mapstructure:"timeout"`
	EnableCluster         bool     `json:"enable-cluster" mapstructure:"enable-cluster"`
	UseSSL                bool     `json:"use-ssl" mapstructure:"use-ssl"`
	SSLInsecureSkipVerify bool     `json:"ssl-insecure-skip-verify" mapstructure:"ssl-insecure-skip-verify"`
}

// RedisConnection Redis 连接实现
type RedisConnection struct {
	config *RedisConfig
	client redis.UniversalClient
}

// NewRedisConnection 创建 Redis 连接
func NewRedisConnection(config *RedisConfig) *RedisConnection {
	return &RedisConnection{
		config: config,
	}
}

// Type 返回数据库类型
func (r *RedisConnection) Type() DatabaseType {
	return Redis
}

// Connect 连接 Redis 数据库
func (r *RedisConnection) Connect() error {
	var addrs []string
	if len(r.config.Addrs) > 0 {
		addrs = r.config.Addrs
	} else {
		addr := fmt.Sprintf("%s:%d", r.config.Host, r.config.Port)
		addrs = []string{addr}
	}

	var client redis.UniversalClient

	if r.config.EnableCluster {
		client = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:    addrs,
			Password: r.config.Password,
			PoolSize: r.config.MaxActive,
		})
	} else {
		client = redis.NewClient(&redis.Options{
			Addr:     addrs[0],
			Password: r.config.Password,
			DB:       r.config.Database,
			PoolSize: r.config.MaxActive,
		})
	}

	// 测试连接
	if err := client.Ping().Err(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	r.client = client
	log.Printf("Redis connected successfully to %v", addrs)
	return nil
}

// Close 关闭 Redis 连接
func (r *RedisConnection) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// HealthCheck 检查 Redis 连接是否健康
func (r *RedisConnection) HealthCheck(ctx context.Context) error {
	if r.client == nil {
		return fmt.Errorf("Redis client is nil")
	}

	return r.client.Ping().Err()
}

// GetClient 获取 Redis 客户端
func (r *RedisConnection) GetClient() interface{} {
	return r.client
}
