package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	aminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
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
	inner             publishedModelInner
	keys              *keyspace.Builder
	policy            cachepolicy.CachePolicy
	observer          *observability.ComponentObserver
	store             *ObjectCacheStore[domain.Snapshot]
	catalogList       *ObjectCacheStore[publishedModelCatalogListPage]
	catalogAlgorithms *ObjectCacheStore[publishedModelCatalogAlgorithms]
}

type publishedModelCatalogListPage struct {
	Models []*domain.PublishedModelSnapshot `json:"models"`
	Total  int64                            `json:"total"`
}

type publishedModelCatalogAlgorithms struct {
	Algorithms []domain.Algorithm `json:"algorithms"`
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
	redisCache := newRedisCacheIfAvailable(client)
	ttl := policy.TTLOr(defaultPublishedModelCacheTTL)
	negativeTTL := policy.NegativeTTLOr(defaultNegativePublishedModelCacheTTL)
	return &CachedPublishedModelStore{
		inner:    inner,
		keys:     builder,
		policy:   policy,
		observer: observer,
		store: NewObjectCacheStore(ObjectCacheStoreOptions[domain.Snapshot]{
			Cache:       redisCache,
			PolicyKey:   cachepolicy.PolicyPublishedModel,
			Policy:      policy,
			TTL:         ttl,
			NegativeTTL: negativeTTL,
			Codec:       newPublishedModelCacheEntryCodec(),
		}),
		catalogList: NewObjectCacheStore(ObjectCacheStoreOptions[publishedModelCatalogListPage]{
			Cache:       redisCache,
			PolicyKey:   cachepolicy.PolicyPublishedModel,
			Policy:      policy,
			TTL:         ttl,
			NegativeTTL: negativeTTL,
			Codec:       newPublishedModelCatalogListCodec(),
		}),
		catalogAlgorithms: NewObjectCacheStore(ObjectCacheStoreOptions[publishedModelCatalogAlgorithms]{
			Cache:       redisCache,
			PolicyKey:   cachepolicy.PolicyPublishedModel,
			Policy:      policy,
			TTL:         ttl,
			NegativeTTL: negativeTTL,
			Codec:       newPublishedModelCatalogAlgorithmsCodec(),
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

func newPublishedModelCatalogListCodec() CacheEntryCodec[publishedModelCatalogListPage] {
	return CacheEntryCodec[publishedModelCatalogListPage]{
		EncodeFunc: func(page *publishedModelCatalogListPage) ([]byte, error) {
			return json.Marshal(page)
		},
		DecodeFunc: func(data []byte) (*publishedModelCatalogListPage, error) {
			var page publishedModelCatalogListPage
			if err := json.Unmarshal(data, &page); err != nil {
				return nil, err
			}
			return &page, nil
		},
	}
}

func newPublishedModelCatalogAlgorithmsCodec() CacheEntryCodec[publishedModelCatalogAlgorithms] {
	return CacheEntryCodec[publishedModelCatalogAlgorithms]{
		EncodeFunc: func(payload *publishedModelCatalogAlgorithms) ([]byte, error) {
			return json.Marshal(payload)
		},
		DecodeFunc: func(data []byte) (*publishedModelCatalogAlgorithms, error) {
			var payload publishedModelCatalogAlgorithms
			if err := json.Unmarshal(data, &payload); err != nil {
				return nil, err
			}
			return &payload, nil
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
	if c == nil || c.inner == nil {
		return nil, domain.ErrNotFound
	}
	if !c.store.available() {
		return c.inner.FindPublishedByModelCode(ctx, kind, code)
	}
	cacheKey := c.modelByCodeCacheKey(kind, code)
	return c.readThrough(ctx, cacheKey, func(ctx context.Context) (*domain.Snapshot, error) {
		snapshot, err := c.inner.FindPublishedByModelCode(ctx, kind, code)
		if domain.IsNotFound(err) {
			return nil, nil
		}
		return snapshot, err
	})
}

func (c *CachedPublishedModelStore) FindPublishedModelByCode(ctx context.Context, kind domain.Kind, code string) (*domain.PublishedModelSnapshot, error) {
	if c == nil || c.inner == nil {
		return nil, domain.ErrNotFound
	}
	if !c.store.available() {
		return c.inner.FindPublishedModelByCode(ctx, kind, code)
	}
	cacheKey := c.modelByCodeCacheKey(kind, code)
	snapshot, err := c.readThrough(ctx, cacheKey, func(ctx context.Context) (*domain.Snapshot, error) {
		published, loadErr := c.inner.FindPublishedModelByCode(ctx, kind, code)
		if loadErr != nil {
			if domain.IsNotFound(loadErr) {
				return nil, nil
			}
			return nil, loadErr
		}
		if published == nil {
			return nil, nil
		}
		return domain.LegacyFromPublished(published), nil
	})
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, nil
	}
	return domain.PublishedFromLegacy(snapshot), nil
}

func (c *CachedPublishedModelStore) ListPublished(ctx context.Context, filter port.ListPublishedFilter) ([]*domain.Snapshot, int64, error) {
	return c.inner.ListPublished(ctx, filter)
}

func (c *CachedPublishedModelStore) ListPublishedModels(ctx context.Context, filter port.ListPublishedFilter) ([]*domain.PublishedModelSnapshot, int64, error) {
	if c == nil || c.inner == nil {
		return nil, 0, domain.ErrNotFound
	}
	if c.catalogList == nil || !c.catalogList.available() {
		return c.inner.ListPublishedModels(ctx, filter)
	}
	cacheKey := c.listCatalogCacheKey(filter)
	page, err := ReadThroughObject(ctx, ObjectReadThroughOptions[publishedModelCatalogListPage]{
		PolicyKey:      cachepolicy.PolicyPublishedModel,
		CacheKey:       cacheKey,
		Policy:         c.policy,
		Observer:       c.observer,
		Store:          c.catalogList,
		CacheNegative:  false,
		AsyncSetCached: true,
		Load: func(ctx context.Context) (*publishedModelCatalogListPage, error) {
			models, total, loadErr := c.inner.ListPublishedModels(ctx, filter)
			if loadErr != nil {
				return nil, loadErr
			}
			return &publishedModelCatalogListPage{Models: models, Total: total}, nil
		},
	})
	if err != nil {
		return nil, 0, err
	}
	if page == nil {
		return nil, 0, nil
	}
	return page.Models, page.Total, nil
}

func (c *CachedPublishedModelStore) ListPublishedAlgorithms(ctx context.Context) ([]domain.Algorithm, error) {
	if c == nil || c.inner == nil {
		return nil, domain.ErrNotFound
	}
	if c.catalogAlgorithms == nil || !c.catalogAlgorithms.available() {
		return c.inner.ListPublishedAlgorithms(ctx)
	}
	cacheKey := c.algorithmsCatalogCacheKey()
	payload, err := ReadThroughObject(ctx, ObjectReadThroughOptions[publishedModelCatalogAlgorithms]{
		PolicyKey:      cachepolicy.PolicyPublishedModel,
		CacheKey:       cacheKey,
		Policy:         c.policy,
		Observer:       c.observer,
		Store:          c.catalogAlgorithms,
		CacheNegative:  false,
		AsyncSetCached: true,
		Load: func(ctx context.Context) (*publishedModelCatalogAlgorithms, error) {
			algorithms, loadErr := c.inner.ListPublishedAlgorithms(ctx)
			if loadErr != nil {
				return nil, loadErr
			}
			return &publishedModelCatalogAlgorithms{Algorithms: algorithms}, nil
		},
	})
	if err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, nil
	}
	return payload.Algorithms, nil
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

func (c *CachedPublishedModelStore) modelByCodeCacheKey(kind domain.Kind, code string) string {
	return c.refCacheKey(port.Ref{
		Kind:    kind,
		Code:    strings.ToLower(strings.TrimSpace(code)),
		Version: "latest",
	})
}

func (c *CachedPublishedModelStore) listCatalogCacheKey(filter port.ListPublishedFilter) string {
	return c.refCacheKey(port.Ref{
		Kind:      filter.Kind,
		SubKind:   filter.SubKind,
		Algorithm: filter.Algorithm,
		Code:      "catalog-list",
		Version: fmt.Sprintf(
			"p%d-ps%d-c%s",
			filter.Page,
			filter.PageSize,
			strings.ToLower(strings.TrimSpace(filter.Category)),
		),
	})
}

func (c *CachedPublishedModelStore) algorithmsCatalogCacheKey() string {
	return c.refCacheKey(port.Ref{
		Kind:    domain.KindPersonality,
		Code:    "catalog-algorithms",
		Version: "all",
	})
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
		c.modelByCodeCacheKey(snapshot.Definition.Kind, snapshot.Definition.Code),
		c.algorithmsCatalogCacheKey(),
	}
	for _, key := range keys {
		if err := c.store.Delete(ctx, key); err != nil {
			logger.L(ctx).Warnw("failed to invalidate published model cache",
				"key", key,
				"error", err,
			)
		}
	}
	if c.catalogAlgorithms != nil {
		if err := c.catalogAlgorithms.Delete(ctx, c.algorithmsCatalogCacheKey()); err != nil {
			logger.L(ctx).Warnw("failed to invalidate published model algorithms cache", "error", err)
		}
	}
	c.invalidateCatalogListCaches(ctx)
}

func (c *CachedPublishedModelStore) invalidateCatalogListCaches(ctx context.Context) {
	if c.catalogList == nil || !c.catalogList.available() {
		return
	}
	filters := []port.ListPublishedFilter{
		{},
		{Page: 1, PageSize: 20},
		{Page: 1, PageSize: 50},
	}
	for _, filter := range filters {
		if err := c.catalogList.Delete(ctx, c.listCatalogCacheKey(filter)); err != nil {
			logger.L(ctx).Warnw("failed to invalidate published model catalog list cache",
				"key", c.listCatalogCacheKey(filter),
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
