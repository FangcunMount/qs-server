package container

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
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

func newQuestionnaireDetailCache(opts *options.Options) questionnaire.PublishedDetailCache {
	if opts == nil || opts.QuestionnaireCache == nil || !opts.QuestionnaireCache.Enabled {
		return nil
	}
	cfg := opts.QuestionnaireCache
	return questionnaire.NewLocalCache(questionnaire.LocalCacheOptions{
		TTL:        time.Duration(cfg.TTLSeconds) * time.Second,
		MaxEntries: cfg.MaxEntries,
	})
}

func questionnaireCacheSingleflightEnabled(opts *options.Options) bool {
	if opts == nil || opts.QuestionnaireCache == nil {
		return false
	}
	return opts.QuestionnaireCache.Singleflight
}

func questionnaireCacheSignalEvictEnabled(opts *options.Options) bool {
	if opts == nil || opts.QuestionnaireCache == nil || !opts.QuestionnaireCache.Enabled {
		return false
	}
	return opts.QuestionnaireCache.SignalEvictEnabled
}

func (c *Container) startQuestionnaireCacheSignalWatcher(cache questionnaire.PublishedDetailCache) {
	if c == nil || cache == nil || !questionnaireCacheSignalEvictEnabled(c.opts) {
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

func newScaleCatalogCache(opts *options.Options) scale.CatalogCache {
	if opts == nil || opts.ScaleCache == nil || !opts.ScaleCache.Enabled {
		return nil
	}
	cfg := opts.ScaleCache
	return scale.NewLocalCatalogCache(scale.LocalCatalogCacheOptions{
		TTL:        time.Duration(cfg.TTLSeconds) * time.Second,
		MaxEntries: cfg.MaxEntries,
	})
}

func scaleCacheSingleflightEnabled(opts *options.Options) bool {
	if opts == nil || opts.ScaleCache == nil {
		return false
	}
	return opts.ScaleCache.Singleflight
}

func scaleCacheSignalEvictEnabled(opts *options.Options) bool {
	if opts == nil || opts.ScaleCache == nil || !opts.ScaleCache.Enabled {
		return false
	}
	return opts.ScaleCache.SignalEvictEnabled
}

func (c *Container) startScaleCacheSignalWatcher(cache scale.CatalogCache) {
	if c == nil || cache == nil || !scaleCacheSignalEvictEnabled(c.opts) {
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

func newPersonalityCatalogCache(opts *options.Options) personalitymodel.CatalogCache {
	if opts == nil || opts.PersonalityCache == nil || !opts.PersonalityCache.Enabled {
		return nil
	}
	cfg := opts.PersonalityCache
	return personalitymodel.NewLocalCatalogCache(personalitymodel.LocalCatalogCacheOptions{
		TTL:        time.Duration(cfg.TTLSeconds) * time.Second,
		MaxEntries: cfg.MaxEntries,
	})
}

func personalityCacheSingleflightEnabled(opts *options.Options) bool {
	if opts == nil || opts.PersonalityCache == nil {
		return false
	}
	return opts.PersonalityCache.Singleflight
}

func personalityCacheSignalEvictEnabled(opts *options.Options) bool {
	if opts == nil || opts.PersonalityCache == nil || !opts.PersonalityCache.Enabled {
		return false
	}
	return opts.PersonalityCache.SignalEvictEnabled
}

func (c *Container) startPersonalityCacheSignalWatcher(cache personalitymodel.CatalogCache) {
	if c == nil || cache == nil || !personalityCacheSignalEvictEnabled(c.opts) {
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
