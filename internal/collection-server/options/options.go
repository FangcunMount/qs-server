package options

import (
	"fmt"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/pkg/configmask"
	cliflag "github.com/FangcunMount/qs-server/pkg/flag"
	"github.com/spf13/pflag"
)

// Options 包含所有配置项
type Options struct {
	Log                     *log.Options                            `json:"log"      mapstructure:"log"`
	GenericServerRunOptions *genericoptions.ServerRunOptions        `json:"server"   mapstructure:"server"`
	InsecureServing         *genericoptions.InsecureServingOptions  `json:"insecure" mapstructure:"insecure"`
	SecureServing           *genericoptions.SecureServingOptions    `json:"secure"   mapstructure:"secure"`
	GRPCClient              *GRPCClientOptions                      `json:"grpc_client" mapstructure:"grpc_client"`
	RedisOptions            *genericoptions.RedisOptions            `json:"redis"     mapstructure:"redis"`
	RedisProfiles           map[string]*genericoptions.RedisOptions `json:"redis_profiles" mapstructure:"redis_profiles"`
	RedisRuntime            *genericoptions.RedisRuntimeOptions     `json:"redis_runtime" mapstructure:"redis_runtime"`
	Concurrency             *ConcurrencyOptions                     `json:"concurrency" mapstructure:"concurrency"`
	RateLimit               *RateLimitOptions                       `json:"rate_limit" mapstructure:"rate_limit"`
	WaitReport              *WaitReportOptions                      `json:"wait_report" mapstructure:"wait_report"`
	ReportEvents            *ReportEventsOptions                    `json:"report_events" mapstructure:"report_events"`
	ReportStatus            *genericoptions.ReportStatusOptions     `json:"report_status" mapstructure:"report_status"`
	Signaling               *genericoptions.SignalingOptions        `json:"signaling" mapstructure:"signaling"`
	SubmitQueue             *SubmitQueueOptions                     `json:"submit_queue" mapstructure:"submit_queue"`
	QuestionnaireCache      *QuestionnaireCacheOptions              `json:"questionnaire_cache" mapstructure:"questionnaire_cache"`
	ScaleCache              *ScaleCacheOptions                      `json:"scale_cache" mapstructure:"scale_cache"`
	PersonalityCache        *PersonalityCacheOptions                `json:"personality_cache" mapstructure:"personality_cache"`
	JWT                     *JWTOptions                             `json:"jwt" mapstructure:"jwt"`
	IAMOptions              *genericoptions.IAMOptions              `json:"iam" mapstructure:"iam"`
	Runtime                 *RuntimeOptions                         `json:"runtime" mapstructure:"runtime"`
}

// GRPCClientOptions GRPC 客户端配置
type GRPCClientOptions struct {
	Endpoint       string `json:"endpoint" mapstructure:"endpoint"`
	Timeout        int    `json:"timeout"  mapstructure:"timeout"`                  // 超时时间（秒）
	Insecure       bool   `json:"insecure" mapstructure:"insecure"`                 // 是否使用不安全连接
	MaxInflight    int    `json:"max_inflight" mapstructure:"max_inflight"`         // 最大并发调用数
	InflightWaitMs int    `json:"inflight_wait_ms" mapstructure:"inflight_wait_ms"` // inflight 槽位排队最长等待（毫秒），0 表示等到 RPC 超时

	// TLS 配置
	TLSCertFile   string `json:"tls_cert_file"   mapstructure:"tls-cert-file"`   // 客户端证书文件
	TLSKeyFile    string `json:"tls_key_file"    mapstructure:"tls-key-file"`    // 客户端密钥文件
	TLSCAFile     string `json:"tls_ca_file"     mapstructure:"tls-ca-file"`     // CA 证书文件
	TLSServerName string `json:"tls_server_name" mapstructure:"tls-server-name"` // 服务端名称（用于验证）
}

// ConcurrencyOptions 并发处理配置
type ConcurrencyOptions struct {
	MaxConcurrency       int `json:"max_concurrency" mapstructure:"max_concurrency"`               // 兼容：未配置 max_query_concurrency 时作为读池上限
	MaxQueryConcurrency  int `json:"max_query_concurrency" mapstructure:"max_query_concurrency"`   // catalog/report-status 与其余读路径
	MaxSubmitConcurrency int `json:"max_submit_concurrency" mapstructure:"max_submit_concurrency"` // 答卷提交等写路径
	MaxWaitMs            int `json:"max_wait_ms" mapstructure:"max_wait_ms"`                       // submit/非 catalog 读 槽位排队最长等待（毫秒），0 表示无限等待
}

// ResolvedQueryConcurrency 返回读路径并发槽位上限。
func (c *ConcurrencyOptions) ResolvedQueryConcurrency() int {
	if c == nil {
		return 0
	}
	if c.MaxQueryConcurrency > 0 {
		return c.MaxQueryConcurrency
	}
	return c.MaxConcurrency
}

// ResolvedSubmitConcurrency 返回写路径并发槽位上限。
func (c *ConcurrencyOptions) ResolvedSubmitConcurrency() int {
	if c == nil {
		return 0
	}
	if c.MaxSubmitConcurrency > 0 {
		return c.MaxSubmitConcurrency
	}
	query := c.ResolvedQueryConcurrency()
	if query > 0 {
		submit := query / 5
		if submit < 32 {
			submit = 32
		}
		return submit
	}
	return 32
}

// QuestionnaireCacheOptions 已发布问卷详情 BFF 进程内 L1 缓存。
type QuestionnaireCacheOptions struct {
	Enabled            bool    `json:"enabled" mapstructure:"enabled"`
	TTLSeconds         int     `json:"ttl_seconds" mapstructure:"ttl_seconds"`
	TTLJitterRatio     float64 `json:"ttl_jitter_ratio" mapstructure:"ttl_jitter_ratio"`
	MaxEntries         int     `json:"max_entries" mapstructure:"max_entries"`
	Singleflight       bool    `json:"singleflight" mapstructure:"singleflight"`
	SignalEvictEnabled bool    `json:"signal_evict_enabled" mapstructure:"signal_evict_enabled"`
}

// ScaleCacheOptions 量表目录 BFF 进程内 L1 缓存。
type ScaleCacheOptions struct {
	Enabled            bool    `json:"enabled" mapstructure:"enabled"`
	TTLSeconds         int     `json:"ttl_seconds" mapstructure:"ttl_seconds"`
	TTLJitterRatio     float64 `json:"ttl_jitter_ratio" mapstructure:"ttl_jitter_ratio"`
	MaxEntries         int     `json:"max_entries" mapstructure:"max_entries"`
	Singleflight       bool    `json:"singleflight" mapstructure:"singleflight"`
	SignalEvictEnabled bool    `json:"signal_evict_enabled" mapstructure:"signal_evict_enabled"`
}

// PersonalityCacheOptions 人格模型目录 BFF 进程内 L1 缓存。
type PersonalityCacheOptions struct {
	Enabled            bool    `json:"enabled" mapstructure:"enabled"`
	TTLSeconds         int     `json:"ttl_seconds" mapstructure:"ttl_seconds"`
	TTLJitterRatio     float64 `json:"ttl_jitter_ratio" mapstructure:"ttl_jitter_ratio"`
	MaxEntries         int     `json:"max_entries" mapstructure:"max_entries"`
	Singleflight       bool    `json:"singleflight" mapstructure:"singleflight"`
	SignalEvictEnabled bool    `json:"signal_evict_enabled" mapstructure:"signal_evict_enabled"`
}

// SubmitQueueOptions 提交排队配置
type SubmitQueueOptions struct {
	Enabled       bool `json:"enabled" mapstructure:"enabled"` // Deprecated: submit queue is always enabled.
	QueueSize     int  `json:"queue_size" mapstructure:"queue_size"`
	WorkerCount   int  `json:"worker_count" mapstructure:"worker_count"`
	WaitTimeoutMs int  `json:"wait_timeout_ms" mapstructure:"wait_timeout_ms"` // Deprecated: submit no longer waits synchronously for queue results.
}

// RateLimitOptions 限流配置
type RateLimitOptions struct {
	Enabled                 bool    `json:"enabled" mapstructure:"enabled"`
	SubmitGlobalQPS         float64 `json:"submit_global_qps" mapstructure:"submit_global_qps"`
	SubmitGlobalBurst       int     `json:"submit_global_burst" mapstructure:"submit_global_burst"`
	SubmitUserQPS           float64 `json:"submit_user_qps" mapstructure:"submit_user_qps"`
	SubmitUserBurst         int     `json:"submit_user_burst" mapstructure:"submit_user_burst"`
	QueryGlobalQPS          float64 `json:"query_global_qps" mapstructure:"query_global_qps"`
	QueryGlobalBurst        int     `json:"query_global_burst" mapstructure:"query_global_burst"`
	QueryUserQPS            float64 `json:"query_user_qps" mapstructure:"query_user_qps"`
	QueryUserBurst          int     `json:"query_user_burst" mapstructure:"query_user_burst"`
	WaitReportGlobalQPS     float64 `json:"wait_report_global_qps" mapstructure:"wait_report_global_qps"`
	WaitReportGlobalBurst   int     `json:"wait_report_global_burst" mapstructure:"wait_report_global_burst"`
	WaitReportUserQPS       float64 `json:"wait_report_user_qps" mapstructure:"wait_report_user_qps"`
	WaitReportUserBurst     int     `json:"wait_report_user_burst" mapstructure:"wait_report_user_burst"`
	ReportEventsGlobalQPS   float64 `json:"report_events_global_qps" mapstructure:"report_events_global_qps"`
	ReportEventsGlobalBurst int     `json:"report_events_global_burst" mapstructure:"report_events_global_burst"`
	ReportEventsUserQPS     float64 `json:"report_events_user_qps" mapstructure:"report_events_user_qps"`
	ReportEventsUserBurst   int     `json:"report_events_user_burst" mapstructure:"report_events_user_burst"`
}

type WaitReportOptions struct {
	DefaultTimeoutSeconds    int    `json:"default_timeout_seconds" mapstructure:"default_timeout_seconds"`
	MinTimeoutSeconds        int    `json:"min_timeout_seconds" mapstructure:"min_timeout_seconds"`
	MaxTimeoutSeconds        int    `json:"max_timeout_seconds" mapstructure:"max_timeout_seconds"`
	PollIntervalMs           int    `json:"poll_interval_ms" mapstructure:"poll_interval_ms"`
	StatusTTLSeconds         int    `json:"status_ttl_seconds" mapstructure:"status_ttl_seconds"`
	MaxActiveWaiters         int    `json:"max_active_waiters" mapstructure:"max_active_waiters"`
	MaxHTTPConcurrency       int    `json:"max_http_concurrency" mapstructure:"max_http_concurrency"`
	DegradeImmediateEnabled  bool   `json:"degrade_immediate_enabled" mapstructure:"degrade_immediate_enabled"`
	DegradeRetryAfterSeconds int    `json:"degrade_retry_after_seconds" mapstructure:"degrade_retry_after_seconds"`
	PubSubEnabled            bool   `json:"pubsub_enabled" mapstructure:"pubsub_enabled"`
	PubSubChannel            string `json:"pubsub_channel" mapstructure:"pubsub_channel"`
}

// ReportEventsOptions WebSocket 报告事件推送配置。
type ReportEventsOptions struct {
	Enabled                  bool   `json:"enabled" mapstructure:"enabled"`
	Path                     string `json:"path" mapstructure:"path"`
	MaxConnections           int    `json:"max_connections" mapstructure:"max_connections"`
	MaxPerTestee             int    `json:"max_per_testee" mapstructure:"max_per_testee"`
	IdleTimeoutSeconds       int    `json:"idle_timeout_seconds" mapstructure:"idle_timeout_seconds"`
	HeartbeatIntervalSeconds int    `json:"heartbeat_interval_seconds" mapstructure:"heartbeat_interval_seconds"`
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
		RedisOptions:  genericoptions.NewRedisOptions(),
		RedisProfiles: map[string]*genericoptions.RedisOptions{},
		RedisRuntime:  defaultRedisRuntimeOptions(),
		Concurrency: &ConcurrencyOptions{
			MaxConcurrency: 10, // 默认最大并发数
		},
		RateLimit:          NewRateLimitOptions(),
		WaitReport:         NewWaitReportOptions(),
		ReportEvents:       NewReportEventsOptions(),
		ReportStatus:       genericoptions.NewReportStatusOptions(),
		Signaling:          genericoptions.NewSignalingOptions(),
		SubmitQueue:        NewSubmitQueueOptions(),
		QuestionnaireCache: NewQuestionnaireCacheOptions(),
		ScaleCache:         NewScaleCacheOptions(),
		PersonalityCache:   NewPersonalityCacheOptions(),
		JWT: &JWTOptions{
			SecretKey:     "your-secret-key-change-in-production",
			TokenDuration: 24 * 7, // 7 天
		},
		IAMOptions: genericoptions.NewIAMOptions(),
		Runtime:    NewRuntimeOptions(),
	}
}

func defaultRedisRuntimeOptions() *genericoptions.RedisRuntimeOptions {
	opts := genericoptions.NewRedisRuntimeOptions()
	opts.Families["ops_runtime"] = &genericoptions.RedisRuntimeFamilyRoute{
		RedisProfile:         "ops_runtime",
		NamespaceSuffix:      "ops:runtime",
		AllowFallbackDefault: boolPtr(true),
	}
	opts.Families["lock_lease"] = &genericoptions.RedisRuntimeFamilyRoute{
		RedisProfile:         "lock_cache",
		NamespaceSuffix:      "cache:lock",
		AllowFallbackDefault: boolPtr(true),
	}
	return opts
}

func boolPtr(v bool) *bool {
	return &v
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
		WaitTimeoutMs: 0,
	}
}

// NewQuestionnaireCacheOptions 创建默认问卷详情 L1 缓存配置。
func NewQuestionnaireCacheOptions() *QuestionnaireCacheOptions {
	return &QuestionnaireCacheOptions{
		Enabled:            false,
		TTLSeconds:         180,
		MaxEntries:         256,
		Singleflight:       true,
		SignalEvictEnabled: true,
	}
}

func NewScaleCacheOptions() *ScaleCacheOptions {
	return &ScaleCacheOptions{
		Enabled:            false,
		TTLSeconds:         180,
		MaxEntries:         256,
		Singleflight:       true,
		SignalEvictEnabled: true,
	}
}

func NewPersonalityCacheOptions() *PersonalityCacheOptions {
	return &PersonalityCacheOptions{
		Enabled:            false,
		TTLSeconds:         180,
		MaxEntries:         256,
		Singleflight:       true,
		SignalEvictEnabled: true,
	}
}

// NewRateLimitOptions 创建默认限流配置
func NewRateLimitOptions() *RateLimitOptions {
	return &RateLimitOptions{
		Enabled:                 true,
		SubmitGlobalQPS:         200,
		SubmitGlobalBurst:       300,
		SubmitUserQPS:           5,
		SubmitUserBurst:         10,
		QueryGlobalQPS:          200,
		QueryGlobalBurst:        300,
		QueryUserQPS:            10,
		QueryUserBurst:          20,
		WaitReportGlobalQPS:     80,
		WaitReportGlobalBurst:   120,
		WaitReportUserQPS:       2,
		WaitReportUserBurst:     5,
		ReportEventsGlobalQPS:   100,
		ReportEventsGlobalBurst: 150,
		ReportEventsUserQPS:     10,
		ReportEventsUserBurst:   20,
	}
}

func NewReportEventsOptions() *ReportEventsOptions {
	return &ReportEventsOptions{
		Enabled:                  false,
		Path:                     "/api/v1/report-events",
		MaxConnections:           2000,
		MaxPerTestee:             2,
		IdleTimeoutSeconds:       120,
		HeartbeatIntervalSeconds: 30,
	}
}

func NewWaitReportOptions() *WaitReportOptions {
	return &WaitReportOptions{
		DefaultTimeoutSeconds:    20,
		MinTimeoutSeconds:        1,
		MaxTimeoutSeconds:        25,
		PollIntervalMs:           500,
		StatusTTLSeconds:         172800,
		MaxActiveWaiters:         3000,
		MaxHTTPConcurrency:       400,
		DegradeImmediateEnabled:  true,
		DegradeRetryAfterSeconds: 5,
		PubSubEnabled:            false,
		PubSubChannel:            "report_status_changed",
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
	o.RedisOptions.AddFlags(fss.FlagSet("redis"))
	o.RedisRuntime.AddFlags(fss.FlagSet("redis_runtime"))
	o.Concurrency.AddFlags(fss.FlagSet("concurrency"))
	o.RateLimit.AddFlags(fss.FlagSet("rate_limit"))
	o.WaitReport.AddFlags(fss.FlagSet("wait_report"))
	o.ReportEvents.AddFlags(fss.FlagSet("report_events"))
	o.SubmitQueue.AddFlags(fss.FlagSet("submit_queue"))
	o.QuestionnaireCache.AddFlags(fss.FlagSet("questionnaire_cache"))
	o.ScaleCache.AddFlags(fss.FlagSet("scale_cache"))
	o.PersonalityCache.AddFlags(fss.FlagSet("personality_cache"))
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
	fs.IntVar(&g.InflightWaitMs, "grpc_client.inflight-wait-ms", g.InflightWaitMs,
		"Maximum wait time in milliseconds when gRPC inflight slots are full (0 waits until RPC timeout).")
}

// AddFlags 添加并发处理相关的命令行参数
func (c *ConcurrencyOptions) AddFlags(fs *pflag.FlagSet) {
	fs.IntVar(&c.MaxConcurrency, "concurrency.max-concurrency", c.MaxConcurrency,
		"Deprecated: use max-query-concurrency; fallback when max-query-concurrency is unset.")
	fs.IntVar(&c.MaxQueryConcurrency, "concurrency.max-query-concurrency", c.MaxQueryConcurrency,
		"Maximum concurrent HTTP handlers for catalog/report-status and other read paths.")
	fs.IntVar(&c.MaxSubmitConcurrency, "concurrency.max-submit-concurrency", c.MaxSubmitConcurrency,
		"Maximum concurrent HTTP handlers for submit/write paths.")
	fs.IntVar(&c.MaxWaitMs, "concurrency.max-wait-ms", c.MaxWaitMs,
		"Maximum wait in milliseconds for submit/non-catalog read slots before returning 503 (0 means block).")
}

// AddFlags 添加提交排队相关的命令行参数
func (s *SubmitQueueOptions) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&s.Enabled, "submit_queue.enabled", s.Enabled, "Deprecated: submit queue is always enabled.")
	fs.IntVar(&s.QueueSize, "submit_queue.queue-size", s.QueueSize, "Submit queue size.")
	fs.IntVar(&s.WorkerCount, "submit_queue.worker-count", s.WorkerCount, "Submit queue worker count.")
	fs.IntVar(&s.WaitTimeoutMs, "submit_queue.wait-timeout-ms", s.WaitTimeoutMs, "Deprecated: submit no longer waits synchronously for queue results.")
}

func (q *QuestionnaireCacheOptions) AddFlags(fs *pflag.FlagSet) {
	if q == nil {
		return
	}
	fs.BoolVar(&q.Enabled, "questionnaire_cache.enabled", q.Enabled, "Enable in-process L1 cache for published questionnaire detail.")
	fs.IntVar(&q.TTLSeconds, "questionnaire_cache.ttl-seconds", q.TTLSeconds, "TTL for questionnaire detail L1 cache in seconds.")
	fs.Float64Var(&q.TTLJitterRatio, "questionnaire_cache.ttl-jitter-ratio", q.TTLJitterRatio, "TTL jitter ratio (0-1) for questionnaire detail L1 cache.")
	fs.IntVar(&q.MaxEntries, "questionnaire_cache.max-entries", q.MaxEntries, "Maximum questionnaire detail entries in L1 cache.")
	fs.BoolVar(&q.Singleflight, "questionnaire_cache.singleflight", q.Singleflight, "Coalesce concurrent questionnaire detail cache misses.")
	fs.BoolVar(&q.SignalEvictEnabled, "questionnaire_cache.signal-evict-enabled", q.SignalEvictEnabled, "Subscribe questionnaire_cache_changed Redis signal to evict L1 entries.")
}

func (s *ScaleCacheOptions) AddFlags(fs *pflag.FlagSet) {
	if s == nil {
		return
	}
	fs.BoolVar(&s.Enabled, "scale_cache.enabled", s.Enabled, "Enable in-process L1 cache for scale catalog reads.")
	fs.IntVar(&s.TTLSeconds, "scale_cache.ttl-seconds", s.TTLSeconds, "TTL for scale catalog L1 cache in seconds.")
	fs.Float64Var(&s.TTLJitterRatio, "scale_cache.ttl-jitter-ratio", s.TTLJitterRatio, "TTL jitter ratio (0-1) for scale catalog L1 cache.")
	fs.IntVar(&s.MaxEntries, "scale_cache.max-entries", s.MaxEntries, "Maximum scale catalog entries in L1 cache.")
	fs.BoolVar(&s.Singleflight, "scale_cache.singleflight", s.Singleflight, "Coalesce concurrent scale catalog cache misses.")
	fs.BoolVar(&s.SignalEvictEnabled, "scale_cache.signal-evict-enabled", s.SignalEvictEnabled, "Subscribe scale_cache_changed Redis signal to evict L1 entries.")
}

func (p *PersonalityCacheOptions) AddFlags(fs *pflag.FlagSet) {
	if p == nil {
		return
	}
	fs.BoolVar(&p.Enabled, "personality_cache.enabled", p.Enabled, "Enable in-process L1 cache for personality model catalog reads.")
	fs.IntVar(&p.TTLSeconds, "personality_cache.ttl-seconds", p.TTLSeconds, "TTL for personality model catalog L1 cache in seconds.")
	fs.Float64Var(&p.TTLJitterRatio, "personality_cache.ttl-jitter-ratio", p.TTLJitterRatio, "TTL jitter ratio (0-1) for personality model catalog L1 cache.")
	fs.IntVar(&p.MaxEntries, "personality_cache.max-entries", p.MaxEntries, "Maximum personality model catalog entries in L1 cache.")
	fs.BoolVar(&p.Singleflight, "personality_cache.singleflight", p.Singleflight, "Coalesce concurrent personality model catalog cache misses.")
	fs.BoolVar(&p.SignalEvictEnabled, "personality_cache.signal-evict-enabled", p.SignalEvictEnabled, "Subscribe personality_model_cache_changed Redis signal to evict L1 entries.")
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
	fs.Float64Var(&r.ReportEventsGlobalQPS, "rate_limit.report-events-global-qps", r.ReportEventsGlobalQPS, "Global QPS limit for report-events WebSocket subscribe.")
	fs.IntVar(&r.ReportEventsGlobalBurst, "rate_limit.report-events-global-burst", r.ReportEventsGlobalBurst, "Global burst for report-events WebSocket subscribe.")
	fs.Float64Var(&r.ReportEventsUserQPS, "rate_limit.report-events-user-qps", r.ReportEventsUserQPS, "Per-user QPS limit for report-events WebSocket subscribe.")
	fs.IntVar(&r.ReportEventsUserBurst, "rate_limit.report-events-user-burst", r.ReportEventsUserBurst, "Per-user burst for report-events WebSocket subscribe.")
}

func (w *WaitReportOptions) AddFlags(fs *pflag.FlagSet) {
	fs.IntVar(&w.DefaultTimeoutSeconds, "wait_report.default-timeout-seconds", w.DefaultTimeoutSeconds, "Default wait-report timeout seconds.")
	fs.IntVar(&w.MinTimeoutSeconds, "wait_report.min-timeout-seconds", w.MinTimeoutSeconds, "Minimum wait-report timeout seconds.")
	fs.IntVar(&w.MaxTimeoutSeconds, "wait_report.max-timeout-seconds", w.MaxTimeoutSeconds, "Maximum wait-report timeout seconds.")
	fs.IntVar(&w.PollIntervalMs, "wait_report.poll-interval-ms", w.PollIntervalMs, "Wait-report polling interval in milliseconds.")
	fs.IntVar(&w.StatusTTLSeconds, "wait_report.status-ttl-seconds", w.StatusTTLSeconds, "Report status cache TTL in seconds.")
	fs.IntVar(&w.MaxActiveWaiters, "wait_report.max-active-waiters", w.MaxActiveWaiters, "Maximum active wait-report requests before degradation.")
	fs.IntVar(&w.MaxHTTPConcurrency, "wait_report.max-http-concurrency", w.MaxHTTPConcurrency, "Maximum concurrent HTTP handlers for wait-report.")
	fs.BoolVar(&w.DegradeImmediateEnabled, "wait_report.degrade-immediate-enabled", w.DegradeImmediateEnabled, "Return pending immediately when wait-report HTTP slots are exhausted.")
	fs.IntVar(&w.DegradeRetryAfterSeconds, "wait_report.degrade-retry-after-seconds", w.DegradeRetryAfterSeconds, "Retry-After seconds for degraded wait-report responses.")
	fs.BoolVar(&w.PubSubEnabled, "wait_report.pubsub-enabled", w.PubSubEnabled, "Enable Redis pubsub wakeups for wait-report.")
	fs.StringVar(&w.PubSubChannel, "wait_report.pubsub-channel", w.PubSubChannel, "Redis pubsub channel for report status change.")
}

func (r *ReportEventsOptions) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&r.Enabled, "report_events.enabled", r.Enabled, "Enable WebSocket report-events endpoint.")
	fs.StringVar(&r.Path, "report_events.path", r.Path, "WebSocket report-events HTTP path.")
	fs.IntVar(&r.MaxConnections, "report_events.max-connections", r.MaxConnections, "Maximum concurrent WebSocket connections.")
	fs.IntVar(&r.MaxPerTestee, "report_events.max-per-testee", r.MaxPerTestee, "Maximum concurrent WebSocket connections per testee.")
	fs.IntVar(&r.IdleTimeoutSeconds, "report_events.idle-timeout-seconds", r.IdleTimeoutSeconds, "Idle timeout seconds before closing WebSocket connections.")
	fs.IntVar(&r.HeartbeatIntervalSeconds, "report_events.heartbeat-interval-seconds", r.HeartbeatIntervalSeconds, "Server heartbeat interval seconds for WebSocket connections.")
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
	return configmask.String(o)
}

// Validate 验证配置选项
func (o *Options) Validate() []error {
	var errs []error

	errs = append(errs, o.GenericServerRunOptions.Validate()...)
	errs = append(errs, o.Log.Validate()...)
	errs = append(errs, validateCollectionGRPCClient(o.GRPCClient)...)
	errs = append(errs, validateCollectionRedis(o.RedisOptions, o.RedisRuntime, o.RedisProfiles)...)
	errs = append(errs, validateCollectionConcurrency(o.Concurrency)...)
	errs = append(errs, validateCollectionSubmitQueue(o.SubmitQueue)...)
	errs = append(errs, validateQuestionnaireCacheOptions(o.QuestionnaireCache)...)
	errs = append(errs, validateScaleCacheOptions(o.ScaleCache)...)
	errs = append(errs, validatePersonalityCacheOptions(o.PersonalityCache)...)
	errs = append(errs, validateCollectionRateLimit(o.RateLimit)...)
	errs = append(errs, validateWaitReportOptions(o.WaitReport)...)
	errs = append(errs, validateReportEventsOptions(o.ReportEvents)...)
	errs = append(errs, validateCollectionJWT(o.JWT)...)

	return errs
}

func validateCollectionGRPCClient(opts *GRPCClientOptions) []error {
	if opts == nil {
		return []error{fmt.Errorf("grpc_client cannot be nil")}
	}

	var errs []error
	if opts.Endpoint == "" {
		errs = append(errs, fmt.Errorf("grpc_client.endpoint cannot be empty"))
	}
	if opts.Timeout <= 0 {
		errs = append(errs, fmt.Errorf("grpc_client.timeout must be greater than 0"))
	}
	if opts.MaxInflight <= 0 {
		errs = append(errs, fmt.Errorf("grpc_client.max_inflight must be greater than 0"))
	}
	return errs
}

func validateCollectionRedis(
	redisOpts *genericoptions.RedisOptions,
	runtimeOpts *genericoptions.RedisRuntimeOptions,
	profiles map[string]*genericoptions.RedisOptions,
) []error {
	if redisOpts == nil {
		return []error{fmt.Errorf("redis cannot be nil")}
	}

	var errs []error
	if redisOpts.Host == "" && len(redisOpts.Addrs) == 0 {
		errs = append(errs, fmt.Errorf("redis.host cannot be empty"))
	}
	if len(redisOpts.Addrs) == 0 && redisOpts.Port <= 0 {
		errs = append(errs, fmt.Errorf("redis.port must be greater than 0 when addrs not provided"))
	}
	errs = append(errs, cacheplane.ValidateRuntimeOptions(
		runtimeOpts,
		[]cacheplane.Family{cacheplane.FamilyOps, cacheplane.FamilyLock},
		profiles,
		"redis_runtime",
	)...)
	return errs
}

func validateCollectionConcurrency(opts *ConcurrencyOptions) []error {
	if opts == nil {
		return []error{fmt.Errorf("concurrency cannot be nil")}
	}

	var errs []error
	maxQuery := opts.ResolvedQueryConcurrency()
	if maxQuery <= 0 {
		errs = append(errs, fmt.Errorf("concurrency.max-query-concurrency (or max-concurrency) must be greater than 0"))
	}
	if maxQuery > 512 {
		errs = append(errs, fmt.Errorf("concurrency.max-query-concurrency cannot be greater than 512"))
	}
	maxSubmit := opts.ResolvedSubmitConcurrency()
	if maxSubmit <= 0 {
		errs = append(errs, fmt.Errorf("concurrency.max-submit-concurrency must be greater than 0"))
	}
	if maxSubmit > 512 {
		errs = append(errs, fmt.Errorf("concurrency.max-submit-concurrency cannot be greater than 512"))
	}
	return errs
}

func validateCollectionSubmitQueue(opts *SubmitQueueOptions) []error {
	if opts == nil {
		return nil
	}

	var errs []error
	if opts.QueueSize <= 0 {
		errs = append(errs, fmt.Errorf("submit_queue.queue_size must be greater than 0"))
	}
	if opts.WorkerCount <= 0 {
		errs = append(errs, fmt.Errorf("submit_queue.worker_count must be greater than 0"))
	}
	if opts.WaitTimeoutMs < 0 {
		errs = append(errs, fmt.Errorf("submit_queue.wait_timeout_ms cannot be negative"))
	}
	return errs
}

func validateCollectionRateLimit(opts *RateLimitOptions) []error {
	if opts == nil || !opts.Enabled {
		return nil
	}

	var errs []error
	if opts.SubmitGlobalQPS <= 0 || opts.SubmitGlobalBurst <= 0 {
		errs = append(errs, fmt.Errorf("rate_limit.submit_* must be greater than 0"))
	}
	if opts.SubmitUserQPS <= 0 || opts.SubmitUserBurst <= 0 {
		errs = append(errs, fmt.Errorf("rate_limit.submit_user_* must be greater than 0"))
	}
	if opts.QueryGlobalQPS <= 0 || opts.QueryGlobalBurst <= 0 {
		errs = append(errs, fmt.Errorf("rate_limit.query_* must be greater than 0"))
	}
	if opts.QueryUserQPS <= 0 || opts.QueryUserBurst <= 0 {
		errs = append(errs, fmt.Errorf("rate_limit.query_user_* must be greater than 0"))
	}
	if opts.WaitReportGlobalQPS <= 0 || opts.WaitReportGlobalBurst <= 0 {
		errs = append(errs, fmt.Errorf("rate_limit.wait_report_* must be greater than 0"))
	}
	if opts.WaitReportUserQPS <= 0 || opts.WaitReportUserBurst <= 0 {
		errs = append(errs, fmt.Errorf("rate_limit.wait_report_user_* must be greater than 0"))
	}
	if opts.ReportEventsGlobalQPS <= 0 || opts.ReportEventsGlobalBurst <= 0 {
		errs = append(errs, fmt.Errorf("rate_limit.report_events_* must be greater than 0"))
	}
	if opts.ReportEventsUserQPS <= 0 || opts.ReportEventsUserBurst <= 0 {
		errs = append(errs, fmt.Errorf("rate_limit.report_events_user_* must be greater than 0"))
	}
	return errs
}

func validateCollectionJWT(opts *JWTOptions) []error {
	if opts == nil {
		return []error{fmt.Errorf("jwt cannot be nil")}
	}

	var errs []error
	if opts.SecretKey == "" {
		errs = append(errs, fmt.Errorf("jwt.secret-key cannot be empty"))
	}
	if opts.TokenDuration <= 0 {
		errs = append(errs, fmt.Errorf("jwt.token-duration must be greater than 0"))
	}
	return errs
}

func validateWaitReportOptions(opts *WaitReportOptions) []error {
	if opts == nil {
		return []error{fmt.Errorf("wait_report cannot be nil")}
	}
	var errs []error
	if opts.DefaultTimeoutSeconds <= 0 {
		errs = append(errs, fmt.Errorf("wait_report.default_timeout_seconds must be greater than 0"))
	}
	if opts.MinTimeoutSeconds <= 0 {
		errs = append(errs, fmt.Errorf("wait_report.min_timeout_seconds must be greater than 0"))
	}
	if opts.MaxTimeoutSeconds < opts.MinTimeoutSeconds {
		errs = append(errs, fmt.Errorf("wait_report.max_timeout_seconds must be greater than or equal to min_timeout_seconds"))
	}
	if opts.PollIntervalMs <= 0 {
		errs = append(errs, fmt.Errorf("wait_report.poll_interval_ms must be greater than 0"))
	}
	if opts.StatusTTLSeconds <= 0 {
		errs = append(errs, fmt.Errorf("wait_report.status_ttl_seconds must be greater than 0"))
	}
	if opts.MaxActiveWaiters <= 0 {
		errs = append(errs, fmt.Errorf("wait_report.max_active_waiters must be greater than 0"))
	}
	if opts.MaxHTTPConcurrency <= 0 {
		errs = append(errs, fmt.Errorf("wait_report.max_http_concurrency must be greater than 0"))
	}
	if opts.DegradeRetryAfterSeconds <= 0 {
		errs = append(errs, fmt.Errorf("wait_report.degrade_retry_after_seconds must be greater than 0"))
	}
	if opts.PubSubEnabled && opts.PubSubChannel == "" {
		errs = append(errs, fmt.Errorf("wait_report.pubsub_channel cannot be empty when pubsub is enabled"))
	}
	return errs
}

func validateReportEventsOptions(opts *ReportEventsOptions) []error {
	if opts == nil {
		return []error{fmt.Errorf("report_events cannot be nil")}
	}
	var errs []error
	if opts.Path == "" {
		errs = append(errs, fmt.Errorf("report_events.path cannot be empty"))
	}
	if opts.MaxConnections <= 0 {
		errs = append(errs, fmt.Errorf("report_events.max_connections must be greater than 0"))
	}
	if opts.MaxPerTestee <= 0 {
		errs = append(errs, fmt.Errorf("report_events.max_per_testee must be greater than 0"))
	}
	if opts.IdleTimeoutSeconds <= 0 {
		errs = append(errs, fmt.Errorf("report_events.idle_timeout_seconds must be greater than 0"))
	}
	if opts.HeartbeatIntervalSeconds <= 0 {
		errs = append(errs, fmt.Errorf("report_events.heartbeat_interval_seconds must be greater than 0"))
	}
	return errs
}

func validateQuestionnaireCacheOptions(opts *QuestionnaireCacheOptions) []error {
	if opts == nil {
		return nil
	}
	if !opts.Enabled {
		return nil
	}

	var errs []error
	if opts.TTLSeconds <= 0 {
		errs = append(errs, fmt.Errorf("questionnaire_cache.ttl_seconds must be greater than 0 when enabled"))
	}
	if opts.MaxEntries <= 0 {
		errs = append(errs, fmt.Errorf("questionnaire_cache.max_entries must be greater than 0 when enabled"))
	}
	if opts.TTLJitterRatio < 0 || opts.TTLJitterRatio > 1 {
		errs = append(errs, fmt.Errorf("questionnaire_cache.ttl_jitter_ratio must be between 0 and 1"))
	}
	return errs
}

func validateScaleCacheOptions(opts *ScaleCacheOptions) []error {
	if opts == nil {
		return nil
	}
	if !opts.Enabled {
		return nil
	}

	var errs []error
	if opts.TTLSeconds <= 0 {
		errs = append(errs, fmt.Errorf("scale_cache.ttl_seconds must be greater than 0 when enabled"))
	}
	if opts.MaxEntries <= 0 {
		errs = append(errs, fmt.Errorf("scale_cache.max_entries must be greater than 0 when enabled"))
	}
	if opts.TTLJitterRatio < 0 || opts.TTLJitterRatio > 1 {
		errs = append(errs, fmt.Errorf("scale_cache.ttl_jitter_ratio must be between 0 and 1"))
	}
	return errs
}

func validatePersonalityCacheOptions(opts *PersonalityCacheOptions) []error {
	if opts == nil {
		return nil
	}
	if !opts.Enabled {
		return nil
	}

	var errs []error
	if opts.TTLSeconds <= 0 {
		errs = append(errs, fmt.Errorf("personality_cache.ttl_seconds must be greater than 0 when enabled"))
	}
	if opts.MaxEntries <= 0 {
		errs = append(errs, fmt.Errorf("personality_cache.max_entries must be greater than 0 when enabled"))
	}
	if opts.TTLJitterRatio < 0 || opts.TTLJitterRatio > 1 {
		errs = append(errs, fmt.Errorf("personality_cache.ttl_jitter_ratio must be between 0 and 1"))
	}
	return errs
}
