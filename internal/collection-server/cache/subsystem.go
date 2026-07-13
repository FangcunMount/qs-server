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
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/cache/signal"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	redis "github.com/redis/go-redis/v9"
)

const warmupTimeout = 30 * time.Second

// Subsystem owns collection-server catalog L1 caches, signal watchers, and
// startup warmup. Construction is side-effect free; Start owns all goroutines.
type Subsystem struct {
	config    Config
	opsHandle *redisruntime.Handle

	questionnaire questionnaire.PublishedDetailCache
	typology      typologymodel.CatalogCache
	warmup        *typologymodel.QueryService
	effective     *sharedcache.Registry

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
}

type CatalogBinding struct {
	Capability   sharedcache.Capability
	Source       string
	Enabled      bool
	Policy       sharedcache.Policy
	MaxEntries   int
	Singleflight bool
	SignalEvict  bool
}

type Config struct {
	Questionnaire   CatalogBinding
	Typology        CatalogBinding
	ReportStatusTTL time.Duration
	Signaling       SignalOptions
}

// SignalOptions controls collection-server's Redis Pub/Sub cache watchers.
type SignalOptions struct {
	Enabled    bool
	Prefix     string
	Channel    string
	BufferSize int
}

func (o SignalOptions) redisOptions() signalredis.Options {
	opts := signalredis.DefaultOptions()
	opts.Prefix = "qs:signal"
	if o.Prefix != "" {
		opts.Prefix = o.Prefix
	}
	if o.Channel != "" {
		opts.Channel = o.Channel
	}
	if o.BufferSize > 0 {
		opts.BufferSize = o.BufferSize
	}
	return opts
}

func NewSubsystem(config Config, opsHandle *redisruntime.Handle) *Subsystem {
	s := &Subsystem{config: config, opsHandle: opsHandle}
	if cfg := config.Questionnaire; cfg.Enabled {
		base := catalogcache.LocalTTLCacheOptions("questionnaire", cfg.Policy.TTL, cfg.MaxEntries, cfg.Policy.JitterRatio)
		s.questionnaire = questionnaire.NewLocalCache(questionnaire.LocalCacheOptions{
			TTL: base.TTL, MaxEntries: base.MaxEntries, TTLJitterRatio: base.TTLJitterRatio,
			OnHit: base.OnHit, OnMiss: base.OnMiss,
		})
	}
	if cfg := config.Typology; cfg.Enabled {
		base := catalogcache.LocalTTLCacheOptions("typology", cfg.Policy.TTL, cfg.MaxEntries, cfg.Policy.JitterRatio)
		s.typology = typologymodel.NewLocalCatalogCache(typologymodel.LocalCatalogCacheOptions{
			TTL: base.TTL, MaxEntries: base.MaxEntries, TTLJitterRatio: base.TTLJitterRatio,
			OnHit: base.OnHit, OnMiss: base.OnMiss,
		})
	}
	s.effective = buildEffectiveRegistry(config)
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
	return s != nil && s.config.Questionnaire.Singleflight
}

func (s *Subsystem) TypologySingleflight() bool {
	return s != nil && s.config.Typology.Singleflight
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
	cfg := s.config.Questionnaire
	if s.questionnaire == nil || !cfg.Enabled || !cfg.SignalEvict {
		return
	}
	client, signaling, ok := s.signaling()
	if !ok {
		return
	}
	signaler := signalredis.NewSignaler[cachesignal.QuestionnaireCacheChangedSignal](client, signaling)
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
	cfg := s.config.Typology
	if s.typology == nil || !cfg.Enabled || !cfg.SignalEvict {
		return
	}
	client, signaling, ok := s.signaling()
	if !ok {
		return
	}
	signaler := signalredis.NewSignaler[cachesignal.TypologyModelCacheChangedSignal](client, signaling)
	watchSignals(ctx, signaler, func(signal cachesignal.TypologyModelCacheChangedSignal) {
		if signal.Code != "" {
			s.typology.EvictOnSignal(signal.Code)
		}
	}, "typology model cache signal evicted")
}

func (s *Subsystem) signaling() (redis.UniversalClient, signalredis.Options, bool) {
	cfg := s.config.Signaling
	if !cfg.Enabled || s.opsHandle == nil || s.opsHandle.Client == nil {
		return nil, signalredis.Options{}, false
	}
	return s.opsHandle.Client, cfg.redisOptions(), true
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

func buildEffectiveRegistry(config Config) *sharedcache.Registry {
	configuredCapabilities := []CatalogBinding{config.Questionnaire, config.Typology}
	entries := make([]sharedcache.EffectiveCapability, 0, len(configuredCapabilities)+1)
	for _, item := range configuredCapabilities {
		entries = append(entries, sharedcache.EffectiveCapability{
			Capability: item.Capability, Owner: "collection", Kind: sharedcache.KindCache,
			Layer: sharedcache.LayerL1, Family: "local", Enabled: item.Enabled, Policy: item.Policy,
			Layers: sharedcache.PolicyLayers{Override: item.Policy},
			Source: item.Source, CatalogVersion: "v2", MetricLabel: string(item.Capability),
		})
	}
	if config.ReportStatusTTL > 0 {
		entries = append(entries, sharedcache.EffectiveCapability{
			Capability: "report_status", Owner: "interpretation", Kind: sharedcache.KindOperationalState,
			Layer: sharedcache.LayerRuntime, Family: "ops_runtime", Enabled: true,
			Policy: sharedcache.Policy{TTL: config.ReportStatusTTL},
			Source: "cache.capabilities.report_status", CatalogVersion: "v2", MetricLabel: "report_status",
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
