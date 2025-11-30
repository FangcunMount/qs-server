package config

import (
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/worker/options"
)

// Config Worker 运行时配置
type Config struct {
	// Options 原始配置选项
	Options *options.Options
	// Log 日志配置
	Log *LogConfig
	// MySQL 数据库配置
	MySQL *MySQLConfig
	// MongoDB 配置
	MongoDB *MongoDBConfig
	// Messaging 消息队列配置
	Messaging *MessagingConfig
	// GRPC gRPC 客户端配置
	GRPC *GRPCConfig
	// Worker 配置
	Worker *WorkerConfig
	// Redis 双实例配置
	Redis *genericoptions.RedisDualOptions
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string
	Format string
}

// MySQLConfig MySQL 配置
type MySQLConfig struct {
	Host     string
	Username string
	Password string
	Database string
}

// MongoDBConfig MongoDB 配置
type MongoDBConfig struct {
	URL string
}

// MessagingConfig 消息队列配置
type MessagingConfig struct {
	Provider       string
	NSQAddr        string
	NSQLookupdAddr string
	RabbitMQURL    string
}

// GRPCConfig gRPC 客户端配置
type GRPCConfig struct {
	ApiserverAddr string
}

// WorkerConfig Worker 运行配置
type WorkerConfig struct {
	Concurrency int
	MaxRetries  int
	ServiceName string
}

// CreateConfigFromOptions 从 Options 创建 Config
func CreateConfigFromOptions(opts *options.Options) (*Config, error) {
	return &Config{
		Options: opts,
		Log: &LogConfig{
			Level:  opts.Log.Level,
			Format: opts.Log.Format,
		},
		MySQL: &MySQLConfig{
			Host:     opts.MySQL.Host,
			Username: opts.MySQL.Username,
			Password: opts.MySQL.Password,
			Database: opts.MySQL.Database,
		},
		MongoDB: &MongoDBConfig{
			URL: opts.MongoDB.URL,
		},
		Messaging: &MessagingConfig{
			Provider:       opts.Messaging.Provider,
			NSQAddr:        opts.Messaging.NSQAddr,
			NSQLookupdAddr: opts.Messaging.NSQLookupdAddr,
			RabbitMQURL:    opts.Messaging.RabbitMQURL,
		},
		GRPC: &GRPCConfig{
			ApiserverAddr: opts.GRPC.ApiserverAddr,
		},
		Worker: &WorkerConfig{
			Concurrency: opts.Worker.Concurrency,
			MaxRetries:  opts.Worker.MaxRetries,
			ServiceName: opts.Worker.ServiceName,
		},
		Redis: opts.Redis,
	}, nil
}
