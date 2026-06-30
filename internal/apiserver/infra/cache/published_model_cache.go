package cache

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	aminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	redis "github.com/redis/go-redis/v9"
)

const (
	defaultPublishedModelCacheTTL         = 24 * time.Hour
	defaultNegativePublishedModelCacheTTL = 5 * time.Minute
)

// publishedModelInner is the delegate for non-cached published-model persistence.
type publishedModelInner interface {
	port.PublishedReader
	port.PublishedLister
	port.PublishedModelReader
	port.PublishedModelLister
	port.PublishedWriter
	port.PublishedAlgorithmLister
}

// CachedPublishedModelStore decorates DualStore with Redis read-through cache on submit hot paths.
type CachedPublishedModelStore struct {
	inner    publishedModelInner
	keys     *keyspace.Builder
	policy   cachepolicy.CachePolicy
	observer *observability.ComponentObserver
	store    *ObjectCacheStore[domain.Snapshot]
}

// NewCachedPublishedModelStore wraps a published-model store with Redis caching.
func NewCachedPublishedModelStore(
	inner publishedModelInner,
	client redis.UniversalClient,
	builder *keyspace.Builder,
	policy cachepolicy.CachePolicy,
	observer *observability.ComponentObserver,
) *CachedPublishedModelStore {
	if builder == nil {
		panic("redis builder is required")
	}
	return &CachedPublishedModelStore{
		inner:    inner,
		keys:     builder,
		policy:   policy,
		observer: observer,
		store: NewObjectCacheStore(ObjectCacheStoreOptions[domain.Snapshot]{
			Cache:       newRedisCacheIfAvailable(client),
			PolicyKey:   cachepolicy.PolicyPublishedModel,
			Policy:      policy,
			TTL:         policy.TTLOr(defaultPublishedModelCacheTTL),
			NegativeTTL: policy.NegativeTTLOr(defaultNegativePublishedModelCacheTTL),
			Codec:       newPublishedModelCacheEntryCodec(),
		}),
	}
}

func newPublishedModelCacheEntryCodec() CacheEntryCodec[domain.Snapshot] {
	return CacheEntryCodec[domain.Snapshot]{
		EncodeFunc: func(snapshot *domain.Snapshot) ([]byte, error) {
			return json.Marshal(snapshot)
		},
		DecodeFunc: func(data []byte) (*domain.Snapshot, error) {
			var snapshot domain.Snapshot
			if err := json.Unmarshal(data, &snapshot); err != nil {
				return nil, err
			}
			return &snapshot, nil
		},
	}
}

func (c *CachedPublishedModelStore) UpsertPublished(ctx context.Context, snapshot *domain.Snapshot) error {
	if err := c.inner.UpsertPublished(ctx, snapshot); err != nil {
		return err
	}
	c.invalidateSnapshot(ctx, snapshot)
	return nil
}

func (c *CachedPublishedModelStore) GetPublishedByRef(ctx context.Context, ref port.Ref) (*domain.Snapshot, error) {
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	cacheKey := c.refCacheKey(ref)
	snapshot, err := c.readThrough(ctx, cacheKey, func(ctx context.Context) (*domain.Snapshot, error) {
		snapshot, err := c.inner.GetPublishedByRef(ctx, ref)
		if domain.IsNotFound(err) {
			return nil, nil
		}
		return snapshot, err
	})
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, domain.ErrNotFound
	}
	c.warmQuestionnaireAlias(ctx, snapshot)
	return snapshot, nil
}

func (c *CachedPublishedModelStore) GetPublishedModelByRef(ctx context.Context, ref port.Ref) (*domain.PublishedModelSnapshot, error) {
	snapshot, err := c.GetPublishedByRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	return domain.PublishedFromLegacy(snapshot), nil
}

func (c *CachedPublishedModelStore) FindPublishedByQuestionnaire(
	ctx context.Context,
	questionnaireCode, questionnaireVersion string,
) (*domain.Snapshot, error) {
	cacheKey := c.questionnaireCacheKey(questionnaireCode, questionnaireVersion)
	snapshot, err := c.readThrough(ctx, cacheKey, func(ctx context.Context) (*domain.Snapshot, error) {
		snapshot, err := c.inner.FindPublishedByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
		if domain.IsNotFound(err) {
			return nil, nil
		}
		return snapshot, err
	})
	if err != nil || snapshot == nil {
		return snapshot, err
	}
	c.warmRefAlias(ctx, snapshot)
	return snapshot, nil
}

func (c *CachedPublishedModelStore) FindPublishedModelByQuestionnaire(
	ctx context.Context,
	questionnaireCode, questionnaireVersion string,
) (*domain.PublishedModelSnapshot, error) {
	snapshot, err := c.FindPublishedByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, nil
	}
	return domain.PublishedFromLegacy(snapshot), nil
}

func (c *CachedPublishedModelStore) FindPublishedByModelCode(ctx context.Context, kind domain.Kind, code string) (*domain.Snapshot, error) {
	return c.inner.FindPublishedByModelCode(ctx, kind, code)
}

func (c *CachedPublishedModelStore) FindPublishedModelByCode(ctx context.Context, kind domain.Kind, code string) (*domain.PublishedModelSnapshot, error) {
	return c.inner.FindPublishedModelByCode(ctx, kind, code)
}

func (c *CachedPublishedModelStore) ListPublished(ctx context.Context, filter port.ListPublishedFilter) ([]*domain.Snapshot, int64, error) {
	return c.inner.ListPublished(ctx, filter)
}

func (c *CachedPublishedModelStore) ListPublishedModels(ctx context.Context, filter port.ListPublishedFilter) ([]*domain.PublishedModelSnapshot, int64, error) {
	return c.inner.ListPublishedModels(ctx, filter)
}

func (c *CachedPublishedModelStore) ListPublishedAlgorithms(ctx context.Context) ([]domain.Algorithm, error) {
	return c.inner.ListPublishedAlgorithms(ctx)
}

func (c *CachedPublishedModelStore) readThrough(
	ctx context.Context,
	cacheKey string,
	load func(context.Context) (*domain.Snapshot, error),
) (*domain.Snapshot, error) {
	return ReadThroughObject(ctx, ObjectReadThroughOptions[domain.Snapshot]{
		PolicyKey:        cachepolicy.PolicyPublishedModel,
		CacheKey:         cacheKey,
		Policy:           c.policy,
		Observer:         c.observer,
		Store:            c.store,
		Load:             load,
		CacheNegative:    c.policy.NegativeEnabled(false),
		AsyncSetCached:   true,
		AsyncSetNegative: true,
	})
}

func (c *CachedPublishedModelStore) questionnaireCacheKey(questionnaireCode, questionnaireVersion string) string {
	return c.keys.BuildPublishedAssessmentModelByQuestionnaireKey(
		strings.ToLower(questionnaireCode),
		strings.ToLower(questionnaireVersion),
	)
}

func (c *CachedPublishedModelStore) refCacheKey(ref port.Ref) string {
	ref = canonicalPublishedModelRef(ref)
	return c.keys.BuildPublishedAssessmentModelByRefKey(
		string(ref.Kind),
		string(ref.SubKind),
		string(ref.Algorithm),
		strings.ToLower(ref.Code),
		strings.ToLower(ref.Version),
	)
}

func canonicalPublishedModelRef(ref port.Ref) port.Ref {
	if ref.SubKind == "" && ref.Algorithm == "" {
		if kind, subKind, algorithm, ok := domain.LegacyKindMapping(ref.Kind); ok {
			ref.Kind = kind
			ref.SubKind = subKind
			ref.Algorithm = algorithm
		}
	}
	return ref
}

func (c *CachedPublishedModelStore) invalidateSnapshot(ctx context.Context, snapshot *domain.Snapshot) {
	if !c.store.available() || snapshot == nil {
		return
	}
	keys := []string{
		c.questionnaireCacheKey(snapshot.Binding.QuestionnaireCode, snapshot.Binding.QuestionnaireVersion),
		c.questionnaireCacheKey(snapshot.Binding.QuestionnaireCode, ""),
		c.refCacheKey(aminfra.RefFromSnapshot(snapshot)),
	}
	for _, key := range keys {
		if err := c.store.Delete(ctx, key); err != nil {
			logger.L(ctx).Warnw("failed to invalidate published model cache",
				"key", key,
				"error", err,
			)
		}
	}
}

func (c *CachedPublishedModelStore) warmRefAlias(ctx context.Context, snapshot *domain.Snapshot) {
	if snapshot == nil || !c.store.available() {
		return
	}
	if err := c.store.Set(ctx, c.refCacheKey(aminfra.RefFromSnapshot(snapshot)), snapshot); err != nil {
		logger.L(ctx).Warnw("failed to warm published model ref cache alias", "error", err)
	}
}

func (c *CachedPublishedModelStore) warmQuestionnaireAlias(ctx context.Context, snapshot *domain.Snapshot) {
	if snapshot == nil || !c.store.available() {
		return
	}
	code := snapshot.Binding.QuestionnaireCode
	if code == "" {
		return
	}
	version := snapshot.Binding.QuestionnaireVersion
	if err := c.store.Set(ctx, c.questionnaireCacheKey(code, version), snapshot); err != nil {
		logger.L(ctx).Warnw("failed to warm published model questionnaire cache alias",
			"questionnaire_code", code,
			"questionnaire_version", version,
			"error", err,
		)
	}
	if version != "" {
		if err := c.store.Set(ctx, c.questionnaireCacheKey(code, ""), snapshot); err != nil {
			logger.L(ctx).Warnw("failed to warm published model questionnaire cache alias without version",
				"questionnaire_code", code,
				"error", err,
			)
		}
	}
}

var (
	_ port.PublishedReader          = (*CachedPublishedModelStore)(nil)
	_ port.PublishedLister          = (*CachedPublishedModelStore)(nil)
	_ port.PublishedModelReader     = (*CachedPublishedModelStore)(nil)
	_ port.PublishedModelLister     = (*CachedPublishedModelStore)(nil)
	_ port.PublishedWriter          = (*CachedPublishedModelStore)(nil)
	_ port.PublishedAlgorithmLister = (*CachedPublishedModelStore)(nil)
)
