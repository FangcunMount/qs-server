// Package cache owns collection-server's process-local L1 cache lifecycle.
package cache

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/signaling"
	signalredis "github.com/FangcunMount/component-base/pkg/signaling/redis"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/catalogcache"
	appevaluation "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	appmodelcatalog "github.com/FangcunMount/qs-server/internal/collection-server/application/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	sharedgovernance "github.com/FangcunMount/qs-server/internal/pkg/cache/governance"
	localcache "github.com/FangcunMount/qs-server/internal/pkg/cache/local"
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

	questionnaire    questionnaire.PublishedDetailCache
	publishedModel   appmodelcatalog.PublishedModelCache
	assessmentDetail appevaluation.AssessmentDetailCache
	assessmentAccess appevaluation.AssessmentAccessCache
	typology         typologymodel.CatalogCache
	warmup           *typologymodel.QueryService
	effective        *sharedcache.Registry

	runtimeMu      sync.RWMutex
	watcherRuntime map[sharedcache.Capability]sharedgovernance.SignalWatcherStatus

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
	Questionnaire    CatalogBinding
	PublishedModel   CatalogBinding
	Typology         CatalogBinding
	AssessmentDetail CatalogBinding
	AssessmentAccess CatalogBinding
	ReportStatusTTL  time.Duration
	Signaling        SignalOptions
	PolicySource     sharedcache.PolicySource
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
	s := &Subsystem{config: config, opsHandle: opsHandle, watcherRuntime: map[sharedcache.Capability]sharedgovernance.SignalWatcherStatus{}}
	for _, binding := range []CatalogBinding{config.Questionnaire, config.PublishedModel, config.Typology, config.AssessmentDetail, config.AssessmentAccess} {
		status := sharedgovernance.SignalWatcherStatus{Configured: binding.Enabled && binding.SignalEvict && config.Signaling.Enabled, Status: "disabled_by_policy"}
		if status.Configured {
			status.Status = "configured"
		}
		s.watcherRuntime[binding.Capability] = status
	}
	if cfg := config.Questionnaire; cfg.Enabled {
		base := catalogcache.LocalTTLCacheOptionsWithRuntime("questionnaire", string(cfg.Capability), "detail", cfg.Policy.TTL, cfg.MaxEntries, cfg.Policy.JitterRatio)
		s.questionnaire = questionnaire.NewLocalCache(questionnaire.LocalCacheOptions{
			TTL: base.TTL, MaxEntries: base.MaxEntries, TTLJitterRatio: base.TTLJitterRatio,
			OnHit: base.OnHit, OnMiss: base.OnMiss, OnEntries: base.OnEntries, OnEviction: base.OnEviction,
		})
	}
	if cfg := config.Typology; cfg.Enabled {
		buckets := localcache.MultiOptions{
			Detail:     catalogcache.LocalTTLCacheOptionsWithRuntime("typology", string(cfg.Capability), "detail", cfg.Policy.TTL, cfg.MaxEntries, cfg.Policy.JitterRatio),
			List:       catalogcache.LocalTTLCacheOptionsWithRuntime("typology", string(cfg.Capability), "list", cfg.Policy.TTL, cfg.MaxEntries, cfg.Policy.JitterRatio),
			Categories: catalogcache.LocalTTLCacheOptionsWithRuntime("typology", string(cfg.Capability), "categories", cfg.Policy.TTL, cfg.MaxEntries, cfg.Policy.JitterRatio),
		}
		s.typology = typologymodel.NewLocalCatalogCache(typologymodel.LocalCatalogCacheOptions{
			BucketOptions: &buckets,
		})
	}
	if cfg := config.PublishedModel; cfg.Enabled {
		s.publishedModel = appmodelcatalog.NewLocalPublishedModelCache(
			catalogcache.LocalTTLCacheOptionsWithRuntime(catalogcache.KindPublishedModelDetail, string(cfg.Capability), "detail", cfg.Policy.TTL, cfg.MaxEntries, cfg.Policy.JitterRatio),
			catalogcache.LocalTTLCacheOptionsWithRuntime(catalogcache.KindPublishedModelList, string(cfg.Capability), "list", cfg.Policy.TTL, cfg.MaxEntries, cfg.Policy.JitterRatio),
			catalogcache.LocalTTLCacheOptionsWithRuntime(catalogcache.KindPublishedModelOptions, string(cfg.Capability), "options", cfg.Policy.TTL, cfg.MaxEntries, cfg.Policy.JitterRatio),
		)
	}
	if cfg := config.AssessmentDetail; cfg.Enabled {
		s.assessmentDetail = appevaluation.NewLocalAssessmentDetailCache(
			catalogcache.LocalTTLCacheOptionsWithRuntime(catalogcache.KindAssessmentDetail, string(cfg.Capability), "detail", cfg.Policy.TTL, cfg.MaxEntries, cfg.Policy.JitterRatio),
		)
	}
	if cfg := config.AssessmentAccess; cfg.Enabled {
		s.assessmentAccess = appevaluation.NewLocalAssessmentAccessCache(
			catalogcache.LocalTTLCacheOptionsWithRuntime(catalogcache.KindAssessmentAccess, string(cfg.Capability), "access", cfg.Policy.TTL, cfg.MaxEntries, cfg.Policy.JitterRatio),
		)
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

func (s *Subsystem) PublishedModel() appmodelcatalog.PublishedModelCache {
	if s == nil {
		return nil
	}
	return s.publishedModel
}

func (s *Subsystem) AssessmentDetail() appevaluation.AssessmentDetailCache {
	if s == nil {
		return nil
	}
	return s.assessmentDetail
}

func (s *Subsystem) AssessmentAccess() appevaluation.AssessmentAccessCache {
	if s == nil {
		return nil
	}
	return s.assessmentAccess
}

func (s *Subsystem) QuestionnaireSingleflight() bool {
	return s != nil && s.config.Questionnaire.Singleflight
}

func (s *Subsystem) TypologySingleflight() bool {
	return s != nil && s.config.Typology.Singleflight
}

func (s *Subsystem) PublishedModelSingleflight() bool {
	return s != nil && s.config.PublishedModel.Singleflight
}

func (s *Subsystem) AssessmentDetailSingleflight() bool {
	return s != nil && s.config.AssessmentDetail.Singleflight
}

func (s *Subsystem) AssessmentAccessSingleflight() bool {
	return s != nil && s.config.AssessmentAccess.Singleflight
}

func (s *Subsystem) EffectiveRegistry() *sharedcache.Registry {
	if s == nil {
		return nil
	}
	return s.effective
}

type runtimeBucketReporter interface {
	RuntimeBuckets() []localcache.BucketSnapshot
}

// L1Runtime returns a point-in-time, read-only view of every collection L1
// capability. Disabled capabilities remain visible with no buckets.
func (s *Subsystem) L1Runtime() []sharedgovernance.L1CapabilityRuntime {
	if s == nil {
		return nil
	}
	items := []struct {
		binding CatalogBinding
		cache   any
	}{
		{s.config.Questionnaire, s.questionnaire},
		{s.config.PublishedModel, s.publishedModel},
		{s.config.Typology, s.typology},
		{s.config.AssessmentDetail, s.assessmentDetail},
		{s.config.AssessmentAccess, s.assessmentAccess},
	}
	result := make([]sharedgovernance.L1CapabilityRuntime, 0, len(items))
	for _, item := range items {
		runtime := sharedgovernance.L1CapabilityRuntime{
			Capability: string(item.binding.Capability), Enabled: item.binding.Enabled,
			Buckets: []sharedgovernance.L1BucketRuntime{}, SignalWatcher: s.signalWatcherSnapshot(item.binding.Capability),
		}
		if reporter, ok := item.cache.(runtimeBucketReporter); ok && reporter != nil {
			for _, bucket := range reporter.RuntimeBuckets() {
				stats := bucket.Stats
				runtime.Buckets = append(runtime.Buckets, sharedgovernance.L1BucketRuntime{
					Bucket: bucket.Bucket, Entries: stats.Entries, MaxEntries: stats.MaxEntries,
					Hits: stats.Hits, Misses: stats.Misses, FIFOEvictions: stats.FIFOEvictions,
					TTLExpirations: stats.TTLExpirations, ExplicitDeletions: stats.ExplicitDeletions,
					SignalDeletions: stats.SignalDeletions,
				})
			}
		}
		result = append(result, runtime)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Capability < result[j].Capability })
	return result
}

func (s *Subsystem) signalWatcherSnapshot(capability sharedcache.Capability) sharedgovernance.SignalWatcherStatus {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()
	return s.watcherRuntime[capability]
}

func (s *Subsystem) markWatcherRunning(capability sharedcache.Capability) {
	if s == nil || capability == "" {
		return
	}
	s.runtimeMu.Lock()
	status := s.watcherRuntime[capability]
	status.Configured = true
	status.Status = "running"
	status.LastError = ""
	s.watcherRuntime[capability] = status
	s.runtimeMu.Unlock()
	catalogcache.SetSignalWatcherUp(string(capability), true)
}

func (s *Subsystem) markSignalEviction(capability sharedcache.Capability) {
	if s == nil || capability == "" {
		return
	}
	now := time.Now()
	s.runtimeMu.Lock()
	status := s.watcherRuntime[capability]
	status.LastSignalAt = now
	status.LastEvictionAt = now
	s.watcherRuntime[capability] = status
	s.runtimeMu.Unlock()
	catalogcache.RecordSignalEviction(string(capability))
}

func (s *Subsystem) markWatcherError(capability sharedcache.Capability, message string) {
	if s == nil || capability == "" {
		return
	}
	s.runtimeMu.Lock()
	status := s.watcherRuntime[capability]
	if !status.Configured {
		s.runtimeMu.Unlock()
		return
	}
	status.Status = "reconnecting"
	status.LastErrorAt = time.Now()
	status.LastError = message
	status.ReconnectCount++
	s.watcherRuntime[capability] = status
	s.runtimeMu.Unlock()
	catalogcache.SetSignalWatcherUp(string(capability), false)
	catalogcache.RecordSignalWatcherError(string(capability))
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
	s.startPublishedModelWatcher(runCtx)
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
		s.markWatcherError(cfg.Capability, "signal transport unavailable")
		return
	}
	s.markWatcherRunning(cfg.Capability)
	signaler := signalredis.NewSignaler[cachesignal.QuestionnaireCacheChangedSignal](client, signaling)
	watchSignals(ctx, signaler, func(signal cachesignal.QuestionnaireCacheChangedSignal) {
		if signal.Code == "" {
			return
		}
		if cache, ok := s.questionnaire.(interface{ EvictOnSignal(string, string) }); ok {
			if signal.Version == "" {
				cache.EvictOnSignal(signal.Code, "")
			} else {
				cache.EvictOnSignal(signal.Code, signal.Version)
				cache.EvictOnSignal(signal.Code, "")
			}
			s.markSignalEviction(cfg.Capability)
			return
		}
		if signal.Version == "" {
			s.questionnaire.Delete(signal.Code, "")
		} else {
			s.questionnaire.Delete(signal.Code, signal.Version)
			s.questionnaire.Delete(signal.Code, "")
		}
		s.markSignalEviction(cfg.Capability)
	}, "questionnaire cache signal evicted", func() { s.markWatcherRunning(cfg.Capability) }, func(err error) { s.markWatcherError(cfg.Capability, err.Error()) })
}

func (s *Subsystem) startTypologyWatcher(ctx context.Context) {
	cfg := s.config.Typology
	if s.typology == nil || !cfg.Enabled || !cfg.SignalEvict {
		return
	}
	client, signaling, ok := s.signaling()
	if !ok {
		s.markWatcherError(cfg.Capability, "signal transport unavailable")
		return
	}
	s.markWatcherRunning(cfg.Capability)
	signaler := signalredis.NewSignaler[cachesignal.TypologyModelCacheChangedSignal](client, signaling)
	watchSignals(ctx, signaler, func(signal cachesignal.TypologyModelCacheChangedSignal) {
		if signal.Code != "" {
			s.typology.EvictOnSignal(signal.Code)
			s.markSignalEviction(cfg.Capability)
		}
	}, "typology model cache signal evicted", func() { s.markWatcherRunning(cfg.Capability) }, func(err error) { s.markWatcherError(cfg.Capability, err.Error()) })
}

func (s *Subsystem) startPublishedModelWatcher(ctx context.Context) {
	cfg := s.config.PublishedModel
	if s.publishedModel == nil || !cfg.Enabled || !cfg.SignalEvict {
		return
	}
	client, signaling, ok := s.signaling()
	if !ok {
		s.markWatcherError(cfg.Capability, "signal transport unavailable")
		return
	}
	s.markWatcherRunning(cfg.Capability)
	signaler := signalredis.NewSignaler[cachesignal.AssessmentModelCacheChangedSignal](client, signaling)
	watchSignals(ctx, signaler, func(changed cachesignal.AssessmentModelCacheChangedSignal) {
		if changed.Code == "" {
			return
		}
		s.evictPublishedModelOnSignal(changed)
		s.markSignalEviction(cfg.Capability)
	}, "published model cache signal evicted", func() { s.markWatcherRunning(cfg.Capability) }, func(err error) { s.markWatcherError(cfg.Capability, err.Error()) })
}

func (s *Subsystem) evictPublishedModelOnSignal(changed cachesignal.AssessmentModelCacheChangedSignal) {
	if s == nil || s.publishedModel == nil || changed.Code == "" {
		return
	}
	s.publishedModel.EvictOnSignal(changed.Code)
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
	configuredCapabilities := []CatalogBinding{config.Questionnaire, config.PublishedModel, config.Typology, config.AssessmentAccess, config.AssessmentDetail}
	entries := make([]sharedcache.EffectiveCapability, 0, len(configuredCapabilities)+1)
	for _, item := range configuredCapabilities {
		spec, ok := lookupCatalogSpec(item.Capability)
		if !ok {
			continue
		}
		entries = append(entries, sharedcache.EffectiveCapability{
			Capability: item.Capability, Owner: spec.Owner, Kind: sharedcache.KindCache,
			Layer: spec.Layer, Family: "local", Enabled: item.Enabled, Policy: item.Policy,
			Layers: sharedcache.PolicyLayers{Override: item.Policy},
			Source: item.Source, CatalogVersion: "v3", MetricLabel: spec.MetricLabel,
			TopologyGroup: spec.TopologyGroup, TopologyOrder: spec.TopologyOrder, ReadModel: spec.ReadModel,
		})
	}
	if config.ReportStatusTTL > 0 {
		entries = append(entries, sharedcache.EffectiveCapability{
			Capability: "report_status", Owner: "interpretation", Kind: sharedcache.KindOperationalState,
			Layer: sharedcache.LayerRuntime, Family: "ops_runtime", Enabled: true,
			Policy: sharedcache.Policy{TTL: config.ReportStatusTTL},
			Source: "runtime_state.report_status", CatalogVersion: "v3", MetricLabel: "report_status",
		})
	}
	if config.PolicySource.Component != "" {
		return sharedcache.NewRegistryWithSource(config.PolicySource, entries...)
	}
	return sharedcache.NewRegistry(entries...)
}

func watchSignals[T signaling.Signal](ctx context.Context, signaler *signalredis.Signaler[T], evict func(T), label string, onRunning func(), onError func(error)) {
	if signaler == nil || evict == nil {
		return
	}
	go func() {
		for {
			if onRunning != nil {
				onRunning()
			}
			err := signaler.Watch(ctx, func(msgCtx context.Context, signal T) {
				evict(signal)
				logger.L(msgCtx).Debugw(label)
			})
			if ctx.Err() != nil {
				return
			}
			if onError != nil && err != nil {
				onError(err)
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
