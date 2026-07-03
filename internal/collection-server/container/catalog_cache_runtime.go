package container

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/catalogcache"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/personalitymodel"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/pkg/cachesignal"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	goredis "github.com/redis/go-redis/v9"
)

type catalogCaches struct {
	questionnaire questionnaire.PublishedDetailCache
	scale         scale.CatalogCache
	personality   personalitymodel.CatalogCache
}

func (c *Container) initCatalogCaches() catalogCaches {
	questionnaireCache := newQuestionnaireDetailCache(c.opts)
	c.startQuestionnaireCacheSignalWatcher(questionnaireCache)

	scaleCache := newScaleCatalogCache(c.opts)
	c.startScaleCacheSignalWatcher(scaleCache)

	personalityCache := newPersonalityCatalogCache(c.opts)
	c.startPersonalityCacheSignalWatcher(personalityCache)

	return catalogCaches{
		questionnaire: questionnaireCache,
		scale:         scaleCache,
		personality:   personalityCache,
	}
}

func (c *Container) cleanupCatalogCaches() {
	if c.questionnaireCacheWatcherCancel != nil {
		c.questionnaireCacheWatcherCancel()
		c.questionnaireCacheWatcherCancel = nil
	}
	if c.scaleCacheWatcherCancel != nil {
		c.scaleCacheWatcherCancel()
		c.scaleCacheWatcherCancel = nil
	}
	if c.personalityCacheWatcherCancel != nil {
		c.personalityCacheWatcherCancel()
		c.personalityCacheWatcherCancel = nil
	}
}

func questionnaireCatalogCfg(opts *options.Options) *options.CatalogL1CacheOptions {
	if opts == nil || opts.QuestionnaireCache == nil {
		return nil
	}
	return &opts.QuestionnaireCache.CatalogL1CacheOptions
}

func scaleCatalogCfg(opts *options.Options) *options.CatalogL1CacheOptions {
	if opts == nil || opts.ScaleCache == nil {
		return nil
	}
	return &opts.ScaleCache.CatalogL1CacheOptions
}

func personalityCatalogCfg(opts *options.Options) *options.CatalogL1CacheOptions {
	if opts == nil || opts.PersonalityCache == nil {
		return nil
	}
	return &opts.PersonalityCache.CatalogL1CacheOptions
}

func newQuestionnaireDetailCache(opts *options.Options) questionnaire.PublishedDetailCache {
	cfg := catalogCacheConfig(questionnaireCatalogCfg(opts))
	if cfg == nil {
		return nil
	}
	cacheOpts := localCacheOptionsFromCatalog("questionnaire", cfg)
	return questionnaire.NewLocalCache(questionnaire.LocalCacheOptions{
		TTL:            cacheOpts.TTL,
		MaxEntries:     cacheOpts.MaxEntries,
		TTLJitterRatio: cacheOpts.TTLJitterRatio,
		OnHit:          cacheOpts.OnHit,
		OnMiss:         cacheOpts.OnMiss,
	})
}

func newScaleCatalogCache(opts *options.Options) scale.CatalogCache {
	cfg := catalogCacheConfig(scaleCatalogCfg(opts))
	if cfg == nil {
		return nil
	}
	cacheOpts := localCacheOptionsFromCatalog("scale", cfg)
	return scale.NewLocalCatalogCache(scale.LocalCatalogCacheOptions{
		TTL:            cacheOpts.TTL,
		MaxEntries:     cacheOpts.MaxEntries,
		TTLJitterRatio: cacheOpts.TTLJitterRatio,
		OnHit:          cacheOpts.OnHit,
		OnMiss:         cacheOpts.OnMiss,
	})
}

func newPersonalityCatalogCache(opts *options.Options) personalitymodel.CatalogCache {
	cfg := catalogCacheConfig(personalityCatalogCfg(opts))
	if cfg == nil {
		return nil
	}
	cacheOpts := localCacheOptionsFromCatalog("personality", cfg)
	return personalitymodel.NewLocalCatalogCache(personalitymodel.LocalCatalogCacheOptions{
		TTL:            cacheOpts.TTL,
		MaxEntries:     cacheOpts.MaxEntries,
		TTLJitterRatio: cacheOpts.TTLJitterRatio,
		OnHit:          cacheOpts.OnHit,
		OnMiss:         cacheOpts.OnMiss,
	})
}

func questionnaireCacheSingleflightEnabled(opts *options.Options) bool {
	if opts == nil {
		return false
	}
	return catalogSingleflightEnabled(questionnaireCatalogCfg(opts))
}

func scaleCacheSingleflightEnabled(opts *options.Options) bool {
	if opts == nil {
		return false
	}
	return catalogSingleflightEnabled(scaleCatalogCfg(opts))
}

func personalityCacheSingleflightEnabled(opts *options.Options) bool {
	if opts == nil {
		return false
	}
	return catalogSingleflightEnabled(personalityCatalogCfg(opts))
}

func catalogCacheConfig(cfg *options.CatalogL1CacheOptions) *options.CatalogL1CacheOptions {
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	return cfg
}

func catalogSingleflightEnabled(cfg *options.CatalogL1CacheOptions) bool {
	return cfg != nil && cfg.Singleflight
}

func catalogSignalEvictEnabled(cfg *options.CatalogL1CacheOptions) bool {
	return catalogCacheConfig(cfg) != nil && cfg.SignalEvictEnabled
}

type localCacheOptions struct {
	TTL            time.Duration
	MaxEntries     int
	TTLJitterRatio float64
	OnHit          func()
	OnMiss         func()
}

func localCacheOptionsFromCatalog(kind string, cfg *options.CatalogL1CacheOptions) localCacheOptions {
	base := catalogcache.LocalTTLCacheOptions(
		kind,
		time.Duration(cfg.TTLSeconds)*time.Second,
		cfg.MaxEntries,
		cfg.TTLJitterRatio,
	)
	return localCacheOptions{
		TTL:            base.TTL,
		MaxEntries:     base.MaxEntries,
		TTLJitterRatio: base.TTLJitterRatio,
		OnHit:          base.OnHit,
		OnMiss:         base.OnMiss,
	}
}

func (c *Container) startQuestionnaireCacheSignalWatcher(cache questionnaire.PublishedDetailCache) {
	if c == nil || cache == nil || !catalogSignalEvictEnabled(questionnaireCatalogCfg(c.opts)) {
		return
	}
	standalone, cfg, ok := c.cacheSignalingStandalone("questionnaire cache signal watcher")
	if !ok {
		return
	}
	signaler, err := cachesignal.NewQuestionnaireSignaler(standalone, cfg)
	if err != nil {
		log.Warnf("questionnaire cache signal watcher disabled: %v", err)
		return
	}
	watchCtx, cancel := context.WithCancel(context.Background())
	questionnaire.StartCacheSignalWatcher(watchCtx, signaler, cache)
	c.questionnaireCacheWatcherCancel = cancel
}

func (c *Container) startScaleCacheSignalWatcher(cache scale.CatalogCache) {
	if c == nil || cache == nil || !catalogSignalEvictEnabled(scaleCatalogCfg(c.opts)) {
		return
	}
	standalone, cfg, ok := c.cacheSignalingStandalone("scale cache signal watcher")
	if !ok {
		return
	}
	signaler, err := cachesignal.NewScaleSignaler(standalone, cfg)
	if err != nil {
		log.Warnf("scale cache signal watcher disabled: %v", err)
		return
	}
	watchCtx, cancel := context.WithCancel(context.Background())
	scale.StartCacheSignalWatcher(watchCtx, signaler, cache)
	c.scaleCacheWatcherCancel = cancel
}

func (c *Container) startPersonalityCacheSignalWatcher(cache personalitymodel.CatalogCache) {
	if c == nil || cache == nil || !catalogSignalEvictEnabled(personalityCatalogCfg(c.opts)) {
		return
	}
	standalone, cfg, ok := c.cacheSignalingStandalone("personality model cache signal watcher")
	if !ok {
		return
	}
	signaler, err := cachesignal.NewPersonalityModelSignaler(standalone, cfg)
	if err != nil {
		log.Warnf("personality model cache signal watcher disabled: %v", err)
		return
	}
	watchCtx, cancel := context.WithCancel(context.Background())
	personalitymodel.StartCacheSignalWatcher(watchCtx, signaler, cache)
	c.personalityCacheWatcherCancel = cancel
}

func (c *Container) cacheSignalingStandalone(label string) (*goredis.Client, cachesignal.SignalingOptions, bool) {
	if c == nil {
		return nil, cachesignal.SignalingOptions{}, false
	}
	var sigOpts *genericoptions.SignalingOptions
	if c.opts != nil {
		sigOpts = c.opts.Signaling
	}
	cfg := cachesignal.ConfigFromOptions(sigOpts, "collection-server")
	if !cfg.Signaling.Enabled || c.opsHandle == nil || c.opsHandle.Client == nil {
		return nil, cachesignal.SignalingOptions{}, false
	}
	standalone, err := cachesignal.AsStandaloneClient(c.opsHandle.Client)
	if err != nil {
		log.Warnf("%s disabled: %v", label, err)
		return nil, cachesignal.SignalingOptions{}, false
	}
	return standalone, cfg.Signaling, true
}
