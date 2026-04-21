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
	// Metrics 观测服务配置
	Metrics *MetricsConfig
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
	// Notification 配置
	Notification *NotificationConfig
	// Redis 配置
	Redis *genericoptions.RedisOptions
	// 可选 Redis profiles 配置
	RedisProfiles map[string]*genericoptions.RedisOptions
	// Redis runtime family 路由
	RedisRuntime *genericoptions.RedisRuntimeOptions
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string
	Format string
}

// MetricsConfig worker metrics/health listener 配置。
type MetricsConfig struct {
	Enable      bool
	BindAddress string
	BindPort    int
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
	Host     string
	Username string
	Password string
	Database string
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
	Insecure      bool
	TLSCertFile   string
	TLSKeyFile    string
	TLSCAFile     string
	TLSServerName string
}

// WorkerConfig Worker 运行配置
type WorkerConfig struct {
	Concurrency int
	MaxRetries  int
	ServiceName string
}

// NotificationConfig 通知配置。
type NotificationConfig struct {
	GatewayURL   string
	GatewayToken string
	WebhookURL   string
	TimeoutMs    int
	SharedSecret string
}

// CreateConfigFromOptions 从 Options 创建 Config
func CreateConfigFromOptions(opts *options.Options) (*Config, error) {
	return &Config{
		Options: opts,
		Log: &LogConfig{
			Level:  opts.Log.Level,
			Format: opts.Log.Format,
		},
		Metrics: &MetricsConfig{
			Enable:      opts.Metrics.Enable,
			BindAddress: opts.Metrics.BindAddress,
			BindPort:    opts.Metrics.BindPort,
		},
		MySQL: &MySQLConfig{
			Host:     opts.MySQL.Host,
			Username: opts.MySQL.Username,
			Password: opts.MySQL.Password,
			Database: opts.MySQL.Database,
		},
		MongoDB: &MongoDBConfig{
			Host:     opts.MongoDB.Host,
			Username: opts.MongoDB.Username,
			Password: opts.MongoDB.Password,
			Database: opts.MongoDB.Database,
		},
		Messaging: &MessagingConfig{
			Provider:       opts.Messaging.Provider,
			NSQAddr:        opts.Messaging.NSQAddr,
			NSQLookupdAddr: opts.Messaging.NSQLookupdAddr,
			RabbitMQURL:    opts.Messaging.RabbitMQURL,
		},
		GRPC: &GRPCConfig{
			ApiserverAddr: opts.GRPC.ApiserverAddr,
			Insecure:      opts.GRPC.Insecure,
			TLSCertFile:   opts.GRPC.TLSCertFile,
			TLSKeyFile:    opts.GRPC.TLSKeyFile,
			TLSCAFile:     opts.GRPC.TLSCAFile,
			TLSServerName: opts.GRPC.TLSServerName,
		},
		Worker: &WorkerConfig{
			Concurrency: opts.Worker.Concurrency,
			MaxRetries:  opts.Worker.MaxRetries,
			ServiceName: opts.Worker.ServiceName,
		},
		Notification: &NotificationConfig{
			GatewayURL:   opts.Notification.GatewayURL,
			GatewayToken: opts.Notification.GatewayToken,
			WebhookURL:   opts.Notification.WebhookURL,
			TimeoutMs:    opts.Notification.TimeoutMs,
			SharedSecret: opts.Notification.SharedSecret,
		},
		Redis:         opts.Redis,
		RedisProfiles: opts.RedisProfiles,
		RedisRuntime:  opts.RedisRuntime,
	}, nil
}
