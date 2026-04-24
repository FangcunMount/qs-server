package cachehotset

import (
	"context"
	cryptorand "crypto/rand"
	"math/big"
	"strings"
	"sync"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	redis "github.com/redis/go-redis/v9"
)

const (
	defaultHotsetSampleRate = 0.1
	hotsetTrimEvery         = int64(20)
)

// FamilyObserver is the narrow observability surface required by the hotset store.
type FamilyObserver interface {
	ObserveFamilySuccess(family string)
	ObserveFamilyFailure(family string, err error)
}

// Options 定义热榜治理策略。
type Options struct {
	Enable          bool
	TopN            int64
	MaxItemsPerKind int64
}

var (
	hotsetRecordTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "qs_cache_hotset_records_total",
			Help: "Total number of hotset record attempts grouped by family, kind and result.",
		},
		[]string{"family", "kind", "result"},
	)
	warmupHotReadTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "qs_cache_warmup_hot_reads_total",
			Help: "Total number of hotset top-N reads grouped by family, kind and result.",
		},
		[]string{"family", "kind", "result"},
	)
)

// RedisStore 使用 meta_cache 的 ZSet 存储热点排行。
type RedisStore struct {
	client   redis.UniversalClient
	keys     *rediskey.Builder
	opts     Options
	observer FamilyObserver
	mu       sync.Mutex
	hits     map[string]int64
}

func NewRedisStore(client redis.UniversalClient, builder *rediskey.Builder, opts Options) cachetarget.HotsetRecorder {
	return NewRedisStoreWithObserver(client, builder, opts, nil)
}

func NewRedisStoreWithObserver(client redis.UniversalClient, builder *rediskey.Builder, opts Options, observer FamilyObserver) cachetarget.HotsetRecorder {
	if client == nil || builder == nil || !opts.Enable {
		return nil
	}
	if opts.TopN <= 0 {
		opts.TopN = 20
	}
	if opts.MaxItemsPerKind <= 0 {
		opts.MaxItemsPerKind = 200
	}
	return &RedisStore{
		client:   client,
		keys:     builder,
		opts:     opts,
		observer: observer,
		hits:     make(map[string]int64),
	}
}

var _ cachetarget.HotsetRecorder = (*RedisStore)(nil)
var _ cachetarget.HotsetInspector = (*RedisStore)(nil)

func (s *RedisStore) Record(ctx context.Context, target cachetarget.WarmupTarget) error {
	if s == nil || s.client == nil || target.Scope == "" {
		return nil
	}
	if cachetarget.HotsetRecordingSuppressed(ctx) {
		hotsetRecordTotal.WithLabelValues(string(target.Family), string(target.Kind), "suppressed").Inc()
		return nil
	}
	if !shouldSampleHotset(target) {
		hotsetRecordTotal.WithLabelValues(string(target.Family), string(target.Kind), "sampled_out").Inc()
		return nil
	}
	key := s.keys.BuildWarmupHotsetKey(string(target.Family), string(target.Kind))
	if err := s.client.ZIncrBy(ctx, key, 1, target.Scope).Err(); err != nil {
		hotsetRecordTotal.WithLabelValues(string(target.Family), string(target.Kind), "error").Inc()
		s.observeFamilyFailure(string(redisplane.FamilyMeta), err)
		return err
	}
	hotsetRecordTotal.WithLabelValues(string(target.Family), string(target.Kind), "ok").Inc()
	s.observeFamilySuccess(string(redisplane.FamilyMeta))
	if s.shouldMaintain(key) {
		s.observeAndTrim(ctx, key, target.Family, target.Kind)
	}
	return nil
}

func (s *RedisStore) Top(ctx context.Context, family redisplane.Family, kind cachetarget.WarmupKind, limit int64) ([]cachetarget.WarmupTarget, error) {
	items, err := s.TopWithScores(ctx, family, kind, limit)
	if err != nil {
		return nil, err
	}
	targets := make([]cachetarget.WarmupTarget, 0, len(items))
	for _, item := range items {
		targets = append(targets, item.Target)
	}
	return targets, nil
}

func (s *RedisStore) TopWithScores(ctx context.Context, family redisplane.Family, kind cachetarget.WarmupKind, limit int64) ([]cachetarget.HotsetItem, error) {
	if s == nil || s.client == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = s.opts.TopN
	}
	key := s.keys.BuildWarmupHotsetKey(string(family), string(kind))
	values, err := s.client.ZRevRangeWithScores(ctx, key, 0, limit-1).Result()
	if err != nil {
		warmupHotReadTotal.WithLabelValues(string(family), string(kind), "error").Inc()
		s.observeFamilyFailure(string(redisplane.FamilyMeta), err)
		return nil, err
	}
	warmupHotReadTotal.WithLabelValues(string(family), string(kind), "ok").Inc()
	s.observeFamilySuccess(string(redisplane.FamilyMeta))
	s.observeSize(ctx, key, family, kind)
	items := make([]cachetarget.HotsetItem, 0, len(values))
	for _, value := range values {
		scope, _ := value.Member.(string)
		if strings.TrimSpace(scope) == "" {
			continue
		}
		items = append(items, cachetarget.HotsetItem{
			Target: cachetarget.WarmupTarget{
				Family: family,
				Kind:   kind,
				Scope:  scope,
			},
			Score: value.Score,
		})
	}
	return items, nil
}

func (s *RedisStore) observeSize(ctx context.Context, key string, family redisplane.Family, kind cachetarget.WarmupKind) {
	if s == nil || s.client == nil {
		return
	}
	card, err := s.client.ZCard(ctx, key).Result()
	if err != nil {
		s.observeFamilyFailure(string(redisplane.FamilyMeta), err)
		return
	}
	s.observeFamilySuccess(string(redisplane.FamilyMeta))
	cacheobservability.SetHotsetSize(string(family), string(kind), card)
}

func (s *RedisStore) shouldMaintain(key string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hits[key]++
	return s.hits[key]%hotsetTrimEvery == 0
}

func (s *RedisStore) observeAndTrim(ctx context.Context, key string, family redisplane.Family, kind cachetarget.WarmupKind) {
	if s == nil || s.client == nil {
		return
	}
	card, err := s.client.ZCard(ctx, key).Result()
	if err != nil {
		s.observeFamilyFailure(string(redisplane.FamilyMeta), err)
		logger.L(ctx).Warnw("failed to inspect warmup hotset size",
			"family", family,
			"kind", kind,
			"error", err,
		)
		return
	}
	s.observeFamilySuccess(string(redisplane.FamilyMeta))
	cacheobservability.SetHotsetSize(string(family), string(kind), card)
	if s.opts.MaxItemsPerKind <= 0 || card <= s.opts.MaxItemsPerKind {
		return
	}
	removeUntil := card - s.opts.MaxItemsPerKind - 1
	if err := s.client.ZRemRangeByRank(ctx, key, 0, removeUntil).Err(); err != nil {
		s.observeFamilyFailure(string(redisplane.FamilyMeta), err)
		logger.L(ctx).Warnw("failed to trim warmup hotset",
			"family", family,
			"kind", kind,
			"error", err,
		)
		return
	}
	s.observeFamilySuccess(string(redisplane.FamilyMeta))
	s.observeSize(ctx, key, family, kind)
}

func (s *RedisStore) observeFamilySuccess(family string) {
	if s != nil && s.observer != nil {
		s.observer.ObserveFamilySuccess(family)
	}
}

func (s *RedisStore) observeFamilyFailure(family string, err error) {
	if s != nil && s.observer != nil {
		s.observer.ObserveFamilyFailure(family, err)
	}
}

func shouldSampleHotset(target cachetarget.WarmupTarget) bool {
	if target.Kind == cachetarget.WarmupKindStaticScaleList {
		return true
	}
	draw, err := cryptorand.Int(cryptorand.Reader, big.NewInt(10000))
	if err != nil {
		return false
	}
	return draw.Int64() < int64(defaultHotsetSampleRate*10000)
}
