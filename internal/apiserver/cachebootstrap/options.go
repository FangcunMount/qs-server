package cachebootstrap

import (
	"time"

	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
)

// CacheOptions 描述 apiserver cache 子系统的运行时配置。
type CacheOptions struct {
	DisableEvaluationCache bool
	DisableStatisticsCache bool
	TTL                    CacheTTLOptions
	TTLJitterRatio         float64
	StatisticsWarmup       *cachegov.StatisticsWarmupConfig
	Warmup                 WarmupOptions
	CompressPayload        bool
	Static                 CacheFamilyOptions
	Object                 CacheFamilyOptions
	Query                  CacheFamilyOptions
	Meta                   CacheFamilyOptions
	SDK                    CacheFamilyOptions
	Lock                   CacheFamilyOptions
}

type WarmupOptions struct {
	Enable          bool
	StartupStatic   bool
	StartupQuery    bool
	HotsetEnable    bool
	HotsetTopN      int64
	MaxItemsPerKind int64
}

// CacheFamilyOptions 定义单个缓存 family 的对象级策略。
type CacheFamilyOptions struct {
	TTL            time.Duration
	NegativeTTL    time.Duration
	TTLJitterRatio float64
	Compress       *bool
	Singleflight   *bool
	Negative       *bool
}

// CacheTTLOptions 缓存 TTL 配置（0 表示使用默认值）。
type CacheTTLOptions struct {
	Scale            time.Duration
	ScaleList        time.Duration
	Questionnaire    time.Duration
	AssessmentDetail time.Duration
	AssessmentList   time.Duration
	Testee           time.Duration
	Plan             time.Duration
	Negative         time.Duration
}
