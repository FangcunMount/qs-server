package options

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/log"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	cliflag "github.com/FangcunMount/qs-server/pkg/flag"
	"github.com/spf13/pflag"
)

// Options 包含所有配置项
type Options struct {
	Log                     *log.Options                           `json:"log"      mapstructure:"log"`
	GenericServerRunOptions *genericoptions.ServerRunOptions       `json:"server"   mapstructure:"server"`
	InsecureServing         *genericoptions.InsecureServingOptions `json:"insecure" mapstructure:"insecure"`
	SecureServing           *genericoptions.SecureServingOptions   `json:"secure"   mapstructure:"secure"`
	GRPCClient              *GRPCClientOptions                     `json:"grpc_client" mapstructure:"grpc_client"`
	RedisDualOptions        *genericoptions.RedisDualOptions       `json:"redis"     mapstructure:"redis"`
	Concurrency             *ConcurrencyOptions                    `json:"concurrency" mapstructure:"concurrency"`
	JWT                     *JWTOptions                            `json:"jwt" mapstructure:"jwt"`
	IAMOptions              *genericoptions.IAMOptions             `json:"iam" mapstructure:"iam"`
	Runtime                 *RuntimeOptions                        `json:"runtime" mapstructure:"runtime"`
}

// GRPCClientOptions GRPC 客户端配置
type GRPCClientOptions struct {
	Endpoint string `json:"endpoint" mapstructure:"endpoint"`
	Timeout  int    `json:"timeout"  mapstructure:"timeout"`  // 超时时间（秒）
	Insecure bool   `json:"insecure" mapstructure:"insecure"` // 是否使用不安全连接

	// TLS 配置
	TLSCertFile   string `json:"tls_cert_file"   mapstructure:"tls-cert-file"`   // 客户端证书文件
	TLSKeyFile    string `json:"tls_key_file"    mapstructure:"tls-key-file"`    // 客户端密钥文件
	TLSCAFile     string `json:"tls_ca_file"     mapstructure:"tls-ca-file"`     // CA 证书文件
	TLSServerName string `json:"tls_server_name" mapstructure:"tls-server-name"` // 服务端名称（用于验证）
}

// ConcurrencyOptions 并发处理配置
type ConcurrencyOptions struct {
	MaxConcurrency int `json:"max_concurrency" mapstructure:"max_concurrency"` // 最大并发数
}

// RuntimeOptions 运行时调优（GC/内存）
type RuntimeOptions struct {
	GoMemLimit string `json:"go-mem-limit" mapstructure:"go-mem-limit"` // GOMEMLIMIT，例如 "700MiB"
	GoGC       int    `json:"gogc" mapstructure:"gogc"`                 // GOGC 百分比
}

// JWTOptions JWT 配置
type JWTOptions struct {
	SecretKey     string `json:"secret_key" mapstructure:"secret_key"`         // JWT 密钥
	TokenDuration int    `json:"token_duration" mapstructure:"token_duration"` // Token 有效期（小时）
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
		RedisDualOptions: genericoptions.NewRedisDualOptions(),
		Concurrency: &ConcurrencyOptions{
			MaxConcurrency: 10, // 默认最大并发数
		},
		JWT: &JWTOptions{
			SecretKey:     "your-secret-key-change-in-production",
			TokenDuration: 24 * 7, // 7 天
		},
		IAMOptions: genericoptions.NewIAMOptions(),
		Runtime:    NewRuntimeOptions(),
	}
}

// NewRuntimeOptions 创建默认运行时调优选项
func NewRuntimeOptions() *RuntimeOptions {
	return &RuntimeOptions{
		GoMemLimit: "",
		GoGC:       100,
	}
}

// Flags 返回一个 NamedFlagSets 对象，包含所有命令行参数
func (o *Options) Flags() (fss cliflag.NamedFlagSets) {
	o.Log.AddFlags(fss.FlagSet("log"))
	o.GenericServerRunOptions.AddFlags(fss.FlagSet("server"))
	o.InsecureServing.AddFlags(fss.FlagSet("insecure"))
	o.SecureServing.AddFlags(fss.FlagSet("secure"))
	o.IAMOptions.AddFlags(fss.FlagSet("iam"))
	o.GRPCClient.AddFlags(fss.FlagSet("grpc_client"))
	o.RedisDualOptions.AddFlags(fss.FlagSet("redis"))
	o.Concurrency.AddFlags(fss.FlagSet("concurrency"))
	o.Runtime.AddFlags(fss.FlagSet("runtime"))
	o.JWT.AddFlags(fss.FlagSet("jwt"))

	return fss
}

// AddFlags 添加 GRPC 客户端相关的命令行参数
func (g *GRPCClientOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&g.Endpoint, "grpc_client.endpoint", g.Endpoint,
		"The endpoint of apiserver gRPC service.")
	fs.IntVar(&g.Timeout, "grpc_client.timeout", g.Timeout,
		"The timeout for gRPC client requests in seconds.")
	fs.BoolVar(&g.Insecure, "grpc_client.insecure", g.Insecure,
		"Whether to use insecure gRPC connection.")
}

// AddFlags 添加并发处理相关的命令行参数
func (c *ConcurrencyOptions) AddFlags(fs *pflag.FlagSet) {
	fs.IntVar(&c.MaxConcurrency, "concurrency.max-concurrency", c.MaxConcurrency,
		"The maximum number of concurrent goroutines for validation.")
}

// AddFlags 添加运行时相关参数
func (r *RuntimeOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&r.GoMemLimit, "runtime.go-mem-limit", r.GoMemLimit,
		"GOMEMLIMIT setting, e.g., 700MiB")
	fs.IntVar(&r.GoGC, "runtime.gogc", r.GoGC,
		"GOGC percentage, e.g., 50")
}

// AddFlags 添加 JWT 相关的命令行参数
func (j *JWTOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&j.SecretKey, "jwt.secret-key", j.SecretKey,
		"The secret key for JWT token signing.")
	fs.IntVar(&j.TokenDuration, "jwt.token-duration", j.TokenDuration,
		"The duration of JWT token in hours.")
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
		errs = append(errs, fmt.Errorf("grpc_client.endpoint cannot be empty"))
	}
	if o.GRPCClient.Timeout <= 0 {
		errs = append(errs, fmt.Errorf("grpc_client.timeout must be greater than 0"))
	}

	// 验证 Redis 配置（cache/store 至少需要配置主机和端口）
	if o.RedisDualOptions.Cache.Host == "" {
		errs = append(errs, fmt.Errorf("redis.cache.host cannot be empty"))
	}
	if o.RedisDualOptions.Cache.Port <= 0 {
		errs = append(errs, fmt.Errorf("redis.cache.port must be greater than 0"))
	}
	if o.RedisDualOptions.Store.Host == "" {
		errs = append(errs, fmt.Errorf("redis.store.host cannot be empty"))
	}
	if o.RedisDualOptions.Store.Port <= 0 {
		errs = append(errs, fmt.Errorf("redis.store.port must be greater than 0"))
	}

	// 验证并发配置
	if o.Concurrency.MaxConcurrency <= 0 {
		errs = append(errs, fmt.Errorf("concurrency.max-concurrency must be greater than 0"))
	}
	if o.Concurrency.MaxConcurrency > 100 {
		errs = append(errs, fmt.Errorf("concurrency.max-concurrency cannot be greater than 100"))
	}

	// 验证 JWT 配置
	if o.JWT.SecretKey == "" {
		errs = append(errs, fmt.Errorf("jwt.secret-key cannot be empty"))
	}
	if o.JWT.TokenDuration <= 0 {
		errs = append(errs, fmt.Errorf("jwt.token-duration must be greater than 0"))
	}

	return errs
}
