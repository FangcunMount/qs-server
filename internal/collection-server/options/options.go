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
	RateLimit               *RateLimitOptions                      `json:"rate_limit" mapstructure:"rate_limit"`
	SubmitQueue             *SubmitQueueOptions                    `json:"submit_queue" mapstructure:"submit_queue"`
	JWT                     *JWTOptions                            `json:"jwt" mapstructure:"jwt"`
	IAMOptions              *genericoptions.IAMOptions             `json:"iam" mapstructure:"iam"`
	Runtime                 *RuntimeOptions                        `json:"runtime" mapstructure:"runtime"`
}

// GRPCClientOptions GRPC 客户端配置
type GRPCClientOptions struct {
	Endpoint    string `json:"endpoint" mapstructure:"endpoint"`
	Timeout     int    `json:"timeout"  mapstructure:"timeout"`          // 超时时间（秒）
	Insecure    bool   `json:"insecure" mapstructure:"insecure"`         // 是否使用不安全连接
	MaxInflight int    `json:"max_inflight" mapstructure:"max_inflight"` // 最大并发调用数

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

// SubmitQueueOptions 提交排队配置
type SubmitQueueOptions struct {
	Enabled       bool `json:"enabled" mapstructure:"enabled"`
	QueueSize     int  `json:"queue_size" mapstructure:"queue_size"`
	WorkerCount   int  `json:"worker_count" mapstructure:"worker_count"`
	WaitTimeoutMs int  `json:"wait_timeout_ms" mapstructure:"wait_timeout_ms"`
}

// RateLimitOptions 限流配置
type RateLimitOptions struct {
	Enabled               bool    `json:"enabled" mapstructure:"enabled"`
	SubmitGlobalQPS       float64 `json:"submit_global_qps" mapstructure:"submit_global_qps"`
	SubmitGlobalBurst     int     `json:"submit_global_burst" mapstructure:"submit_global_burst"`
	SubmitUserQPS         float64 `json:"submit_user_qps" mapstructure:"submit_user_qps"`
	SubmitUserBurst       int     `json:"submit_user_burst" mapstructure:"submit_user_burst"`
	QueryGlobalQPS        float64 `json:"query_global_qps" mapstructure:"query_global_qps"`
	QueryGlobalBurst      int     `json:"query_global_burst" mapstructure:"query_global_burst"`
	QueryUserQPS          float64 `json:"query_user_qps" mapstructure:"query_user_qps"`
	QueryUserBurst        int     `json:"query_user_burst" mapstructure:"query_user_burst"`
	WaitReportGlobalQPS   float64 `json:"wait_report_global_qps" mapstructure:"wait_report_global_qps"`
	WaitReportGlobalBurst int     `json:"wait_report_global_burst" mapstructure:"wait_report_global_burst"`
	WaitReportUserQPS     float64 `json:"wait_report_user_qps" mapstructure:"wait_report_user_qps"`
	WaitReportUserBurst   int     `json:"wait_report_user_burst" mapstructure:"wait_report_user_burst"`
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
			Endpoint:    "localhost:9090", // apiserver 的 GRPC 端口
			Timeout:     30,
			Insecure:    true,
			MaxInflight: 200,
		},
		RedisDualOptions: genericoptions.NewRedisDualOptions(),
		Concurrency: &ConcurrencyOptions{
			MaxConcurrency: 10, // 默认最大并发数
		},
		RateLimit:   NewRateLimitOptions(),
		SubmitQueue: NewSubmitQueueOptions(),
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

// NewSubmitQueueOptions 创建默认提交排队配置
func NewSubmitQueueOptions() *SubmitQueueOptions {
	return &SubmitQueueOptions{
		Enabled:       true,
		QueueSize:     1000,
		WorkerCount:   8,
		WaitTimeoutMs: 200,
	}
}

// NewRateLimitOptions 创建默认限流配置
func NewRateLimitOptions() *RateLimitOptions {
	return &RateLimitOptions{
		Enabled:               true,
		SubmitGlobalQPS:       200,
		SubmitGlobalBurst:     300,
		SubmitUserQPS:         5,
		SubmitUserBurst:       10,
		QueryGlobalQPS:        200,
		QueryGlobalBurst:      300,
		QueryUserQPS:          10,
		QueryUserBurst:        20,
		WaitReportGlobalQPS:   80,
		WaitReportGlobalBurst: 120,
		WaitReportUserQPS:     2,
		WaitReportUserBurst:   5,
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
	o.RateLimit.AddFlags(fss.FlagSet("rate_limit"))
	o.SubmitQueue.AddFlags(fss.FlagSet("submit_queue"))
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
	fs.IntVar(&g.MaxInflight, "grpc_client.max-inflight", g.MaxInflight,
		"The maximum number of in-flight gRPC calls.")
}

// AddFlags 添加并发处理相关的命令行参数
func (c *ConcurrencyOptions) AddFlags(fs *pflag.FlagSet) {
	fs.IntVar(&c.MaxConcurrency, "concurrency.max-concurrency", c.MaxConcurrency,
		"The maximum number of concurrent goroutines for validation.")
}

// AddFlags 添加提交排队相关的命令行参数
func (s *SubmitQueueOptions) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&s.Enabled, "submit_queue.enabled", s.Enabled, "Enable submit queue.")
	fs.IntVar(&s.QueueSize, "submit_queue.queue-size", s.QueueSize, "Submit queue size.")
	fs.IntVar(&s.WorkerCount, "submit_queue.worker-count", s.WorkerCount, "Submit queue worker count.")
	fs.IntVar(&s.WaitTimeoutMs, "submit_queue.wait-timeout-ms", s.WaitTimeoutMs, "Submit queue wait timeout in milliseconds.")
}

// AddFlags 添加限流相关的命令行参数
func (r *RateLimitOptions) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&r.Enabled, "rate_limit.enabled", r.Enabled, "Enable rate limiting.")
	fs.Float64Var(&r.SubmitGlobalQPS, "rate_limit.submit-global-qps", r.SubmitGlobalQPS, "Global QPS limit for submit.")
	fs.IntVar(&r.SubmitGlobalBurst, "rate_limit.submit-global-burst", r.SubmitGlobalBurst, "Global burst for submit.")
	fs.Float64Var(&r.SubmitUserQPS, "rate_limit.submit-user-qps", r.SubmitUserQPS, "Per-user QPS limit for submit.")
	fs.IntVar(&r.SubmitUserBurst, "rate_limit.submit-user-burst", r.SubmitUserBurst, "Per-user burst for submit.")
	fs.Float64Var(&r.QueryGlobalQPS, "rate_limit.query-global-qps", r.QueryGlobalQPS, "Global QPS limit for queries.")
	fs.IntVar(&r.QueryGlobalBurst, "rate_limit.query-global-burst", r.QueryGlobalBurst, "Global burst for queries.")
	fs.Float64Var(&r.QueryUserQPS, "rate_limit.query-user-qps", r.QueryUserQPS, "Per-user QPS limit for queries.")
	fs.IntVar(&r.QueryUserBurst, "rate_limit.query-user-burst", r.QueryUserBurst, "Per-user burst for queries.")
	fs.Float64Var(&r.WaitReportGlobalQPS, "rate_limit.wait-report-global-qps", r.WaitReportGlobalQPS, "Global QPS limit for wait-report.")
	fs.IntVar(&r.WaitReportGlobalBurst, "rate_limit.wait-report-global-burst", r.WaitReportGlobalBurst, "Global burst for wait-report.")
	fs.Float64Var(&r.WaitReportUserQPS, "rate_limit.wait-report-user-qps", r.WaitReportUserQPS, "Per-user QPS limit for wait-report.")
	fs.IntVar(&r.WaitReportUserBurst, "rate_limit.wait-report-user-burst", r.WaitReportUserBurst, "Per-user burst for wait-report.")
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
	if o.GRPCClient.MaxInflight <= 0 {
		errs = append(errs, fmt.Errorf("grpc_client.max_inflight must be greater than 0"))
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

	if o.SubmitQueue != nil && o.SubmitQueue.Enabled {
		if o.SubmitQueue.QueueSize <= 0 {
			errs = append(errs, fmt.Errorf("submit_queue.queue_size must be greater than 0"))
		}
		if o.SubmitQueue.WorkerCount <= 0 {
			errs = append(errs, fmt.Errorf("submit_queue.worker_count must be greater than 0"))
		}
		if o.SubmitQueue.WaitTimeoutMs < 0 {
			errs = append(errs, fmt.Errorf("submit_queue.wait_timeout_ms cannot be negative"))
		}
	}

	if o.RateLimit != nil && o.RateLimit.Enabled {
		if o.RateLimit.SubmitGlobalQPS <= 0 || o.RateLimit.SubmitGlobalBurst <= 0 {
			errs = append(errs, fmt.Errorf("rate_limit.submit_* must be greater than 0"))
		}
		if o.RateLimit.SubmitUserQPS <= 0 || o.RateLimit.SubmitUserBurst <= 0 {
			errs = append(errs, fmt.Errorf("rate_limit.submit_user_* must be greater than 0"))
		}
		if o.RateLimit.QueryGlobalQPS <= 0 || o.RateLimit.QueryGlobalBurst <= 0 {
			errs = append(errs, fmt.Errorf("rate_limit.query_* must be greater than 0"))
		}
		if o.RateLimit.QueryUserQPS <= 0 || o.RateLimit.QueryUserBurst <= 0 {
			errs = append(errs, fmt.Errorf("rate_limit.query_user_* must be greater than 0"))
		}
		if o.RateLimit.WaitReportGlobalQPS <= 0 || o.RateLimit.WaitReportGlobalBurst <= 0 {
			errs = append(errs, fmt.Errorf("rate_limit.wait_report_* must be greater than 0"))
		}
		if o.RateLimit.WaitReportUserQPS <= 0 || o.RateLimit.WaitReportUserBurst <= 0 {
			errs = append(errs, fmt.Errorf("rate_limit.wait_report_user_* must be greater than 0"))
		}
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
