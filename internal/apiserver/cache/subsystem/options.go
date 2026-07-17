package cachebootstrap

import (
	"context"
	"time"

	cachepolicy "github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
)

// CacheOptions 描述 apiserver cache 子系统的运行时配置。
type CacheOptions struct {
	Capabilities       map[sharedcache.Capability]cachepolicy.Binding
	TTLJitterRatio     float64
	StatisticsWarmup   *cachegov.StatisticsWarmupConfig
	StatisticsOverview StatisticsReadGuardOptions
	Warmup             WarmupOptions
	Signal             SignalOptions
	CompressPayload    bool
	Static             CacheFamilyOptions
	Object             CacheFamilyOptions
	Query              CacheFamilyOptions
}

// SignalOptions controls the best-effort Redis Pub/Sub cache invalidation signals.
type SignalOptions struct {
	Enabled    bool
	Prefix     string
	Channel    string
	BufferSize int
}

// SignalNotifier is the narrow cache-signal port exposed to business modules.
type SignalNotifier interface {
	NotifyQuestionnaireCacheChanged(context.Context, string, string, string)
	NotifyScaleCacheChanged(context.Context, string, string)
	NotifyTypologyModelCacheChanged(context.Context, string, string)
}

// StatisticsReadGuardOptions controls Overview read coalescing and stale fallback.
type StatisticsReadGuardOptions struct {
	ServiceSingleflight bool
	StaleOnTimeout      bool
	LoadTimeout         time.Duration
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
	NegativeTTL    time.Duration
	TTLJitterRatio float64
	Compress       *bool
	Singleflight   *bool
	Negative       *bool
}
