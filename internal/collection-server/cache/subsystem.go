// Package cache owns collection-server's process-local catalog cache lifecycle.
package cache

import (
	"context"
	"sync"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/signaling"
	signalredis "github.com/FangcunMount/component-base/pkg/signaling/redis"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/catalogcache"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/cachesignal"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	redis "github.com/redis/go-redis/v9"
)

const warmupTimeout = 30 * time.Second

// Subsystem owns collection-server catalog L1 caches, signal watchers, and
// startup warmup. Construction is side-effect free; Start owns all goroutines.
type Subsystem struct {
	opts      *options.Options
	opsHandle *redisruntime.Handle

	questionnaire questionnaire.PublishedDetailCache
	typology      typologymodel.CatalogCache
	warmup        *typologymodel.QueryService
	effective     *sharedcache.Registry

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
}

func NewSubsystem(opts *options.Options, opsHandle *redisruntime.Handle) *Subsystem {
	s := &Subsystem{opts: opts, opsHandle: opsHandle}
	if cfg := questionnaireConfig(opts); enabled(cfg) {
		base := catalogcache.LocalTTLCacheOptions("questionnaire", time.Duration(cfg.TTLSeconds)*time.Second, cfg.MaxEntries, cfg.TTLJitterRatio)
		s.questionnaire = questionnaire.NewLocalCache(questionnaire.LocalCacheOptions{
			TTL: base.TTL, MaxEntries: base.MaxEntries, TTLJitterRatio: base.TTLJitterRatio,
			OnHit: base.OnHit, OnMiss: base.OnMiss,
		})
	}
	if cfg := typologyConfig(opts); enabled(cfg) {
		base := catalogcache.LocalTTLCacheOptions("typology", time.Duration(cfg.TTLSeconds)*time.Second, cfg.MaxEntries, cfg.TTLJitterRatio)
		s.typology = typologymodel.NewLocalCatalogCache(typologymodel.LocalCatalogCacheOptions{
			TTL: base.TTL, MaxEntries: base.MaxEntries, TTLJitterRatio: base.TTLJitterRatio,
			OnHit: base.OnHit, OnMiss: base.OnMiss,
		})
	}
	s.effective = buildEffectiveRegistry(opts)
	return s
}

func (s *Subsystem) Questionnaire() questionnaire.PublishedDetailCache {
	if s == nil {
		return nil
	}
	return s.questionnaire
}

func (s *Subsystem) Typology() typologymodel.CatalogCache {
	if s == nil {
		return nil
	}
	return s.typology
}

func (s *Subsystem) QuestionnaireSingleflight() bool {
	return s != nil && singleflight(questionnaireConfig(s.opts))
}

func (s *Subsystem) TypologySingleflight() bool {
	return s != nil && singleflight(typologyConfig(s.opts))
}

func (s *Subsystem) EffectiveRegistry() *sharedcache.Registry {
	if s == nil {
		return nil
	}
	return s.effective
}

func (s *Subsystem) BindWarmup(service *typologymodel.QueryService) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.warmup = service
	s.mu.Unlock()
}

// Start starts signal watchers and startup warmup once. Repeated calls are no-ops.
func (s *Subsystem) Start(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.started = true
	s.cancel = cancel
	warmup := s.warmup
	s.mu.Unlock()

	s.startQuestionnaireWatcher(runCtx)
	s.startTypologyWatcher(runCtx)
	if warmup != nil {
		go warmCatalog(runCtx, warmup)
	}
	return nil
}

// Close cancels all subsystem goroutines. Repeated calls are safe.
func (s *Subsystem) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.started = false
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (s *Subsystem) startQuestionnaireWatcher(ctx context.Context) {
	cfg := questionnaireConfig(s.opts)
	if s.questionnaire == nil || !signalEvict(cfg) {
		return
	}
	client, signaling, ok := s.signaling("questionnaire cache signal watcher")
	if !ok {
		return
	}
	signaler, err := cachesignal.NewQuestionnaireSignaler(client, signaling)
	if err != nil {
		log.Warnf("questionnaire cache signal watcher disabled: %v", err)
		return
	}
	watchSignals(ctx, signaler, func(signal cachesignal.QuestionnaireCacheChangedSignal) {
		if signal.Code == "" {
			return
		}
		if signal.Version == "" {
			s.questionnaire.Delete(signal.Code, "")
		} else {
			s.questionnaire.Delete(signal.Code, signal.Version)
			s.questionnaire.Delete(signal.Code, "")
		}
	}, "questionnaire cache signal evicted")
}

func (s *Subsystem) startTypologyWatcher(ctx context.Context) {
	cfg := typologyConfig(s.opts)
	if s.typology == nil || !signalEvict(cfg) {
		return
	}
	client, signaling, ok := s.signaling("typology model cache signal watcher")
	if !ok {
		return
	}
	signaler, err := cachesignal.NewTypologyModelSignaler(client, signaling)
	if err != nil {
		log.Warnf("typology model cache signal watcher disabled: %v", err)
		return
	}
	watchSignals(ctx, signaler, func(signal cachesignal.TypologyModelCacheChangedSignal) {
		if signal.Code != "" {
			s.typology.EvictOnSignal(signal.Code)
		}
	}, "typology model cache signal evicted")
}

func (s *Subsystem) signaling(label string) (*redis.Client, cachesignal.SignalingOptions, bool) {
	var sigOpts *genericoptions.SignalingOptions
	if s.opts != nil {
		sigOpts = s.opts.Signaling
	}
	cfg := cachesignal.ConfigFromOptions(sigOpts, "collection-server")
	if !cfg.Signaling.Enabled || s.opsHandle == nil || s.opsHandle.Client == nil {
		return nil, cachesignal.SignalingOptions{}, false
	}
	client, err := cachesignal.AsStandaloneClient(s.opsHandle.Client)
	if err != nil {
		log.Warnf("%s disabled: %v", label, err)
		return nil, cachesignal.SignalingOptions{}, false
	}
	return client, cfg.Signaling, true
}

func warmCatalog(ctx context.Context, service *typologymodel.QueryService) {
	ctx, cancel := context.WithTimeout(ctx, warmupTimeout)
	defer cancel()
	if _, err := service.List(ctx, &typologymodel.ListTypologyModelsRequest{Page: 1, PageSize: 20}); err != nil {
		log.Warnf("catalog warmup: personality list: %v", err)
	}
	if _, err := service.GetCategories(ctx); err != nil {
		log.Warnf("catalog warmup: personality categories: %v", err)
	}
	log.Info("catalog L1 warmup finished")
}

func questionnaireConfig(opts *options.Options) *options.CatalogL1CacheOptions {
	if opts == nil || opts.Cache == nil || opts.Cache.Capabilities == nil || opts.Cache.Capabilities.Catalog == nil || opts.Cache.Capabilities.Catalog.Questionnaire == nil {
		return nil
	}
	return &opts.Cache.Capabilities.Catalog.Questionnaire.CatalogL1CacheOptions
}

func typologyConfig(opts *options.Options) *options.CatalogL1CacheOptions {
	if opts == nil || opts.Cache == nil || opts.Cache.Capabilities == nil || opts.Cache.Capabilities.Catalog == nil || opts.Cache.Capabilities.Catalog.Typology == nil {
		return nil
	}
	return &opts.Cache.Capabilities.Catalog.Typology.CatalogL1CacheOptions
}

func enabled(cfg *options.CatalogL1CacheOptions) bool      { return cfg != nil && cfg.Enabled }
func singleflight(cfg *options.CatalogL1CacheOptions) bool { return cfg != nil && cfg.Singleflight }
func signalEvict(cfg *options.CatalogL1CacheOptions) bool {
	return enabled(cfg) && cfg.SignalEvictEnabled
}

func buildEffectiveRegistry(opts *options.Options) *sharedcache.Registry {
	type configured struct {
		id, source string
		cfg        *options.CatalogL1CacheOptions
	}
	configuredCapabilities := []configured{
		{"catalog.questionnaire", "cache.capabilities.catalog.questionnaire", questionnaireConfig(opts)},
		{"catalog.typology", "cache.capabilities.catalog.typology", typologyConfig(opts)},
	}
	entries := make([]sharedcache.EffectiveCapability, 0, len(configuredCapabilities))
	for _, item := range configuredCapabilities {
		policy := sharedcache.Policy{}
		if item.cfg != nil {
			policy.TTL = time.Duration(item.cfg.TTLSeconds) * time.Second
			policy.JitterRatio = item.cfg.TTLJitterRatio
			policy.Singleflight = sharedcache.PolicySwitchFromBool(item.cfg.Singleflight)
		}
		entries = append(entries, sharedcache.EffectiveCapability{
			Capability: sharedcache.Capability(item.id), Layer: sharedcache.LayerL1,
			Family: "local", Policy: policy, Source: item.source, Version: "v1",
		})
	}
	return sharedcache.NewRegistry(entries...)
}

func watchSignals[T signaling.Signal](ctx context.Context, signaler *signalredis.Signaler[T], evict func(T), label string) {
	if signaler == nil || evict == nil {
		return
	}
	go func() {
		for {
			err := signaler.Watch(ctx, func(msgCtx context.Context, signal T) {
				evict(signal)
				logger.L(msgCtx).Debugw(label)
			})
			if ctx.Err() != nil {
				return
			}
			logger.L(ctx).Errorw(label+" watcher stopped", "error", err)
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
}
