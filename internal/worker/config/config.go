package config

import (
	"time"

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
	// Notification 配置
	Notification *NotificationConfig
	// PlanScheduler 配置
	PlanScheduler *PlanSchedulerConfig
	// Redis 配置
	Redis *genericoptions.RedisOptions
	// 可选 Redis profiles 配置
	RedisProfiles map[string]*genericoptions.RedisOptions
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

// PlanSchedulerConfig worker plan scheduler 配置。
type PlanSchedulerConfig struct {
	Enable       bool
	OrgIDs       []int64
	InitialDelay time.Duration
	Interval     time.Duration
	LockKey      string
	LockTTL      time.Duration
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
		PlanScheduler: &PlanSchedulerConfig{
			Enable:       opts.PlanScheduler.Enable,
			OrgIDs:       append([]int64(nil), opts.PlanScheduler.OrgIDs...),
			InitialDelay: opts.PlanScheduler.InitialDelay,
			Interval:     opts.PlanScheduler.Interval,
			LockKey:      opts.PlanScheduler.LockKey,
			LockTTL:      opts.PlanScheduler.LockTTL,
		},
		Redis:         opts.Redis,
		RedisProfiles: opts.RedisProfiles,
	}, nil
}
