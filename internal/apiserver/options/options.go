package options

import (
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/pkg/app"
	"github.com/FangcunMount/qs-server/pkg/configmask"
	cliflag "github.com/FangcunMount/qs-server/pkg/flag"
	"github.com/spf13/pflag"
)

// Options 包含所有配置项
type Options struct {
	Log                            *log.Options                            `json:"log"       mapstructure:"log"`
	GenericServerRunOptions        *genericoptions.ServerRunOptions        `json:"server"    mapstructure:"server"`
	GRPCOptions                    *genericoptions.GRPCOptions             `json:"grpc"      mapstructure:"grpc"`
	InsecureServing                *genericoptions.InsecureServingOptions  `json:"insecure"  mapstructure:"insecure"`
	SecureServing                  *genericoptions.SecureServingOptions    `json:"secure"    mapstructure:"secure"`
	MySQLOptions                   *genericoptions.MySQLOptions            `json:"mysql"     mapstructure:"mysql"`
	MigrationOptions               *genericoptions.MigrationOptions        `json:"migration" mapstructure:"migration"`
	RedisOptions                   *genericoptions.RedisOptions            `json:"redis"     mapstructure:"redis"`
	RedisProfiles                  map[string]*genericoptions.RedisOptions `json:"redis_profiles" mapstructure:"redis_profiles"`
	RedisRuntime                   *genericoptions.RedisRuntimeOptions     `json:"redis_runtime" mapstructure:"redis_runtime"`
	LockLease                      *genericoptions.LockLeaseOptions        `json:"lock_lease" mapstructure:"lock_lease"`
	MongoDBOptions                 *genericoptions.MongoDBOptions          `json:"mongodb"   mapstructure:"mongodb"`
	MessagingOptions               *genericoptions.MessagingOptions        `json:"messaging" mapstructure:"messaging"`
	IAMOptions                     *genericoptions.IAMOptions              `json:"iam"       mapstructure:"iam"`
	OSSOptions                     *genericoptions.OSSOptions              `json:"oss"       mapstructure:"oss"`
	AssessmentAssets               *AssessmentAssetsOptions                `json:"assessment_assets" mapstructure:"assessment_assets"`
	WeChatOptions                  *genericoptions.WeChatOptions           `json:"wechat"    mapstructure:"wechat"`
	Plan                           *PlanOptions                            `json:"plan"      mapstructure:"plan"`
	PlanScheduler                  *PlanSchedulerOptions                   `json:"plan_scheduler" mapstructure:"plan_scheduler"`
	BehaviorPendingReconcile       *BehaviorPendingReconcileOptions        `json:"behavior_pending_reconcile" mapstructure:"behavior_pending_reconcile"`
	EvaluationConsistencyReconcile *EvaluationConsistencyReconcileOptions  `json:"evaluation_consistency_reconcile" mapstructure:"evaluation_consistency_reconcile"`
	BehaviorJourneyScan            *BehaviorJourneyScanOptions             `json:"behavior_journey_scan" mapstructure:"behavior_journey_scan"`
	OutboxRelay                    *OutboxRelayOptions                     `json:"outbox_relay" mapstructure:"outbox_relay"`
	Eventing                       *EventingOptions                        `json:"eventing" mapstructure:"eventing"`
	RateLimit                      *RateLimitOptions                       `json:"rate_limit" mapstructure:"rate_limit"`
	Backpressure                   *BackpressureOptions                    `json:"backpressure" mapstructure:"backpressure"`
	Cache                          *CacheOptions                           `json:"cache"     mapstructure:"cache"`
	StatisticsSync                 *StatisticsSyncOptions                  `json:"statistics_sync" mapstructure:"statistics_sync"`
	Signaling                      *genericoptions.SignalingOptions        `json:"signaling" mapstructure:"signaling"`
	SystemGovernance               *SystemGovernanceOptions                `json:"system_governance" mapstructure:"system_governance"`
	rawSettingsSource              app.RawSettingsSource
}

func (o *Options) SetRawSettingsSource(source app.RawSettingsSource) {
	if o != nil {
		o.rawSettingsSource = source
	}
}

func (o *Options) RawSettingsSource() app.RawSettingsSource {
	if o == nil {
		return nil
	}
	return o.rawSettingsSource
}

// NewOptions 创建一个 Options 对象，包含默认参数
func NewOptions() *Options {
	return &Options{
		Log:                            log.NewOptions(),
		GenericServerRunOptions:        genericoptions.NewServerRunOptions(),
		GRPCOptions:                    genericoptions.NewGRPCOptions(),
		InsecureServing:                genericoptions.NewInsecureServingOptions(),
		SecureServing:                  genericoptions.NewSecureServingOptions(),
		MySQLOptions:                   genericoptions.NewMySQLOptions(),
		MigrationOptions:               genericoptions.NewMigrationOptions(),
		RedisOptions:                   genericoptions.NewRedisOptions(),
		RedisProfiles:                  map[string]*genericoptions.RedisOptions{},
		RedisRuntime:                   defaultRedisRuntimeOptions(),
		LockLease:                      genericoptions.NewLockLeaseOptions(),
		MongoDBOptions:                 genericoptions.NewMongoDBOptions(),
		MessagingOptions:               genericoptions.NewMessagingOptions(),
		IAMOptions:                     genericoptions.NewIAMOptions(),
		OSSOptions:                     genericoptions.NewOSSOptions(),
		AssessmentAssets:               NewAssessmentAssetsOptions(),
		WeChatOptions:                  genericoptions.NewWeChatOptions(),
		Plan:                           NewPlanOptions(),
		PlanScheduler:                  NewPlanSchedulerOptions(),
		BehaviorPendingReconcile:       NewBehaviorPendingReconcileOptions(),
		EvaluationConsistencyReconcile: NewEvaluationConsistencyReconcileOptions(),
		BehaviorJourneyScan:            NewBehaviorJourneyScanOptions(),
		OutboxRelay:                    NewOutboxRelayOptions(),
		Eventing:                       NewEventingOptions(),
		RateLimit:                      NewRateLimitOptions(),
		Backpressure:                   NewBackpressureOptions(),
		Cache:                          NewCacheOptions(),
		StatisticsSync:                 NewStatisticsSyncOptions(),
		Signaling:                      genericoptions.NewSignalingOptions(),
		SystemGovernance:               NewSystemGovernanceOptions(),
	}
}

func defaultRedisRuntimeOptions() *genericoptions.RedisRuntimeOptions {
	opts := genericoptions.NewRedisRuntimeOptions()
	opts.Families["static_meta"] = &genericoptions.RedisRuntimeFamilyRoute{
		RedisProfile:         "static_cache",
		NamespaceSuffix:      "cache:static",
		AllowFallbackDefault: boolPtr(true),
		AllowWarmup:          boolPtr(true),
	}
	opts.Families["object_view"] = &genericoptions.RedisRuntimeFamilyRoute{
		RedisProfile:         "object_cache",
		NamespaceSuffix:      "cache:object",
		AllowFallbackDefault: boolPtr(true),
	}
	opts.Families["query_result"] = &genericoptions.RedisRuntimeFamilyRoute{
		RedisProfile:         "query_cache",
		NamespaceSuffix:      "cache:query",
		AllowFallbackDefault: boolPtr(true),
		AllowWarmup:          boolPtr(true),
	}
	opts.Families["meta_hotset"] = &genericoptions.RedisRuntimeFamilyRoute{
		RedisProfile:         "meta_cache",
		NamespaceSuffix:      "cache:meta",
		AllowFallbackDefault: boolPtr(true),
	}
	opts.Families["business_rank"] = &genericoptions.RedisRuntimeFamilyRoute{
		RedisProfile:         "rank_cache",
		NamespaceSuffix:      "cache:rank",
		AllowFallbackDefault: boolPtr(true),
	}
	opts.Families["sdk_token"] = &genericoptions.RedisRuntimeFamilyRoute{
		RedisProfile:         "sdk_cache",
		NamespaceSuffix:      "cache:sdk",
		AllowFallbackDefault: boolPtr(true),
	}
	opts.Families["lock_lease"] = &genericoptions.RedisRuntimeFamilyRoute{
		RedisProfile:         "lock_cache",
		NamespaceSuffix:      "cache:lock",
		AllowFallbackDefault: boolPtr(true),
	}
	opts.Families["ops_runtime"] = &genericoptions.RedisRuntimeFamilyRoute{
		RedisProfile:         "ops_runtime",
		NamespaceSuffix:      "ops:runtime",
		AllowFallbackDefault: boolPtr(true),
	}
	return opts
}

func boolPtr(v bool) *bool {
	return &v
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

const DefaultPlanEntryBaseURL = "https://collect.fangcunmount.cn/entry"

// PlanOptions 测评计划相关配置。
type PlanOptions struct {
	EntryBaseURL string `json:"entry_base_url" mapstructure:"entry_base_url"`
}

// NewPlanOptions 创建默认 plan 配置。
func NewPlanOptions() *PlanOptions {
	return &PlanOptions{
		EntryBaseURL: DefaultPlanEntryBaseURL,
	}
}

// PlanSchedulerOptions 内建 plan 调度器配置。
type PlanSchedulerOptions struct {
	Enable          bool          `json:"enable" mapstructure:"enable"`
	OrgIDs          []int64       `json:"org_ids" mapstructure:"org_ids"`
	InitialDelay    time.Duration `json:"initial_delay" mapstructure:"initial_delay"`
	Interval        time.Duration `json:"interval" mapstructure:"interval"`
	PendingLookback time.Duration `json:"pending_lookback" mapstructure:"pending_lookback"`
	LockKey         string        `json:"lock_key" mapstructure:"lock_key"`
	LockTTL         time.Duration `json:"lock_ttl" mapstructure:"lock_ttl"`
}

// NewPlanSchedulerOptions 创建默认 plan scheduler 配置。
func NewPlanSchedulerOptions() *PlanSchedulerOptions {
	return &PlanSchedulerOptions{
		Enable:          false,
		OrgIDs:          []int64{1},
		InitialDelay:    time.Minute,
		Interval:        time.Minute,
		PendingLookback: 24 * time.Hour,
		LockKey:         "qs:plan-scheduler:leader",
		LockTTL:         50 * time.Second,
	}
}

// AddFlags 注册 plan 相关命令行参数。
func (p *PlanOptions) AddFlags(fs *pflag.FlagSet) {
	if p == nil {
		return
	}
	fs.StringVar(&p.EntryBaseURL, "plan.entry-base-url", p.EntryBaseURL, "Public base URL used to generate plan task entry links.")
}

// AddFlags 注册内建 plan scheduler 相关参数。
func (p *PlanSchedulerOptions) AddFlags(fs *pflag.FlagSet) {
	if p == nil {
		return
	}
	fs.BoolVar(&p.Enable, "plan_scheduler.enable", p.Enable, "Enable the built-in plan task scheduler in qs-apiserver.")
	fs.Int64SliceVar(&p.OrgIDs, "plan_scheduler.org-ids", p.OrgIDs, "Organization IDs included in the built-in plan task scheduler.")
	fs.DurationVar(&p.InitialDelay, "plan_scheduler.initial-delay", p.InitialDelay, "Initial delay before starting the built-in plan task scheduler.")
	fs.DurationVar(&p.Interval, "plan_scheduler.interval", p.Interval, "Interval for scanning plan tasks in the built-in scheduler.")
	fs.DurationVar(&p.PendingLookback, "plan_scheduler.pending-lookback", p.PendingLookback, "How far back the built-in scheduler opens pending tasks.")
	fs.StringVar(&p.LockKey, "plan_scheduler.lock-key", p.LockKey, "Redis distributed lock key used by the built-in plan scheduler.")
	fs.DurationVar(&p.LockTTL, "plan_scheduler.lock-ttl", p.LockTTL, "Redis distributed lock TTL used by the built-in plan scheduler.")
}

// BehaviorPendingReconcileOptions 控制 pending behavior 事件归因补偿任务。
type BehaviorPendingReconcileOptions struct {
	Enable     bool          `json:"enable" mapstructure:"enable"`
	Interval   time.Duration `json:"interval" mapstructure:"interval"`
	BatchLimit int           `json:"batch_limit" mapstructure:"batch_limit"`
	LockKey    string        `json:"lock_key" mapstructure:"lock_key"`
	LockTTL    time.Duration `json:"lock_ttl" mapstructure:"lock_ttl"`
}

// NewBehaviorPendingReconcileOptions 创建默认 behavior pending reconcile 配置。
func NewBehaviorPendingReconcileOptions() *BehaviorPendingReconcileOptions {
	return &BehaviorPendingReconcileOptions{
		Enable:     true,
		Interval:   10 * time.Second,
		BatchLimit: 100,
		LockKey:    "qs:behavior-pending-reconcile:leader",
		LockTTL:    30 * time.Second,
	}
}

// AddFlags 注册 behavior pending reconcile 相关参数。
func (b *BehaviorPendingReconcileOptions) AddFlags(fs *pflag.FlagSet) {
	if b == nil {
		return
	}
	fs.BoolVar(&b.Enable, "behavior_pending_reconcile.enable", b.Enable, "Enable scheduled pending behavior reconcile.")
	fs.DurationVar(&b.Interval, "behavior_pending_reconcile.interval", b.Interval, "Interval for scanning pending behavior events.")
	fs.IntVar(&b.BatchLimit, "behavior_pending_reconcile.batch-limit", b.BatchLimit, "Maximum pending behavior events to process in one reconcile tick.")
	fs.StringVar(&b.LockKey, "behavior_pending_reconcile.lock-key", b.LockKey, "Redis distributed lock key used by the pending behavior reconcile scheduler.")
	fs.DurationVar(&b.LockTTL, "behavior_pending_reconcile.lock-ttl", b.LockTTL, "Redis distributed lock TTL used by the pending behavior reconcile scheduler.")
}

// EvaluationConsistencyReconcileOptions 控制 scoring/reporting 跨库终态对账补偿任务。
type EvaluationConsistencyReconcileOptions struct {
	Enable     bool          `json:"enable" mapstructure:"enable"`
	Interval   time.Duration `json:"interval" mapstructure:"interval"`
	BatchLimit int           `json:"batch_limit" mapstructure:"batch_limit"`
	LockKey    string        `json:"lock_key" mapstructure:"lock_key"`
	LockTTL    time.Duration `json:"lock_ttl" mapstructure:"lock_ttl"`
}

// NewEvaluationConsistencyReconcileOptions 创建默认 evaluation consistency reconcile 配置。
func NewEvaluationConsistencyReconcileOptions() *EvaluationConsistencyReconcileOptions {
	return &EvaluationConsistencyReconcileOptions{
		Enable:     true,
		Interval:   10 * time.Second,
		BatchLimit: 100,
		LockKey:    "qs:evaluation-consistency-reconcile:leader",
		LockTTL:    30 * time.Second,
	}
}

// AddFlags 注册 evaluation consistency reconcile 相关参数。
func (e *EvaluationConsistencyReconcileOptions) AddFlags(fs *pflag.FlagSet) {
	if e == nil {
		return
	}
	fs.BoolVar(&e.Enable, "evaluation_consistency_reconcile.enable", e.Enable, "Enable scheduled evaluation cross-store consistency reconcile.")
	fs.DurationVar(&e.Interval, "evaluation_consistency_reconcile.interval", e.Interval, "Interval for scanning evaluation consistency drift.")
	fs.IntVar(&e.BatchLimit, "evaluation_consistency_reconcile.batch-limit", e.BatchLimit, "Maximum assessments to scan in one evaluation consistency reconcile tick.")
	fs.StringVar(&e.LockKey, "evaluation_consistency_reconcile.lock-key", e.LockKey, "Redis distributed lock key used by the evaluation consistency reconcile scheduler.")
	fs.DurationVar(&e.LockTTL, "evaluation_consistency_reconcile.lock-ttl", e.LockTTL, "Redis distributed lock TTL used by the evaluation consistency reconcile scheduler.")
}

// BehaviorJourneyScanOptions controls background behavior journey scan projection.
type BehaviorJourneyScanOptions struct {
	Enable       bool          `json:"enable" mapstructure:"enable"`
	OrgIDs       []int64       `json:"org_ids" mapstructure:"org_ids"`
	InitialDelay time.Duration `json:"initial_delay" mapstructure:"initial_delay"`
	Interval     time.Duration `json:"interval" mapstructure:"interval"`
	BatchSize    int           `json:"batch_size" mapstructure:"batch_size"`
	Lookback     time.Duration `json:"lookback" mapstructure:"lookback"`
	LockKey      string        `json:"lock_key" mapstructure:"lock_key"`
	LockTTL      time.Duration `json:"lock_ttl" mapstructure:"lock_ttl"`
	Sources      []string      `json:"sources" mapstructure:"sources"`
	DryRun       bool          `json:"dry_run" mapstructure:"dry_run"`
	WindowRecalc bool          `json:"window_recalc" mapstructure:"window_recalc"`
}

// NewBehaviorJourneyScanOptions creates default behavior journey scan options.
func NewBehaviorJourneyScanOptions() *BehaviorJourneyScanOptions {
	return &BehaviorJourneyScanOptions{
		Enable:       false,
		InitialDelay: 2 * time.Minute,
		Interval:     30 * time.Minute,
		BatchSize:    1000,
		Lookback:     2 * time.Hour,
		LockKey:      "qs:behavior-journey-scan:leader",
		LockTTL:      25 * time.Minute,
		Sources: []string{
			"entry_resolve_log",
			"entry_intake_log",
			"answersheet",
			"assessment",
			"report",
		},
		WindowRecalc: true,
	}
}

// AddFlags registers behavior journey scan flags.
func (b *BehaviorJourneyScanOptions) AddFlags(fs *pflag.FlagSet) {
	if b == nil {
		return
	}
	fs.BoolVar(&b.Enable, "behavior_journey_scan.enable", b.Enable, "Enable background behavior journey scan projection.")
	fs.Int64SliceVar(&b.OrgIDs, "behavior_journey_scan.org-ids", b.OrgIDs, "Organization IDs included in behavior journey scan.")
	fs.DurationVar(&b.InitialDelay, "behavior_journey_scan.initial-delay", b.InitialDelay, "Initial delay before starting behavior journey scan.")
	fs.DurationVar(&b.Interval, "behavior_journey_scan.interval", b.Interval, "Interval for behavior journey scan ticks.")
	fs.IntVar(&b.BatchSize, "behavior_journey_scan.batch-size", b.BatchSize, "Maximum facts to scan per source in one tick.")
	fs.DurationVar(&b.Lookback, "behavior_journey_scan.lookback", b.Lookback, "Lookback window when no watermark exists.")
	fs.StringVar(&b.LockKey, "behavior_journey_scan.lock-key", b.LockKey, "Redis distributed lock key used by behavior journey scan.")
	fs.DurationVar(&b.LockTTL, "behavior_journey_scan.lock-ttl", b.LockTTL, "Redis distributed lock TTL used by behavior journey scan.")
	fs.StringSliceVar(&b.Sources, "behavior_journey_scan.sources", b.Sources, "Scan sources in execution order.")
	fs.BoolVar(&b.DryRun, "behavior_journey_scan.dry-run", b.DryRun, "Scan facts without writing projections.")
	fs.BoolVar(&b.WindowRecalc, "behavior_journey_scan.window-recalc", b.WindowRecalc, "Rebuild statistics_journey_daily for the lookback window after each org scan.")
}

// OutboxRelayOptions controls durable outbox relay loops inside qs-apiserver.
type OutboxRelayOptions struct {
	Mongo      *OutboxRelayStoreOptions `json:"mongo" mapstructure:"mongo"`
	Assessment *OutboxRelayStoreOptions `json:"assessment" mapstructure:"assessment"`
}

type OutboxRelayStoreOptions struct {
	Interval               time.Duration `json:"interval" mapstructure:"interval"`
	BatchSize              int           `json:"batch_size" mapstructure:"batch_size"`
	PublishWorkers         int           `json:"publish_workers" mapstructure:"publish_workers"`
	ImmediateMaxConcurrent int           `json:"immediate_max_concurrent" mapstructure:"immediate_max_concurrent"`
}

type EventingOptions struct {
	Consumers *EventConsumerOptions `json:"consumers" mapstructure:"consumers"`
}

type EventConsumerOptions struct {
	ModelCatalogHotRank *EventConsumerBindingOptions `json:"modelcatalog-hot-rank" mapstructure:"modelcatalog-hot-rank"`
}

type EventConsumerBindingOptions struct {
	Enabled bool   `json:"enabled" mapstructure:"enabled"`
	Channel string `json:"channel" mapstructure:"channel"`
}

func NewEventingOptions() *EventingOptions {
	return &EventingOptions{Consumers: &EventConsumerOptions{ModelCatalogHotRank: &EventConsumerBindingOptions{
		Enabled: true, Channel: "qs-apiserver-modelcatalog-hot-rank-v1",
	}}}
}

func (o *EventingOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil || o.Consumers == nil || o.Consumers.ModelCatalogHotRank == nil {
		return
	}
	hotRank := o.Consumers.ModelCatalogHotRank
	fs.BoolVar(&hotRank.Enabled, "eventing.consumer.modelcatalog-hot-rank.enabled", hotRank.Enabled, "Enable the independent modelcatalog hot-rank event consumer.")
	fs.StringVar(&hotRank.Channel, "eventing.consumer.modelcatalog-hot-rank.channel", hotRank.Channel, "Stable MQ channel for the modelcatalog hot-rank projection.")
}

func NewOutboxRelayOptions() *OutboxRelayOptions {
	return &OutboxRelayOptions{
		Mongo: &OutboxRelayStoreOptions{
			Interval:       500 * time.Millisecond,
			BatchSize:      300,
			PublishWorkers: 8,
		},
		Assessment: &OutboxRelayStoreOptions{
			Interval:               500 * time.Millisecond,
			BatchSize:              200,
			PublishWorkers:         24,
			ImmediateMaxConcurrent: 16,
		},
	}
}

func (o *OutboxRelayOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil {
		return
	}
	if o.Mongo != nil {
		fs.DurationVar(&o.Mongo.Interval, "outbox_relay.mongo.interval", o.Mongo.Interval, "Interval for dispatching Mongo durable outbox events.")
		fs.IntVar(&o.Mongo.BatchSize, "outbox_relay.mongo.batch-size", o.Mongo.BatchSize, "Maximum Mongo durable outbox events to claim in one relay tick.")
		fs.IntVar(&o.Mongo.PublishWorkers, "outbox_relay.mongo.publish-workers", o.Mongo.PublishWorkers, "Maximum concurrent Mongo durable outbox publish workers.")
		fs.IntVar(&o.Mongo.ImmediateMaxConcurrent, "outbox_relay.mongo.immediate-max-concurrent", o.Mongo.ImmediateMaxConcurrent, "Maximum concurrent post-commit immediate outbox dispatches for Mongo outbox (0 uses default).")
	}
	if o.Assessment != nil {
		fs.DurationVar(&o.Assessment.Interval, "outbox_relay.assessment.interval", o.Assessment.Interval, "Interval for dispatching assessment MySQL durable outbox events.")
		fs.IntVar(&o.Assessment.BatchSize, "outbox_relay.assessment.batch-size", o.Assessment.BatchSize, "Maximum assessment MySQL durable outbox events to claim in one relay tick.")
		fs.IntVar(&o.Assessment.PublishWorkers, "outbox_relay.assessment.publish-workers", o.Assessment.PublishWorkers, "Maximum concurrent assessment MySQL durable outbox publish workers.")
		fs.IntVar(&o.Assessment.ImmediateMaxConcurrent, "outbox_relay.assessment.immediate-max-concurrent", o.Assessment.ImmediateMaxConcurrent, "Maximum concurrent post-commit immediate outbox dispatches for assessment MySQL outbox (0 uses default).")
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
	o.RedisOptions.AddFlags(fss.FlagSet("redis"))
	o.RedisRuntime.AddFlags(fss.FlagSet("redis_runtime"))
	o.MongoDBOptions.AddFlags(fss.FlagSet("mongodb"))
	o.MessagingOptions.AddFlags(fss.FlagSet("messaging"))
	o.IAMOptions.AddFlags(fss.FlagSet("iam"))
	o.OSSOptions.AddFlags(fss.FlagSet("oss"))
	o.AssessmentAssets.AddFlags(fss.FlagSet("assessment_assets"))
	o.WeChatOptions.AddFlags(fss.FlagSet("wechat"))
	o.Plan.AddFlags(fss.FlagSet("plan"))
	o.PlanScheduler.AddFlags(fss.FlagSet("plan_scheduler"))
	o.BehaviorPendingReconcile.AddFlags(fss.FlagSet("behavior_pending_reconcile"))
	o.EvaluationConsistencyReconcile.AddFlags(fss.FlagSet("evaluation_consistency_reconcile"))
	o.BehaviorJourneyScan.AddFlags(fss.FlagSet("behavior_journey_scan"))
	o.OutboxRelay.AddFlags(fss.FlagSet("outbox_relay"))
	o.Eventing.AddFlags(fss.FlagSet("eventing"))
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
	Capabilities *CacheCapabilityOptions `json:"capabilities" mapstructure:"capabilities"`
	Defaults     *CacheDefaultsOptions   `json:"defaults" mapstructure:"defaults"`
	Governance   *CacheGovernanceOptions `json:"governance" mapstructure:"governance"`
}

type CacheCapabilityOptions struct {
	Survey       *SurveyCacheCapabilities            `json:"survey" mapstructure:"survey"`
	ModelCatalog *ModelCatalogCacheCapabilities      `json:"modelcatalog" mapstructure:"modelcatalog"`
	Evaluation   *EvaluationCacheCapabilities        `json:"evaluation" mapstructure:"evaluation"`
	Actor        *ActorCacheCapabilities             `json:"actor" mapstructure:"actor"`
	Plan         *PlanCacheCapabilities              `json:"plan" mapstructure:"plan"`
	Statistics   *StatisticsCacheCapabilities        `json:"statistics" mapstructure:"statistics"`
	ReportStatus *genericoptions.ReportStatusOptions `json:"report_status" mapstructure:"report_status"`
}

type SurveyCacheCapabilities struct {
	Questionnaire *CapabilityPolicyOptions `json:"questionnaire" mapstructure:"questionnaire"`
}
type ModelCatalogCacheCapabilities struct {
	PublishedModel *CapabilityPolicyOptions `json:"published_model" mapstructure:"published_model"`
}
type EvaluationCacheCapabilities struct {
	AssessmentDetail *CapabilityPolicyOptions `json:"assessment_detail" mapstructure:"assessment_detail"`
	AssessmentList   *CapabilityPolicyOptions `json:"assessment_list" mapstructure:"assessment_list"`
}
type ActorCacheCapabilities struct {
	Testee *CapabilityPolicyOptions `json:"testee" mapstructure:"testee"`
}
type PlanCacheCapabilities struct {
	Detail *CapabilityPolicyOptions `json:"detail" mapstructure:"detail"`
}
type StatisticsCacheCapabilities struct {
	Query *CapabilityPolicyOptions `json:"query" mapstructure:"query"`
}

type CapabilityPolicyOptions struct {
	Enabled        bool          `json:"enabled" mapstructure:"enabled"`
	TTL            time.Duration `json:"ttl" mapstructure:"ttl"`
	NegativeTTL    time.Duration `json:"negative_ttl" mapstructure:"negative_ttl"`
	TTLJitterRatio float64       `json:"ttl_jitter_ratio" mapstructure:"ttl_jitter_ratio"`
	Compress       *bool         `json:"compress,omitempty" mapstructure:"compress"`
	Singleflight   *bool         `json:"singleflight,omitempty" mapstructure:"singleflight"`
	Negative       *bool         `json:"negative,omitempty" mapstructure:"negative"`
}

type CacheDefaultsOptions struct {
	CompressPayload bool                `json:"compress_payload" mapstructure:"compress_payload"`
	TTLJitterRatio  float64             `json:"ttl_jitter_ratio" mapstructure:"ttl_jitter_ratio"`
	Static          *CacheFamilyOptions `json:"static" mapstructure:"static"`
	Object          *CacheFamilyOptions `json:"object" mapstructure:"object"`
	Query           *CacheFamilyOptions `json:"query" mapstructure:"query"`
}

type CacheGovernanceOptions struct {
	StatisticsWarmup   *StatisticsWarmupOptions   `json:"statistics_warmup" mapstructure:"statistics_warmup"`
	StatisticsOverview *StatisticsOverviewOptions `json:"statistics_overview" mapstructure:"statistics_overview"`
	Warmup             *WarmupOptions             `json:"warmup" mapstructure:"warmup"`
}

// NewCacheOptions 创建默认缓存配置
func NewCacheOptions() *CacheOptions {
	return &CacheOptions{
		Capabilities: &CacheCapabilityOptions{
			Survey:       &SurveyCacheCapabilities{Questionnaire: &CapabilityPolicyOptions{Enabled: true, TTL: 12 * time.Hour, Negative: cacheBoolPtr(true)}},
			ModelCatalog: &ModelCatalogCacheCapabilities{PublishedModel: &CapabilityPolicyOptions{Enabled: true, TTL: 24 * time.Hour, Negative: cacheBoolPtr(true)}},
			Evaluation: &EvaluationCacheCapabilities{
				AssessmentDetail: &CapabilityPolicyOptions{Enabled: true, TTL: 2 * time.Hour, Singleflight: cacheBoolPtr(true)},
				AssessmentList:   &CapabilityPolicyOptions{Enabled: true, TTL: 10 * time.Minute, Singleflight: cacheBoolPtr(false)},
			},
			Actor:        &ActorCacheCapabilities{Testee: &CapabilityPolicyOptions{Enabled: true, TTL: 30 * time.Minute, Negative: cacheBoolPtr(true)}},
			Plan:         &PlanCacheCapabilities{Detail: &CapabilityPolicyOptions{Enabled: true, TTL: 2 * time.Hour, Singleflight: cacheBoolPtr(true)}},
			Statistics:   &StatisticsCacheCapabilities{Query: &CapabilityPolicyOptions{Enabled: true, TTL: 5 * time.Minute, Singleflight: cacheBoolPtr(false)}},
			ReportStatus: genericoptions.NewReportStatusOptions(),
		},
		Defaults: &CacheDefaultsOptions{
			TTLJitterRatio: 0.1,
			Static:         &CacheFamilyOptions{NegativeTTL: 5 * time.Minute},
			Object:         &CacheFamilyOptions{NegativeTTL: 5 * time.Minute},
			Query:          &CacheFamilyOptions{NegativeTTL: 5 * time.Minute},
		},
		Governance: &CacheGovernanceOptions{
			StatisticsWarmup: &StatisticsWarmupOptions{
				Enable:          false,
				WarmOnStartup:   true,
				OrgIDs:          []int64{1},
				OverviewPresets: []string{"today", "7d", "30d"},
			},
			StatisticsOverview: &StatisticsOverviewOptions{
				ServiceSingleflight: true,
				StaleOnTimeout:      true,
				LoadTimeout:         25 * time.Second,
			},
			Warmup: &WarmupOptions{
				Enable: true,
				Startup: &WarmupStartupOptions{
					Static: true,
					Query:  true,
				},
				Hotset: &WarmupHotsetOptions{
					Enable:          true,
					TopN:            20,
					MaxItemsPerKind: 200,
				},
			},
		},
	}
}

func cacheBoolPtr(value bool) *bool { return &value }

// AddFlags 注册缓存相关命令行参数
func (c *CacheOptions) AddFlags(fs *pflag.FlagSet) {
	if c == nil {
		return
	}
	if c.Capabilities == nil {
		c.Capabilities = NewCacheOptions().Capabilities
	}
	if c.Defaults == nil {
		c.Defaults = &CacheDefaultsOptions{}
	}
	if c.Governance == nil {
		c.Governance = &CacheGovernanceOptions{}
	}
	ensureCacheCapabilities(c.Capabilities)
	addCapabilityFlags(fs, "cache.capabilities.survey.questionnaire", c.Capabilities.Survey.Questionnaire)
	addCapabilityFlags(fs, "cache.capabilities.modelcatalog.published_model", c.Capabilities.ModelCatalog.PublishedModel)
	addCapabilityFlags(fs, "cache.capabilities.evaluation.assessment_detail", c.Capabilities.Evaluation.AssessmentDetail)
	addCapabilityFlags(fs, "cache.capabilities.evaluation.assessment_list", c.Capabilities.Evaluation.AssessmentList)
	addCapabilityFlags(fs, "cache.capabilities.actor.testee", c.Capabilities.Actor.Testee)
	addCapabilityFlags(fs, "cache.capabilities.plan.detail", c.Capabilities.Plan.Detail)
	addCapabilityFlags(fs, "cache.capabilities.statistics.query", c.Capabilities.Statistics.Query)
	fs.Float64Var(&c.Defaults.TTLJitterRatio, "cache.defaults.ttl_jitter_ratio", c.Defaults.TTLJitterRatio, "Jitter ratio (0-1) to spread cache expirations.")
	fs.BoolVar(&c.Defaults.CompressPayload, "cache.defaults.compress_payload", c.Defaults.CompressPayload, "Compress cache payloads (gzip) to save memory/bandwidth.")
	if c.Defaults.Static == nil {
		c.Defaults.Static = &CacheFamilyOptions{}
	}
	if c.Defaults.Query == nil {
		c.Defaults.Query = &CacheFamilyOptions{}
	}
	if c.Defaults.Object == nil {
		c.Defaults.Object = &CacheFamilyOptions{}
	}
	fs.DurationVar(&c.Defaults.Static.NegativeTTL, "cache.defaults.static.negative_ttl", c.Defaults.Static.NegativeTTL, "Default negative TTL for static caches.")
	fs.DurationVar(&c.Defaults.Object.NegativeTTL, "cache.defaults.object.negative_ttl", c.Defaults.Object.NegativeTTL, "Default negative TTL for object caches.")
	fs.DurationVar(&c.Defaults.Query.NegativeTTL, "cache.defaults.query.negative_ttl", c.Defaults.Query.NegativeTTL, "Default negative-cache TTL used by query-result caches.")
	fs.Float64Var(&c.Defaults.Query.TTLJitterRatio, "cache.defaults.query.ttl_jitter_ratio", c.Defaults.Query.TTLJitterRatio, "TTL jitter ratio override for query-result caches (0 uses the global cache.defaults.ttl_jitter_ratio).")
	if c.Governance.StatisticsWarmup == nil {
		c.Governance.StatisticsWarmup = &StatisticsWarmupOptions{
			Enable:          false,
			WarmOnStartup:   true,
			OrgIDs:          []int64{1},
			OverviewPresets: []string{"today", "7d", "30d"},
		}
	}
	if c.Governance.StatisticsOverview == nil {
		c.Governance.StatisticsOverview = &StatisticsOverviewOptions{
			ServiceSingleflight: true,
			StaleOnTimeout:      true,
			LoadTimeout:         25 * time.Second,
		}
	}
	if c.Governance.Warmup == nil {
		c.Governance.Warmup = &WarmupOptions{
			Enable: true,
			Startup: &WarmupStartupOptions{
				Static: true,
				Query:  true,
			},
			Hotset: &WarmupHotsetOptions{
				Enable:          true,
				TopN:            20,
				MaxItemsPerKind: 200,
			},
		}
	}
	if c.Governance.Warmup.Startup == nil {
		c.Governance.Warmup.Startup = &WarmupStartupOptions{Static: true, Query: true}
	}
	if c.Governance.Warmup.Hotset == nil {
		c.Governance.Warmup.Hotset = &WarmupHotsetOptions{Enable: true, TopN: 20, MaxItemsPerKind: 200}
	}
	fs.BoolVar(&c.Governance.Warmup.Enable, "cache.governance.warmup.enable", c.Governance.Warmup.Enable, "Enable cache governance warmup orchestration.")
	fs.BoolVar(&c.Governance.Warmup.Startup.Static, "cache.governance.warmup.startup.static", c.Governance.Warmup.Startup.Static, "Enable startup warmup for static cache family.")
	fs.BoolVar(&c.Governance.Warmup.Startup.Query, "cache.governance.warmup.startup.query", c.Governance.Warmup.Startup.Query, "Enable startup warmup for query cache family.")
	fs.BoolVar(&c.Governance.Warmup.Hotset.Enable, "cache.governance.warmup.hotset.enable", c.Governance.Warmup.Hotset.Enable, "Enable internal hotset recording and selection for warmup governance.")
	fs.Int64Var(&c.Governance.Warmup.Hotset.TopN, "cache.governance.warmup.hotset.top_n", c.Governance.Warmup.Hotset.TopN, "Top-N hot targets loaded from meta_cache per warmup kind.")
	fs.Int64Var(&c.Governance.Warmup.Hotset.MaxItemsPerKind, "cache.governance.warmup.hotset.max_items_per_kind", c.Governance.Warmup.Hotset.MaxItemsPerKind, "Maximum hotset members retained per warmup kind.")
}

func ensureCacheCapabilities(c *CacheCapabilityOptions) {
	defaults := NewCacheOptions().Capabilities
	if c.Survey == nil {
		c.Survey = defaults.Survey
	}
	if c.Survey.Questionnaire == nil {
		c.Survey.Questionnaire = defaults.Survey.Questionnaire
	}
	if c.ModelCatalog == nil {
		c.ModelCatalog = defaults.ModelCatalog
	}
	if c.ModelCatalog.PublishedModel == nil {
		c.ModelCatalog.PublishedModel = defaults.ModelCatalog.PublishedModel
	}
	if c.Evaluation == nil {
		c.Evaluation = defaults.Evaluation
	}
	if c.Evaluation.AssessmentDetail == nil {
		c.Evaluation.AssessmentDetail = defaults.Evaluation.AssessmentDetail
	}
	if c.Evaluation.AssessmentList == nil {
		c.Evaluation.AssessmentList = defaults.Evaluation.AssessmentList
	}
	if c.Actor == nil {
		c.Actor = defaults.Actor
	}
	if c.Actor.Testee == nil {
		c.Actor.Testee = defaults.Actor.Testee
	}
	if c.Plan == nil {
		c.Plan = defaults.Plan
	}
	if c.Plan.Detail == nil {
		c.Plan.Detail = defaults.Plan.Detail
	}
	if c.Statistics == nil {
		c.Statistics = defaults.Statistics
	}
	if c.Statistics.Query == nil {
		c.Statistics.Query = defaults.Statistics.Query
	}
	if c.ReportStatus == nil {
		c.ReportStatus = defaults.ReportStatus
	}
}

func addCapabilityFlags(fs *pflag.FlagSet, prefix string, c *CapabilityPolicyOptions) {
	fs.BoolVar(&c.Enabled, prefix+".enabled", c.Enabled, "Enable this cache capability.")
	fs.DurationVar(&c.TTL, prefix+".ttl", c.TTL, "TTL for this cache capability.")
	fs.DurationVar(&c.NegativeTTL, prefix+".negative_ttl", c.NegativeTTL, "Negative-cache TTL override for this capability.")
	fs.Float64Var(&c.TTLJitterRatio, prefix+".ttl_jitter_ratio", c.TTLJitterRatio, "TTL jitter ratio override for this capability.")
	addOptionalBoolFlag(fs, prefix+".compress", &c.Compress, "Override payload compression for this capability.")
	addOptionalBoolFlag(fs, prefix+".singleflight", &c.Singleflight, "Override miss coalescing for this capability.")
	addOptionalBoolFlag(fs, prefix+".negative", &c.Negative, "Override negative caching for this capability.")
}

type optionalBoolValue struct{ target **bool }

func (v *optionalBoolValue) String() string {
	if v == nil || v.target == nil || *v.target == nil {
		return "inherit"
	}
	return strconv.FormatBool(**v.target)
}

func (v *optionalBoolValue) Set(raw string) error {
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return err
	}
	*v.target = &parsed
	return nil
}

func (*optionalBoolValue) Type() string { return "bool" }

func addOptionalBoolFlag(fs *pflag.FlagSet, name string, target **bool, usage string) {
	fs.Var(&optionalBoolValue{target: target}, name, usage)
	fs.Lookup(name).NoOptDefVal = "true"
}

// CacheFamilyOptions 定义单个缓存 family 的对象级策略。
// Redis profile 与 namespace 统一由 redis_runtime 管理，这里只保留 TTL、negative、压缩与 singleflight 语义。
type CacheFamilyOptions struct {
	NegativeTTL    time.Duration `json:"negative_ttl" mapstructure:"negative_ttl"`
	TTLJitterRatio float64       `json:"ttl_jitter_ratio" mapstructure:"ttl_jitter_ratio"`
	Compress       *bool         `json:"compress,omitempty" mapstructure:"compress"`
	Singleflight   *bool         `json:"singleflight,omitempty" mapstructure:"singleflight"`
	Negative       *bool         `json:"negative,omitempty" mapstructure:"negative"`
}

// StatisticsWarmupOptions 统计查询结果缓存预热配置
type StatisticsWarmupOptions struct {
	Enable          bool     `json:"enable" mapstructure:"enable"`
	WarmOnStartup   bool     `json:"warm_on_startup" mapstructure:"warm_on_startup"`
	OrgIDs          []int64  `json:"org_ids" mapstructure:"org_ids"`
	OverviewPresets []string `json:"overview_presets" mapstructure:"overview_presets"`
}

// StatisticsOverviewOptions 机构统计总览读保护与降级配置。
type StatisticsOverviewOptions struct {
	ServiceSingleflight bool          `json:"service_singleflight" mapstructure:"service_singleflight"`
	StaleOnTimeout      bool          `json:"stale_on_timeout" mapstructure:"stale_on_timeout"`
	LoadTimeout         time.Duration `json:"load_timeout" mapstructure:"load_timeout"`
}

type WarmupOptions struct {
	Enable  bool                  `json:"enable" mapstructure:"enable"`
	Startup *WarmupStartupOptions `json:"startup" mapstructure:"startup"`
	Hotset  *WarmupHotsetOptions  `json:"hotset" mapstructure:"hotset"`
}

type WarmupStartupOptions struct {
	Static bool `json:"static" mapstructure:"static"`
	Query  bool `json:"query" mapstructure:"query"`
}

type WarmupHotsetOptions struct {
	Enable          bool  `json:"enable" mapstructure:"enable"`
	TopN            int64 `json:"top_n" mapstructure:"top_n"`
	MaxItemsPerKind int64 `json:"max_items_per_kind" mapstructure:"max_items_per_kind"`
}

// StatisticsSyncOptions 统计同步定时任务配置
type StatisticsSyncOptions struct {
	Enable           bool          `json:"enable" mapstructure:"enable"`
	OrgIDs           []int64       `json:"org_ids" mapstructure:"org_ids"`
	RunAt            string        `json:"run_at" mapstructure:"run_at"`
	RepairWindowDays int           `json:"repair_window_days" mapstructure:"repair_window_days"`
	LockKey          string        `json:"lock_key" mapstructure:"lock_key"`
	LockTTL          time.Duration `json:"lock_ttl" mapstructure:"lock_ttl"`
}

// NewStatisticsSyncOptions 默认开启，每日凌晨 00:30 同步一次。
func NewStatisticsSyncOptions() *StatisticsSyncOptions {
	return &StatisticsSyncOptions{
		Enable:           true,
		OrgIDs:           []int64{1},
		RunAt:            "00:30",
		RepairWindowDays: 7,
		LockKey:          "qs:statistics-sync:leader",
		LockTTL:          30 * time.Minute,
	}
}

// AddFlags 注册统计同步相关命令行参数
func (s *StatisticsSyncOptions) AddFlags(fs *pflag.FlagSet) {
	if s == nil {
		return
	}
	fs.BoolVar(&s.Enable, "statistics_sync.enable", s.Enable, "Enable scheduled nightly statistics sync.")
	fs.Int64SliceVar(&s.OrgIDs, "statistics_sync.org-ids", s.OrgIDs, "Organization IDs included in scheduled statistics sync.")
	fs.StringVar(&s.RunAt, "statistics_sync.run-at", s.RunAt, "Daily wall-clock time for statistics sync, in HH:MM format.")
	fs.IntVar(&s.RepairWindowDays, "statistics_sync.repair-window-days", s.RepairWindowDays, "Number of completed days to rebuild when running scheduled daily statistics sync.")
	fs.StringVar(&s.LockKey, "statistics_sync.lock-key", s.LockKey, "Redis distributed lock key used by the scheduled statistics sync.")
	fs.DurationVar(&s.LockTTL, "statistics_sync.lock-ttl", s.LockTTL, "Redis distributed lock TTL used by the scheduled statistics sync.")
}

// Complete 完成配置选项
func (o *Options) Complete() error {
	return o.SecureServing.Complete()
}

// String 返回配置的字符串表示
func (o *Options) String() string {
	return configmask.String(o)
}
