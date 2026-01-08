package options

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/log"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	cliflag "github.com/FangcunMount/qs-server/pkg/flag"
	"github.com/spf13/pflag"
)

// Options 包含 Worker 的所有配置项
type Options struct {
	Log                     *log.Options                     `json:"log"      mapstructure:"log"`
	GenericServerRunOptions *genericoptions.ServerRunOptions `json:"server"   mapstructure:"server"`
	// MySQL 配置
	MySQL *genericoptions.MySQLOptions `json:"mysql" mapstructure:"mysql"`
	// MongoDB 配置
	MongoDB *genericoptions.MongoDBOptions `json:"mongodb" mapstructure:"mongodb"`
	// 消息队列配置
	Messaging *MessagingOptions `json:"messaging" mapstructure:"messaging"`
	// gRPC 客户端配置
	GRPC *GRPCOptions `json:"grpc" mapstructure:"grpc"`
	// Worker 配置
	Worker *WorkerOptions `json:"worker" mapstructure:"worker"`
	// Redis 双实例配置（cache/store）
	Redis *genericoptions.RedisDualOptions `json:"redis" mapstructure:"redis"`
	// Cache 控制缓存输出
	Cache *CacheOptions `json:"cache" mapstructure:"cache"`
}

// MessagingOptions 消息队列配置
type MessagingOptions struct {
	// Provider 消息队列提供者 (nsq, rabbitmq)
	Provider string `json:"provider" mapstructure:"provider"`
	// NSQ 配置
	NSQAddr        string `json:"nsq_addr" mapstructure:"nsq-addr"`
	NSQLookupdAddr string `json:"nsq_lookupd_addr" mapstructure:"nsq-lookupd-addr"`
	// RabbitMQ 配置
	RabbitMQURL string `json:"rabbitmq_url" mapstructure:"rabbitmq_url"`
}

// WorkerOptions Worker 运行配置
type WorkerOptions struct {
	// Concurrency 并发处理数
	Concurrency int `json:"concurrency" mapstructure:"concurrency"`
	// MaxRetries 最大重试次数
	MaxRetries int `json:"max_retries" mapstructure:"max-retries"`
	// ServiceName 服务名称（用于消息队列 channel）
	ServiceName string `json:"service_name" mapstructure:"service-name"`
	// EventConfigPath 事件配置文件路径
	EventConfigPath string `json:"event_config_path" mapstructure:"event-config-path"`
}

// CacheOptions 缓存控制配置
type CacheOptions struct {
	DisableStatisticsCache bool `json:"disable_statistics_cache" mapstructure:"disable_statistics_cache"`
}

// NewCacheOptions 创建缓存控制配置
func NewCacheOptions() *CacheOptions {
	return &CacheOptions{
		DisableStatisticsCache: false,
	}
}

// GRPCOptions gRPC 客户端配置
type GRPCOptions struct {
	// ApiserverAddr apiserver gRPC 服务地址
	ApiserverAddr string `json:"apiserver_addr" mapstructure:"apiserver-addr"`
	// Insecure 是否使用明文连接
	Insecure bool `json:"insecure" mapstructure:"insecure"`
	// TLS 配置
	TLSCertFile   string `json:"tls_cert_file" mapstructure:"tls-cert-file"`
	TLSKeyFile    string `json:"tls_key_file" mapstructure:"tls-key-file"`
	TLSCAFile     string `json:"tls_ca_file" mapstructure:"tls-ca-file"`
	TLSServerName string `json:"tls_server_name" mapstructure:"tls-server-name"`
}

// NewOptions 创建默认配置
func NewOptions() *Options {
	return &Options{
		Log:                     log.NewOptions(),
		GenericServerRunOptions: genericoptions.NewServerRunOptions(),
		MySQL:                   genericoptions.NewMySQLOptions(),
		MongoDB:                 genericoptions.NewMongoDBOptions(),
		Messaging: &MessagingOptions{
			Provider:       "nsq",
			NSQAddr:        "localhost:4150",
			NSQLookupdAddr: "localhost:4161",
		},
		GRPC: &GRPCOptions{
			ApiserverAddr: "localhost:9090",
			Insecure:      true,
		},
		Worker: &WorkerOptions{
			Concurrency: 10,
			MaxRetries:  3,
			ServiceName: "qs-worker",
		},
		Redis: genericoptions.NewRedisDualOptions(),
		Cache: NewCacheOptions(),
	}
}

// Flags 返回命令行参数
func (o *Options) Flags() (fss cliflag.NamedFlagSets) {
	o.Log.AddFlags(fss.FlagSet("log"))
	o.GenericServerRunOptions.AddFlags(fss.FlagSet("server"))
	o.MySQL.AddFlags(fss.FlagSet("mysql"))
	o.MongoDB.AddFlags(fss.FlagSet("mongodb"))

	// Messaging flags
	messagingFS := fss.FlagSet("messaging")
	messagingFS.StringVar(&o.Messaging.Provider, "messaging.provider", o.Messaging.Provider,
		"Message queue provider (nsq, rabbitmq)")
	messagingFS.StringVar(&o.Messaging.NSQAddr, "messaging.nsq-addr", o.Messaging.NSQAddr,
		"NSQ daemon address")
	messagingFS.StringVar(&o.Messaging.NSQLookupdAddr, "messaging.nsq-lookupd-addr", o.Messaging.NSQLookupdAddr,
		"NSQ lookupd address")
	messagingFS.StringVar(&o.Messaging.RabbitMQURL, "messaging.rabbitmq-url", o.Messaging.RabbitMQURL,
		"RabbitMQ connection URL")

	// gRPC flags
	grpcFS := fss.FlagSet("grpc")
	grpcFS.StringVar(&o.GRPC.ApiserverAddr, "grpc.apiserver-addr", o.GRPC.ApiserverAddr,
		"Apiserver gRPC service address")
	grpcFS.BoolVar(&o.GRPC.Insecure, "grpc.insecure", o.GRPC.Insecure,
		"Use insecure gRPC connection (plaintext, no TLS)")
	grpcFS.StringVar(&o.GRPC.TLSCertFile, "grpc.tls-cert-file", o.GRPC.TLSCertFile,
		"gRPC client certificate file (for mTLS)")
	grpcFS.StringVar(&o.GRPC.TLSKeyFile, "grpc.tls-key-file", o.GRPC.TLSKeyFile,
		"gRPC client private key file (for mTLS)")
	grpcFS.StringVar(&o.GRPC.TLSCAFile, "grpc.tls-ca-file", o.GRPC.TLSCAFile,
		"gRPC CA certificate file")
	grpcFS.StringVar(&o.GRPC.TLSServerName, "grpc.tls-server-name", o.GRPC.TLSServerName,
		"gRPC server name override for TLS verification")

	// Worker flags
	workerFS := fss.FlagSet("worker")
	workerFS.IntVar(&o.Worker.Concurrency, "worker.concurrency", o.Worker.Concurrency,
		"Maximum number of concurrent handlers")
	workerFS.IntVar(&o.Worker.MaxRetries, "worker.max-retries", o.Worker.MaxRetries,
		"Maximum retry attempts for failed messages")
	workerFS.StringVar(&o.Worker.ServiceName, "worker.service-name", o.Worker.ServiceName,
		"Service name for message queue channel")

	// Redis flags
	o.Redis.AddFlags(fss.FlagSet("redis"))
	o.Cache.AddFlags(fss.FlagSet("cache"))

	return fss
}

// Validate 验证配置
func (o *Options) Validate() []error {
	var errs []error

	errs = append(errs, o.Log.Validate()...)
	errs = append(errs, o.GenericServerRunOptions.Validate()...)
	errs = append(errs, o.MySQL.Validate()...)
	errs = append(errs, o.MongoDB.Validate()...)

	// Redis 校验（cache/store 主机端口）
	if o.Redis.Cache.Host == "" {
		errs = append(errs, fmt.Errorf("redis.cache.host cannot be empty"))
	}
	if o.Redis.Cache.Port <= 0 {
		errs = append(errs, fmt.Errorf("redis.cache.port must be greater than 0"))
	}
	if o.Redis.Store.Host == "" {
		errs = append(errs, fmt.Errorf("redis.store.host cannot be empty"))
	}
	if o.Redis.Store.Port <= 0 {
		errs = append(errs, fmt.Errorf("redis.store.port must be greater than 0"))
	}

	return errs
}

// Complete 完成配置
func (o *Options) Complete() error {
	return nil
}

// String 返回配置的字符串表示
func (o *Options) String() string {
	data, _ := json.Marshal(o)
	return string(data)
}

// AddFlags 注册 cache 相关命令行参数
func (c *CacheOptions) AddFlags(fs *pflag.FlagSet) {
	if c == nil {
		return
	}
	fs.BoolVar(&c.DisableStatisticsCache, "cache.disable-statistics-cache", c.DisableStatisticsCache,
		"Disable Redis-based statistics caching in worker event handlers")
}
