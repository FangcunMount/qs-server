package container

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/signaling"
	signalredis "github.com/FangcunMount/component-base/pkg/signaling/redis"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/catalogcache"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/pkg/cachesignal"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	goredis "github.com/redis/go-redis/v9"
)

type catalogCaches struct {
	questionnaire questionnaire.PublishedDetailCache
	scale         scale.CatalogCache
	personality   typologymodel.CatalogCache
}

func (c *Container) initCatalogCaches() catalogCaches {
	var caches catalogCaches
	if cache := newCatalogL1Cache(c.opts, catalogKindQuestionnaire); cache != nil {
		caches.questionnaire = cache.(questionnaire.PublishedDetailCache)
		c.startCatalogSignalWatcher(catalogKindQuestionnaire, cache)
	}
	if cache := newCatalogL1Cache(c.opts, catalogKindScale); cache != nil {
		caches.scale = cache.(scale.CatalogCache)
		c.startCatalogSignalWatcher(catalogKindScale, cache)
	}
	if cache := newCatalogL1Cache(c.opts, catalogKindPersonality); cache != nil {
		caches.personality = cache.(typologymodel.CatalogCache)
		c.startCatalogSignalWatcher(catalogKindPersonality, cache)
	}
	return caches
}

func (c *Container) cleanupCatalogCaches() {
	for _, cancel := range c.catalogCacheWatcherCancels {
		if cancel != nil {
			cancel()
		}
	}
	c.catalogCacheWatcherCancels = nil
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

func (c *Container) startCatalogSignalWatcher(kind catalogKind, cache any) {
	if c == nil || cache == nil {
		return
	}
	spec, ok := catalogSpecs[kind]
	if !ok || !catalogSignalEvictEnabled(catalogL1Config(c.opts, kind)) {
		return
	}
	switch kind {
	case catalogKindQuestionnaire:
		c.startQuestionnaireCacheSignalWatcher(cache.(questionnaire.PublishedDetailCache))
	case catalogKindScale:
		startCodeCatalogSignalWatcher(c, spec.watcherLabel, catalogL1Config(c.opts, kind), cache,
			cachesignal.NewScaleSignaler,
			func(ctx context.Context, signaler *signalredis.Signaler[cachesignal.ScaleCacheChangedSignal], target scale.CatalogCache) {
				scale.StartCacheSignalWatcher(ctx, signaler, target)
			},
			func(v any) scale.CatalogCache { return v.(scale.CatalogCache) },
		)
	case catalogKindPersonality:
		startCodeCatalogSignalWatcher(c, spec.watcherLabel, catalogL1Config(c.opts, kind), cache,
			cachesignal.NewPersonalityModelSignaler,
			func(ctx context.Context, signaler *signalredis.Signaler[cachesignal.PersonalityModelCacheChangedSignal], target typologymodel.CatalogCache) {
				typologymodel.StartCacheSignalWatcher(ctx, signaler, target)
			},
			func(v any) typologymodel.CatalogCache { return v.(typologymodel.CatalogCache) },
		)
	}
}

func startCodeCatalogSignalWatcher[T signaling.Signal, C any](
	c *Container,
	label string,
	cfg *options.CatalogL1CacheOptions,
	cache any,
	newSignaler func(goredis.UniversalClient, cachesignal.SignalingOptions) (*signalredis.Signaler[T], error),
	start func(context.Context, *signalredis.Signaler[T], C),
	cast func(any) C,
) {
	if !catalogSignalEvictEnabled(cfg) {
		return
	}
	standalone, sigCfg, ok := c.cacheSignalingStandalone(label)
	if !ok {
		return
	}
	signaler, err := newSignaler(standalone, sigCfg)
	if err != nil {
		log.Warnf("%s disabled: %v", label, err)
		return
	}
	watchCtx, cancel := context.WithCancel(context.Background())
	start(watchCtx, signaler, cast(cache))
	c.catalogCacheWatcherCancels = append(c.catalogCacheWatcherCancels, cancel)
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
	c.catalogCacheWatcherCancels = append(c.catalogCacheWatcherCancels, cancel)
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
