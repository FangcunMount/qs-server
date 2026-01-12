package options

import (
	"encoding/json"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	cliflag "github.com/FangcunMount/qs-server/pkg/flag"
	"github.com/spf13/pflag"
)

// Options 包含所有配置项
type Options struct {
	Log                     *log.Options                           `json:"log"       mapstructure:"log"`
	GenericServerRunOptions *genericoptions.ServerRunOptions       `json:"server"    mapstructure:"server"`
	GRPCOptions             *genericoptions.GRPCOptions            `json:"grpc"      mapstructure:"grpc"`
	InsecureServing         *genericoptions.InsecureServingOptions `json:"insecure"  mapstructure:"insecure"`
	SecureServing           *genericoptions.SecureServingOptions   `json:"secure"    mapstructure:"secure"`
	MySQLOptions            *genericoptions.MySQLOptions           `json:"mysql"     mapstructure:"mysql"`
	MigrationOptions        *genericoptions.MigrationOptions       `json:"migration" mapstructure:"migration"`
	RedisDualOptions        *genericoptions.RedisDualOptions       `json:"redis"     mapstructure:"redis"`
	MongoDBOptions          *genericoptions.MongoDBOptions         `json:"mongodb"   mapstructure:"mongodb"`
	MessagingOptions        *genericoptions.MessagingOptions       `json:"messaging" mapstructure:"messaging"`
	IAMOptions              *genericoptions.IAMOptions             `json:"iam"       mapstructure:"iam"`
	WeChatOptions           *genericoptions.WeChatOptions          `json:"wechat"    mapstructure:"wechat"`
	RateLimit               *RateLimitOptions                      `json:"rate_limit" mapstructure:"rate_limit"`
	Backpressure            *BackpressureOptions                   `json:"backpressure" mapstructure:"backpressure"`
	Cache                   *CacheOptions                          `json:"cache"     mapstructure:"cache"`
	StatisticsSync          *StatisticsSyncOptions                 `json:"statistics_sync" mapstructure:"statistics_sync"`
}

// NewOptions 创建一个 Options 对象，包含默认参数
func NewOptions() *Options {
	return &Options{
		Log:                     log.NewOptions(),
		GenericServerRunOptions: genericoptions.NewServerRunOptions(),
		GRPCOptions:             genericoptions.NewGRPCOptions(),
		InsecureServing:         genericoptions.NewInsecureServingOptions(),
		SecureServing:           genericoptions.NewSecureServingOptions(),
		MySQLOptions:            genericoptions.NewMySQLOptions(),
		MigrationOptions:        genericoptions.NewMigrationOptions(),
		RedisDualOptions:        genericoptions.NewRedisDualOptions(),
		MongoDBOptions:          genericoptions.NewMongoDBOptions(),
		MessagingOptions:        genericoptions.NewMessagingOptions(),
		IAMOptions:              genericoptions.NewIAMOptions(),
		WeChatOptions:           genericoptions.NewWeChatOptions(),
		RateLimit:               NewRateLimitOptions(),
		Backpressure:            NewBackpressureOptions(),
		Cache:                   NewCacheOptions(),
		StatisticsSync:          NewStatisticsSyncOptions(),
	}
}

// BackpressureOptions 下游背压配置
type BackpressureOptions struct {
	MySQL *DependencyBackpressure `json:"mysql" mapstructure:"mysql"`
	Mongo *DependencyBackpressure `json:"mongo" mapstructure:"mongo"`
	IAM   *DependencyBackpressure `json:"iam" mapstructure:"iam"`
}

// DependencyBackpressure 单个依赖的背压配置
type DependencyBackpressure struct {
	Enabled     bool `json:"enabled" mapstructure:"enabled"`
	MaxInflight int  `json:"max_inflight" mapstructure:"max_inflight"`
	TimeoutMs   int  `json:"timeout_ms" mapstructure:"timeout_ms"`
}

// NewBackpressureOptions 创建默认背压配置
func NewBackpressureOptions() *BackpressureOptions {
	return &BackpressureOptions{
		MySQL: &DependencyBackpressure{
			Enabled:     true,
			MaxInflight: 200,
			TimeoutMs:   2000,
		},
		Mongo: &DependencyBackpressure{
			Enabled:     true,
			MaxInflight: 200,
			TimeoutMs:   2000,
		},
		IAM: &DependencyBackpressure{
			Enabled:     true,
			MaxInflight: 100,
			TimeoutMs:   2000,
		},
	}
}

// RateLimitOptions 限流配置
type RateLimitOptions struct {
	Enabled                bool    `json:"enabled" mapstructure:"enabled"`
	SubmitGlobalQPS        float64 `json:"submit_global_qps" mapstructure:"submit_global_qps"`
	SubmitGlobalBurst      int     `json:"submit_global_burst" mapstructure:"submit_global_burst"`
	SubmitUserQPS          float64 `json:"submit_user_qps" mapstructure:"submit_user_qps"`
	SubmitUserBurst        int     `json:"submit_user_burst" mapstructure:"submit_user_burst"`
	AdminSubmitGlobalQPS   float64 `json:"admin_submit_global_qps" mapstructure:"admin_submit_global_qps"`
	AdminSubmitGlobalBurst int     `json:"admin_submit_global_burst" mapstructure:"admin_submit_global_burst"`
	AdminSubmitUserQPS     float64 `json:"admin_submit_user_qps" mapstructure:"admin_submit_user_qps"`
	AdminSubmitUserBurst   int     `json:"admin_submit_user_burst" mapstructure:"admin_submit_user_burst"`
	QueryGlobalQPS         float64 `json:"query_global_qps" mapstructure:"query_global_qps"`
	QueryGlobalBurst       int     `json:"query_global_burst" mapstructure:"query_global_burst"`
	QueryUserQPS           float64 `json:"query_user_qps" mapstructure:"query_user_qps"`
	QueryUserBurst         int     `json:"query_user_burst" mapstructure:"query_user_burst"`
	WaitReportGlobalQPS    float64 `json:"wait_report_global_qps" mapstructure:"wait_report_global_qps"`
	WaitReportGlobalBurst  int     `json:"wait_report_global_burst" mapstructure:"wait_report_global_burst"`
	WaitReportUserQPS      float64 `json:"wait_report_user_qps" mapstructure:"wait_report_user_qps"`
	WaitReportUserBurst    int     `json:"wait_report_user_burst" mapstructure:"wait_report_user_burst"`
}

// NewRateLimitOptions 创建默认限流配置
func NewRateLimitOptions() *RateLimitOptions {
	return &RateLimitOptions{
		Enabled:                true,
		SubmitGlobalQPS:        200,
		SubmitGlobalBurst:      300,
		SubmitUserQPS:          5,
		SubmitUserBurst:        10,
		AdminSubmitGlobalQPS:   400,
		AdminSubmitGlobalBurst: 600,
		AdminSubmitUserQPS:     20,
		AdminSubmitUserBurst:   40,
		QueryGlobalQPS:         200,
		QueryGlobalBurst:       300,
		QueryUserQPS:           10,
		QueryUserBurst:         20,
		WaitReportGlobalQPS:    80,
		WaitReportGlobalBurst:  120,
		WaitReportUserQPS:      2,
		WaitReportUserBurst:    5,
	}
}

// Flags 返回一个 NamedFlagSets 对象，包含所有命令行参数
func (o *Options) Flags() (fss cliflag.NamedFlagSets) {
	o.Log.AddFlags(fss.FlagSet("log"))
	o.GenericServerRunOptions.AddFlags(fss.FlagSet("server"))
	o.GRPCOptions.AddFlags(fss.FlagSet("grpc"))
	o.InsecureServing.AddFlags(fss.FlagSet("insecure"))
	o.SecureServing.AddFlags(fss.FlagSet("secure"))
	o.MySQLOptions.AddFlags(fss.FlagSet("mysql"))
	o.MigrationOptions.AddFlags(fss.FlagSet("migration"))
	o.RedisDualOptions.AddFlags(fss.FlagSet("redis"))
	o.MongoDBOptions.AddFlags(fss.FlagSet("mongodb"))
	o.MessagingOptions.AddFlags(fss.FlagSet("messaging"))
	o.IAMOptions.AddFlags(fss.FlagSet("iam"))
	o.WeChatOptions.AddFlags(fss.FlagSet("wechat"))
	o.RateLimit.AddFlags(fss.FlagSet("rate_limit"))
	o.Backpressure.AddFlags(fss.FlagSet("backpressure"))
	o.Cache.AddFlags(fss.FlagSet("cache"))
	o.StatisticsSync.AddFlags(fss.FlagSet("statistics_sync"))

	return fss
}

// AddFlags 添加限流相关的命令行参数
func (r *RateLimitOptions) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&r.Enabled, "rate_limit.enabled", r.Enabled, "Enable rate limiting.")
	fs.Float64Var(&r.SubmitGlobalQPS, "rate_limit.submit-global-qps", r.SubmitGlobalQPS, "Global QPS limit for submit.")
	fs.IntVar(&r.SubmitGlobalBurst, "rate_limit.submit-global-burst", r.SubmitGlobalBurst, "Global burst for submit.")
	fs.Float64Var(&r.SubmitUserQPS, "rate_limit.submit-user-qps", r.SubmitUserQPS, "Per-user QPS limit for submit.")
	fs.IntVar(&r.SubmitUserBurst, "rate_limit.submit-user-burst", r.SubmitUserBurst, "Per-user burst for submit.")
	fs.Float64Var(&r.AdminSubmitGlobalQPS, "rate_limit.admin-submit-global-qps", r.AdminSubmitGlobalQPS, "Global QPS limit for admin submit.")
	fs.IntVar(&r.AdminSubmitGlobalBurst, "rate_limit.admin-submit-global-burst", r.AdminSubmitGlobalBurst, "Global burst for admin submit.")
	fs.Float64Var(&r.AdminSubmitUserQPS, "rate_limit.admin-submit-user-qps", r.AdminSubmitUserQPS, "Per-user QPS limit for admin submit.")
	fs.IntVar(&r.AdminSubmitUserBurst, "rate_limit.admin-submit-user-burst", r.AdminSubmitUserBurst, "Per-user burst for admin submit.")
	fs.Float64Var(&r.QueryGlobalQPS, "rate_limit.query-global-qps", r.QueryGlobalQPS, "Global QPS limit for queries.")
	fs.IntVar(&r.QueryGlobalBurst, "rate_limit.query-global-burst", r.QueryGlobalBurst, "Global burst for queries.")
	fs.Float64Var(&r.QueryUserQPS, "rate_limit.query-user-qps", r.QueryUserQPS, "Per-user QPS limit for queries.")
	fs.IntVar(&r.QueryUserBurst, "rate_limit.query-user-burst", r.QueryUserBurst, "Per-user burst for queries.")
	fs.Float64Var(&r.WaitReportGlobalQPS, "rate_limit.wait-report-global-qps", r.WaitReportGlobalQPS, "Global QPS limit for wait-report.")
	fs.IntVar(&r.WaitReportGlobalBurst, "rate_limit.wait-report-global-burst", r.WaitReportGlobalBurst, "Global burst for wait-report.")
	fs.Float64Var(&r.WaitReportUserQPS, "rate_limit.wait-report-user-qps", r.WaitReportUserQPS, "Per-user QPS limit for wait-report.")
	fs.IntVar(&r.WaitReportUserBurst, "rate_limit.wait-report-user-burst", r.WaitReportUserBurst, "Per-user burst for wait-report.")
}

// AddFlags 添加背压相关的命令行参数
func (b *BackpressureOptions) AddFlags(fs *pflag.FlagSet) {
	if b.MySQL == nil {
		b.MySQL = &DependencyBackpressure{}
	}
	if b.Mongo == nil {
		b.Mongo = &DependencyBackpressure{}
	}
	if b.IAM == nil {
		b.IAM = &DependencyBackpressure{}
	}
	fs.BoolVar(&b.MySQL.Enabled, "backpressure.mysql.enabled", b.MySQL.Enabled, "Enable MySQL backpressure.")
	fs.IntVar(&b.MySQL.MaxInflight, "backpressure.mysql.max-inflight", b.MySQL.MaxInflight, "Max inflight MySQL operations.")
	fs.IntVar(&b.MySQL.TimeoutMs, "backpressure.mysql.timeout-ms", b.MySQL.TimeoutMs, "MySQL backpressure timeout in ms.")

	fs.BoolVar(&b.Mongo.Enabled, "backpressure.mongo.enabled", b.Mongo.Enabled, "Enable Mongo backpressure.")
	fs.IntVar(&b.Mongo.MaxInflight, "backpressure.mongo.max-inflight", b.Mongo.MaxInflight, "Max inflight Mongo operations.")
	fs.IntVar(&b.Mongo.TimeoutMs, "backpressure.mongo.timeout-ms", b.Mongo.TimeoutMs, "Mongo backpressure timeout in ms.")

	fs.BoolVar(&b.IAM.Enabled, "backpressure.iam.enabled", b.IAM.Enabled, "Enable IAM backpressure.")
	fs.IntVar(&b.IAM.MaxInflight, "backpressure.iam.max-inflight", b.IAM.MaxInflight, "Max inflight IAM calls.")
	fs.IntVar(&b.IAM.TimeoutMs, "backpressure.iam.timeout-ms", b.IAM.TimeoutMs, "IAM backpressure timeout in ms.")
}

// CacheOptions 缓存控制配置
type CacheOptions struct {
	DisableEvaluationCache bool                     `json:"disable_evaluation_cache" mapstructure:"disable_evaluation_cache"`
	DisableStatisticsCache bool                     `json:"disable_statistics_cache" mapstructure:"disable_statistics_cache"`
	TTL                    *CacheTTLOptions         `json:"ttl" mapstructure:"ttl"`
	TTLJitterRatio         float64                  `json:"ttl_jitter_ratio" mapstructure:"ttl_jitter_ratio"`
	StatisticsWarmup       *StatisticsWarmupOptions `json:"statistics_warmup" mapstructure:"statistics_warmup"`
	Namespace              string                   `json:"namespace" mapstructure:"namespace"`
	CompressPayload        bool                     `json:"compress_payload" mapstructure:"compress_payload"`
}

// NewCacheOptions 创建默认缓存配置
func NewCacheOptions() *CacheOptions {
	return &CacheOptions{
		DisableEvaluationCache: false,
		DisableStatisticsCache: false,
		TTL: &CacheTTLOptions{
			Scale:            24 * time.Hour,
			Questionnaire:    12 * time.Hour,
			AssessmentDetail: 2 * time.Hour,
			AssessmentStatus: 30 * time.Minute,
			Testee:           2 * time.Hour,
			Plan:             2 * time.Hour,
			Negative:         5 * time.Minute,
		},
		TTLJitterRatio:  0.1,
		Namespace:       "",
		CompressPayload: false,
		StatisticsWarmup: &StatisticsWarmupOptions{
			Enable:             false,
			OrgIDs:             []int64{1},
			QuestionnaireCodes: []string{},
			PlanIDs:            []uint64{},
		},
	}
}

// WithDefaultsForProd keeps caching disabled by default so redis writes stop unless explicitly re-enabled.

// AddFlags 注册缓存相关命令行参数
func (c *CacheOptions) AddFlags(fs *pflag.FlagSet) {
	if c == nil {
		return
	}
	if c.TTL == nil {
		c.TTL = &CacheTTLOptions{
			Scale:            24 * time.Hour,
			Questionnaire:    12 * time.Hour,
			AssessmentDetail: 2 * time.Hour,
			AssessmentStatus: 30 * time.Minute,
			Testee:           2 * time.Hour,
			Plan:             2 * time.Hour,
		}
	}
	fs.BoolVar(&c.DisableEvaluationCache, "cache.disable-evaluation-cache", c.DisableEvaluationCache,
		"Disable Redis caching for evaluation details")
	fs.BoolVar(&c.DisableStatisticsCache, "cache.disable-statistics-cache", c.DisableStatisticsCache,
		"Disable Redis caching for statistics data")
	fs.DurationVar(&c.TTL.Scale, "cache.ttl.scale", c.TTL.Scale, "TTL for scale cache entries.")
	fs.DurationVar(&c.TTL.Questionnaire, "cache.ttl.questionnaire", c.TTL.Questionnaire, "TTL for questionnaire cache entries.")
	fs.DurationVar(&c.TTL.AssessmentDetail, "cache.ttl.assessment-detail", c.TTL.AssessmentDetail, "TTL for assessment detail cache entries.")
	fs.DurationVar(&c.TTL.AssessmentStatus, "cache.ttl.assessment-status", c.TTL.AssessmentStatus, "TTL for assessment status cache entries.")
	fs.DurationVar(&c.TTL.Testee, "cache.ttl.testee", c.TTL.Testee, "TTL for testee cache entries.")
	fs.DurationVar(&c.TTL.Plan, "cache.ttl.plan", c.TTL.Plan, "TTL for plan cache entries.")
	fs.DurationVar(&c.TTL.Negative, "cache.ttl.negative", c.TTL.Negative, "TTL for negative cache entries (cache penetration protection).")
	fs.Float64Var(&c.TTLJitterRatio, "cache.ttl-jitter-ratio", c.TTLJitterRatio, "Jitter ratio (0-1) to spread cache expirations.")
	fs.StringVar(&c.Namespace, "cache.namespace", c.Namespace, "Optional Redis key namespace prefix (e.g., env name).")
	fs.BoolVar(&c.CompressPayload, "cache.compress-payload", c.CompressPayload, "Compress cache payloads (gzip) to save memory/bandwidth.")
	if c.StatisticsWarmup == nil {
		c.StatisticsWarmup = &StatisticsWarmupOptions{
			Enable:             false,
			OrgIDs:             []int64{1},
			QuestionnaireCodes: []string{},
			PlanIDs:            []uint64{},
		}
	}
}

// CacheTTLOptions 缓存 TTL 配置
type CacheTTLOptions struct {
	Scale            time.Duration `json:"scale" mapstructure:"scale"`
	Questionnaire    time.Duration `json:"questionnaire" mapstructure:"questionnaire"`
	AssessmentDetail time.Duration `json:"assessment_detail" mapstructure:"assessment_detail"`
	AssessmentStatus time.Duration `json:"assessment_status" mapstructure:"assessment_status"`
	Testee           time.Duration `json:"testee" mapstructure:"testee"`
	Plan             time.Duration `json:"plan" mapstructure:"plan"`
	Negative         time.Duration `json:"negative" mapstructure:"negative"`
}

// StatisticsWarmupOptions 统计查询结果缓存预热配置
type StatisticsWarmupOptions struct {
	Enable             bool     `json:"enable" mapstructure:"enable"`
	OrgIDs             []int64  `json:"org_ids" mapstructure:"org_ids"`
	QuestionnaireCodes []string `json:"questionnaire_codes" mapstructure:"questionnaire_codes"`
	PlanIDs            []uint64 `json:"plan_ids" mapstructure:"plan_ids"`
}

// StatisticsSyncOptions 统计同步定时任务配置
type StatisticsSyncOptions struct {
	Enable              bool          `json:"enable" mapstructure:"enable"`
	InitialDelay        time.Duration `json:"initial_delay" mapstructure:"initial_delay"`
	DailyInterval       time.Duration `json:"daily_interval" mapstructure:"daily_interval"`
	AccumulatedInterval time.Duration `json:"accumulated_interval" mapstructure:"accumulated_interval"`
	PlanInterval        time.Duration `json:"plan_interval" mapstructure:"plan_interval"`
}

// NewStatisticsSyncOptions 默认开启，10 分钟一次
func NewStatisticsSyncOptions() *StatisticsSyncOptions {
	return &StatisticsSyncOptions{
		Enable:              true,
		InitialDelay:        time.Minute,
		DailyInterval:       10 * time.Minute,
		AccumulatedInterval: 10 * time.Minute,
		PlanInterval:        30 * time.Minute,
	}
}

// AddFlags 注册统计同步相关命令行参数
func (s *StatisticsSyncOptions) AddFlags(fs *pflag.FlagSet) {
	if s == nil {
		return
	}
	fs.BoolVar(&s.Enable, "statistics_sync.enable", s.Enable, "Enable scheduled sync from Redis to MySQL for statistics.")
	fs.DurationVar(&s.InitialDelay, "statistics_sync.initial-delay", s.InitialDelay, "Initial delay before starting statistics sync schedulers.")
	fs.DurationVar(&s.DailyInterval, "statistics_sync.daily-interval", s.DailyInterval, "Interval for syncing daily statistics.")
	fs.DurationVar(&s.AccumulatedInterval, "statistics_sync.accumulated-interval", s.AccumulatedInterval, "Interval for syncing accumulated statistics.")
	fs.DurationVar(&s.PlanInterval, "statistics_sync.plan-interval", s.PlanInterval, "Interval for syncing plan statistics.")
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
