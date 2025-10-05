package options

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/pflag"
	genericoptions "github.com/fangcun-mount/qs-server/internal/pkg/options"
	cliflag "github.com/fangcun-mount/qs-server/pkg/flag"
	"github.com/fangcun-mount/qs-server/pkg/log"
	"github.com/fangcun-mount/qs-server/pkg/pubsub"
)

// Options 包含所有配置项
type Options struct {
	Log                     *log.Options                           `json:"log"      mapstructure:"log"`
	GenericServerRunOptions *genericoptions.ServerRunOptions       `json:"server"   mapstructure:"server"`
	InsecureServing         *genericoptions.InsecureServingOptions `json:"insecure" mapstructure:"insecure"`
	SecureServing           *genericoptions.SecureServingOptions   `json:"secure"   mapstructure:"secure"`
	// GRPC 客户端配置，用于连接 apiserver
	GRPCClient *GRPCClientOptions `json:"grpc_client" mapstructure:"grpc_client"`
	// Redis 配置，用于消息队列
	Redis *genericoptions.RedisOptions `json:"redis" mapstructure:"redis"`
	// 并发处理配置
	Concurrency *ConcurrencyOptions `json:"concurrency" mapstructure:"concurrency"`
}

// GRPCClientOptions GRPC 客户端配置
type GRPCClientOptions struct {
	Endpoint string `json:"endpoint" mapstructure:"endpoint"`
	Timeout  int    `json:"timeout"  mapstructure:"timeout"`  // 超时时间（秒）
	Insecure bool   `json:"insecure" mapstructure:"insecure"` // 是否使用不安全连接
}

// ConcurrencyOptions 并发处理配置
type ConcurrencyOptions struct {
	MaxConcurrency int `json:"max_concurrency" mapstructure:"max_concurrency"` // 最大并发数
}

// LoggingOptions 日志配置选项
type LoggingOptions struct {
	// EnableAPILogging 是否启用详细API日志
	EnableAPILogging bool `json:"enable_api_logging" mapstructure:"enable_api_logging"`

	// EnableGRPCLogging 是否启用gRPC日志
	EnableGRPCLogging bool `json:"enable_grpc_logging" mapstructure:"enable_grpc_logging"`

	// LogLevel 日志级别 (0=INFO, 1=DEBUG)
	LogLevel int `json:"log_level" mapstructure:"log_level"`

	// MaxBodySize API日志最大记录体大小
	MaxBodySize int64 `json:"max_body_size" mapstructure:"max_body_size"`

	// MaxPayloadSize gRPC日志最大载荷大小
	MaxPayloadSize int `json:"max_payload_size" mapstructure:"max_payload_size"`
}

// NewLoggingOptions 创建默认日志选项
func NewLoggingOptions() *LoggingOptions {
	return &LoggingOptions{
		EnableAPILogging:  true,
		EnableGRPCLogging: true,
		LogLevel:          0,         // INFO level
		MaxBodySize:       10 * 1024, // 10KB
		MaxPayloadSize:    2048,      // 2KB
	}
}

// NewOptions 创建一个 Options 对象，包含默认参数
func NewOptions() *Options {
	return &Options{
		Log:                     log.NewOptions(),
		GenericServerRunOptions: genericoptions.NewServerRunOptions(),
		InsecureServing:         genericoptions.NewInsecureServingOptions(),
		SecureServing:           genericoptions.NewSecureServingOptions(),
		GRPCClient: &GRPCClientOptions{
			Endpoint: "localhost:9090", // apiserver 的 GRPC 端口
			Timeout:  30,
			Insecure: true,
		},
		Redis: genericoptions.NewRedisOptions(),
		Concurrency: &ConcurrencyOptions{
			MaxConcurrency: 10, // 默认最大并发数
		},
	}
}

// Flags 返回一个 NamedFlagSets 对象，包含所有命令行参数
func (o *Options) Flags() (fss cliflag.NamedFlagSets) {
	o.Log.AddFlags(fss.FlagSet("log"))
	o.GenericServerRunOptions.AddFlags(fss.FlagSet("server"))
	o.InsecureServing.AddFlags(fss.FlagSet("insecure"))
	o.SecureServing.AddFlags(fss.FlagSet("secure"))
	o.GRPCClient.AddFlags(fss.FlagSet("grpc-client"))
	o.Redis.AddFlags(fss.FlagSet("redis"))
	o.Concurrency.AddFlags(fss.FlagSet("concurrency"))

	return fss
}

// AddFlags 添加 GRPC 客户端相关的命令行参数
func (g *GRPCClientOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&g.Endpoint, "grpc-client.endpoint", g.Endpoint,
		"The endpoint of apiserver gRPC service.")
	fs.IntVar(&g.Timeout, "grpc-client.timeout", g.Timeout,
		"The timeout for gRPC client requests in seconds.")
	fs.BoolVar(&g.Insecure, "grpc-client.insecure", g.Insecure,
		"Whether to use insecure gRPC connection.")
}

// AddFlags 添加并发处理相关的命令行参数
func (c *ConcurrencyOptions) AddFlags(fs *pflag.FlagSet) {
	fs.IntVar(&c.MaxConcurrency, "concurrency.max-concurrency", c.MaxConcurrency,
		"The maximum number of concurrent goroutines for validation.")
}

// ToPubSubConfig 将RedisOptions转换为pubsub.Config
func (o *Options) ToPubSubConfig() *pubsub.Config {
	addr := fmt.Sprintf("%s:%d", o.Redis.Host, o.Redis.Port)
	config := pubsub.DefaultConfig()
	config.Addr = addr
	config.Password = o.Redis.Password
	config.DB = o.Redis.Database
	config.ConsumerGroup = "collection-server-group"
	config.Consumer = "collection-server-consumer"
	return config
}

// Complete 完成配置选项
func (o *Options) Complete() error {
	return o.SecureServing.Complete()
}

// String 返回配置的字符串表示
func (o *Options) String() string {
	data, _ := json.Marshal(o)
	return string(data)
}

// Validate 验证配置选项
func (o *Options) Validate() []error {
	var errs []error

	errs = append(errs, o.GenericServerRunOptions.Validate()...)
	errs = append(errs, o.Log.Validate()...)

	// 验证 GRPC 客户端配置
	if o.GRPCClient.Endpoint == "" {
		errs = append(errs, fmt.Errorf("grpc-client.endpoint cannot be empty"))
	}
	if o.GRPCClient.Timeout <= 0 {
		errs = append(errs, fmt.Errorf("grpc-client.timeout must be greater than 0"))
	}

	// 验证 Redis 配置
	if o.Redis.Host == "" {
		errs = append(errs, fmt.Errorf("redis.host cannot be empty"))
	}
	if o.Redis.Port <= 0 {
		errs = append(errs, fmt.Errorf("redis.port must be greater than 0"))
	}

	// 验证并发配置
	if o.Concurrency.MaxConcurrency <= 0 {
		errs = append(errs, fmt.Errorf("concurrency.max-concurrency must be greater than 0"))
	}
	if o.Concurrency.MaxConcurrency > 100 {
		errs = append(errs, fmt.Errorf("concurrency.max-concurrency cannot be greater than 100"))
	}

	return errs
}
