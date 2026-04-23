package cache

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"sync"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	redis "github.com/redis/go-redis/v9"
)

type warmupContextKey string

const suppressHotsetRecordingKey warmupContextKey = "suppress-hotset-recording"

// WarmupKind 标识可治理的预热目标类型。
type WarmupKind string

const (
	WarmupKindStaticScale             WarmupKind = "static.scale"
	WarmupKindStaticQuestionnaire     WarmupKind = "static.questionnaire"
	WarmupKindStaticScaleList         WarmupKind = "static.scale_list"
	WarmupKindQueryStatsSystem        WarmupKind = "query.stats_system"
	WarmupKindQueryStatsQuestionnaire WarmupKind = "query.stats_questionnaire"
	WarmupKindQueryStatsPlan          WarmupKind = "query.stats_plan"
)

const scaleListWarmupScope = "published"

const (
	defaultHotsetSampleRate = 0.1
	hotsetTrimEvery         = int64(20)
)

// WarmupTarget 描述一个稳定的预热目标。
type WarmupTarget struct {
	Family redisplane.Family
	Kind   WarmupKind
	Scope  string
}

type HotsetItem struct {
	Target WarmupTarget `json:"target"`
	Score  float64      `json:"score"`
}

func (t WarmupTarget) Key() string {
	return fmt.Sprintf("%s|%s|%s", t.Family, t.Kind, t.Scope)
}

func normalizeCodeScope(prefix, code string) string {
	return prefix + ":" + strings.ToLower(strings.TrimSpace(code))
}

// NewStaticScaleWarmupTarget 创建量表静态缓存预热目标。
func NewStaticScaleWarmupTarget(code string) WarmupTarget {
	return WarmupTarget{
		Family: redisplane.FamilyStatic,
		Kind:   WarmupKindStaticScale,
		Scope:  normalizeCodeScope("scale", code),
	}
}

// NewStaticQuestionnaireWarmupTarget 创建问卷静态缓存预热目标。
func NewStaticQuestionnaireWarmupTarget(code string) WarmupTarget {
	return WarmupTarget{
		Family: redisplane.FamilyStatic,
		Kind:   WarmupKindStaticQuestionnaire,
		Scope:  normalizeCodeScope("questionnaire", code),
	}
}

// NewStaticScaleListWarmupTarget 创建量表列表预热目标。
func NewStaticScaleListWarmupTarget() WarmupTarget {
	return WarmupTarget{
		Family: redisplane.FamilyStatic,
		Kind:   WarmupKindStaticScaleList,
		Scope:  scaleListWarmupScope,
	}
}

// NewQueryStatsSystemWarmupTarget 创建系统统计查询预热目标。
func NewQueryStatsSystemWarmupTarget(orgID int64) WarmupTarget {
	return WarmupTarget{
		Family: redisplane.FamilyQuery,
		Kind:   WarmupKindQueryStatsSystem,
		Scope:  fmt.Sprintf("org:%d", orgID),
	}
}

// NewQueryStatsQuestionnaireWarmupTarget 创建问卷统计查询预热目标。
func NewQueryStatsQuestionnaireWarmupTarget(orgID int64, code string) WarmupTarget {
	return WarmupTarget{
		Family: redisplane.FamilyQuery,
		Kind:   WarmupKindQueryStatsQuestionnaire,
		Scope:  fmt.Sprintf("org:%d:questionnaire:%s", orgID, strings.ToLower(strings.TrimSpace(code))),
	}
}

// NewQueryStatsPlanWarmupTarget 创建计划统计查询预热目标。
func NewQueryStatsPlanWarmupTarget(orgID int64, planID uint64) WarmupTarget {
	return WarmupTarget{
		Family: redisplane.FamilyQuery,
		Kind:   WarmupKindQueryStatsPlan,
		Scope:  fmt.Sprintf("org:%d:plan:%d", orgID, planID),
	}
}

func ParseWarmupKind(raw string) (WarmupKind, bool) {
	switch WarmupKind(strings.TrimSpace(raw)) {
	case WarmupKindStaticScale,
		WarmupKindStaticQuestionnaire,
		WarmupKindStaticScaleList,
		WarmupKindQueryStatsSystem,
		WarmupKindQueryStatsQuestionnaire,
		WarmupKindQueryStatsPlan:
		return WarmupKind(strings.TrimSpace(raw)), true
	default:
		return "", false
	}
}

func ParseStaticScaleScope(scope string) (string, bool) {
	if !strings.HasPrefix(scope, "scale:") {
		return "", false
	}
	code := strings.TrimPrefix(scope, "scale:")
	return code, code != ""
}

func ParseStaticQuestionnaireScope(scope string) (string, bool) {
	if !strings.HasPrefix(scope, "questionnaire:") {
		return "", false
	}
	code := strings.TrimPrefix(scope, "questionnaire:")
	return code, code != ""
}

func ParseQueryStatsSystemScope(scope string) (int64, bool) {
	var orgID int64
	if _, err := fmt.Sscanf(scope, "org:%d", &orgID); err != nil || orgID == 0 {
		return 0, false
	}
	return orgID, true
}

func ParseQueryStatsQuestionnaireScope(scope string) (int64, string, bool) {
	var orgID int64
	var code string
	if _, err := fmt.Sscanf(scope, "org:%d:questionnaire:%s", &orgID, &code); err != nil || orgID == 0 || code == "" {
		return 0, "", false
	}
	return orgID, code, true
}

func ParseQueryStatsPlanScope(scope string) (int64, uint64, bool) {
	parts := strings.Split(scope, ":")
	if len(parts) != 4 || parts[0] != "org" || parts[2] != "plan" {
		return 0, 0, false
	}
	orgID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || orgID == 0 {
		return 0, 0, false
	}
	planID, err := strconv.ParseUint(parts[3], 10, 64)
	if err != nil || planID == 0 {
		return 0, 0, false
	}
	return orgID, planID, true
}

// SuppressHotsetRecording returns a context that prevents best-effort hotset writes.
func SuppressHotsetRecording(ctx context.Context) context.Context {
	return context.WithValue(ctx, suppressHotsetRecordingKey, true)
}

func hotsetRecordingSuppressed(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	suppressed, _ := ctx.Value(suppressHotsetRecordingKey).(bool)
	return suppressed
}

// HotsetRecorder 记录和读取内部热点排行榜。
type HotsetRecorder interface {
	Record(context.Context, WarmupTarget) error
	Top(context.Context, redisplane.Family, WarmupKind, int64) ([]WarmupTarget, error)
}

type HotsetInspector interface {
	TopWithScores(context.Context, redisplane.Family, WarmupKind, int64) ([]HotsetItem, error)
}

// HotsetOptions 定义热榜治理策略。
type HotsetOptions struct {
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

// RedisHotsetStore 使用 meta_cache 的 ZSet 存储热点排行。
type RedisHotsetStore struct {
	client   redis.UniversalClient
	keys     *rediskey.Builder
	opts     HotsetOptions
	observer *Observer
	mu       sync.Mutex
	hits     map[string]int64
}

func NewRedisHotsetStore(client redis.UniversalClient, builder *rediskey.Builder, opts HotsetOptions) HotsetRecorder {
	return NewRedisHotsetStoreWithObserver(client, builder, opts, nil)
}

func NewRedisHotsetStoreWithObserver(client redis.UniversalClient, builder *rediskey.Builder, opts HotsetOptions, observer *Observer) HotsetRecorder {
	if client == nil || builder == nil || !opts.Enable {
		return nil
	}
	if opts.TopN <= 0 {
		opts.TopN = 20
	}
	if opts.MaxItemsPerKind <= 0 {
		opts.MaxItemsPerKind = 200
	}
	return &RedisHotsetStore{
		client:   client,
		keys:     builder,
		opts:     opts,
		observer: observer,
		hits:     make(map[string]int64),
	}
}

func (s *RedisHotsetStore) Record(ctx context.Context, target WarmupTarget) error {
	if s == nil || s.client == nil || target.Scope == "" {
		return nil
	}
	if hotsetRecordingSuppressed(ctx) {
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
		s.observer.ObserveFamilyFailure(string(redisplane.FamilyMeta), err)
		return err
	}
	hotsetRecordTotal.WithLabelValues(string(target.Family), string(target.Kind), "ok").Inc()
	s.observer.ObserveFamilySuccess(string(redisplane.FamilyMeta))
	if s.shouldMaintain(key) {
		s.observeAndTrim(ctx, key, target.Family, target.Kind)
	}
	return nil
}

func (s *RedisHotsetStore) Top(ctx context.Context, family redisplane.Family, kind WarmupKind, limit int64) ([]WarmupTarget, error) {
	items, err := s.TopWithScores(ctx, family, kind, limit)
	if err != nil {
		return nil, err
	}
	targets := make([]WarmupTarget, 0, len(items))
	for _, item := range items {
		targets = append(targets, item.Target)
	}
	return targets, nil
}

func (s *RedisHotsetStore) TopWithScores(ctx context.Context, family redisplane.Family, kind WarmupKind, limit int64) ([]HotsetItem, error) {
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
		s.observer.ObserveFamilyFailure(string(redisplane.FamilyMeta), err)
		return nil, err
	}
	warmupHotReadTotal.WithLabelValues(string(family), string(kind), "ok").Inc()
	s.observer.ObserveFamilySuccess(string(redisplane.FamilyMeta))
	s.observeSize(ctx, key, family, kind)
	items := make([]HotsetItem, 0, len(values))
	for _, value := range values {
		scope, _ := value.Member.(string)
		if strings.TrimSpace(scope) == "" {
			continue
		}
		items = append(items, HotsetItem{
			Target: WarmupTarget{
				Family: family,
				Kind:   kind,
				Scope:  scope,
			},
			Score: value.Score,
		})
	}
	return items, nil
}

func (s *RedisHotsetStore) observeSize(ctx context.Context, key string, family redisplane.Family, kind WarmupKind) {
	if s == nil || s.client == nil {
		return
	}
	card, err := s.client.ZCard(ctx, key).Result()
	if err != nil {
		s.observer.ObserveFamilyFailure(string(redisplane.FamilyMeta), err)
		return
	}
	s.observer.ObserveFamilySuccess(string(redisplane.FamilyMeta))
	cacheobservability.SetHotsetSize(string(family), string(kind), card)
}

func (s *RedisHotsetStore) shouldMaintain(key string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hits[key]++
	return s.hits[key]%hotsetTrimEvery == 0
}

func (s *RedisHotsetStore) observeAndTrim(ctx context.Context, key string, family redisplane.Family, kind WarmupKind) {
	if s == nil || s.client == nil {
		return
	}
	card, err := s.client.ZCard(ctx, key).Result()
	if err != nil {
		s.observer.ObserveFamilyFailure(string(redisplane.FamilyMeta), err)
		logger.L(ctx).Warnw("failed to inspect warmup hotset size",
			"family", family,
			"kind", kind,
			"error", err,
		)
		return
	}
	s.observer.ObserveFamilySuccess(string(redisplane.FamilyMeta))
	cacheobservability.SetHotsetSize(string(family), string(kind), card)
	if s.opts.MaxItemsPerKind <= 0 || card <= s.opts.MaxItemsPerKind {
		return
	}
	removeUntil := card - s.opts.MaxItemsPerKind - 1
	if err := s.client.ZRemRangeByRank(ctx, key, 0, removeUntil).Err(); err != nil {
		s.observer.ObserveFamilyFailure(string(redisplane.FamilyMeta), err)
		logger.L(ctx).Warnw("failed to trim warmup hotset",
			"family", family,
			"kind", kind,
			"error", err,
		)
		return
	}
	s.observer.ObserveFamilySuccess(string(redisplane.FamilyMeta))
	s.observeSize(ctx, key, family, kind)
}

func shouldSampleHotset(target WarmupTarget) bool {
	if target.Kind == WarmupKindStaticScaleList {
		return true
	}
	draw, err := cryptorand.Int(cryptorand.Reader, big.NewInt(10000))
	if err != nil {
		return false
	}
	return draw.Int64() < int64(defaultHotsetSampleRate*10000)
}
