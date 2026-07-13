package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	redis "github.com/redis/go-redis/v9"
)

const (
	defaultPublishedModelCacheTTL         = 24 * time.Hour
	defaultNegativePublishedModelCacheTTL = 5 * time.Minute
)

// publishedModelInner is the delegate for non-cached published-model persistence.
type publishedModelInner interface {
	port.PublishedModelReader
	port.PublishedModelLister
	port.PublishedWriter
	port.PublishedAlgorithmLister
}

// CachedPublishedModelStore decorates PublishedStore with Redis read-through cache on submit hot paths.
type CachedPublishedModelStore struct {
	inner             publishedModelInner
	keys              *keyspace.Builder
	policy            cachepolicy.CachePolicy
	observer          *observability.ComponentObserver
	catalogList       *ObjectCacheStore[publishedModelCatalogListPage]
	catalogAlgorithms *ObjectCacheStore[publishedModelCatalogAlgorithms]
}

type publishedModelCatalogListPage struct {
	Models []*port.PublishedModel `json:"models"`
	Total  int64                  `json:"total"`
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
	redisCache := newRedisStoreIfAvailable(client)
	ttl := policy.TTLOr(defaultPublishedModelCacheTTL)
	negativeTTL := policy.NegativeTTLOr(defaultNegativePublishedModelCacheTTL)
	return &CachedPublishedModelStore{
		inner:    inner,
		keys:     builder,
		policy:   policy,
		observer: observer,
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

func (c *CachedPublishedModelStore) UpsertPublishedModel(ctx context.Context, model *port.PublishedModel) error {
	if err := c.inner.UpsertPublishedModel(ctx, model); err != nil {
		return err
	}
	c.invalidatePublishedModel(ctx, model)
	return nil
}

func (c *CachedPublishedModelStore) GetPublishedModelByRef(ctx context.Context, ref port.Ref) (*port.PublishedModel, error) {
	if c == nil || c.inner == nil {
		return nil, domain.ErrNotFound
	}
	return c.inner.GetPublishedModelByRef(ctx, ref)
}

func (c *CachedPublishedModelStore) FindPublishedModelByQuestionnaire(
	ctx context.Context,
	questionnaireCode, questionnaireVersion string,
) (*port.PublishedModel, error) {
	if c == nil || c.inner == nil {
		return nil, domain.ErrNotFound
	}
	return c.inner.FindPublishedModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
}

func (c *CachedPublishedModelStore) FindPublishedModelByCode(ctx context.Context, kind domain.Kind, code string) (*port.PublishedModel, error) {
	if c == nil || c.inner == nil {
		return nil, domain.ErrNotFound
	}
	return c.inner.FindPublishedModelByCode(ctx, kind, code)
}

func (c *CachedPublishedModelStore) ListPublishedModels(ctx context.Context, filter port.ListPublishedFilter) ([]*port.PublishedModel, int64, error) {
	if c == nil || c.inner == nil {
		return nil, 0, domain.ErrNotFound
	}
	if c.catalogList == nil || !c.catalogList.Available() {
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
	if c.catalogAlgorithms == nil || !c.catalogAlgorithms.Available() {
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
		Kind:    domain.KindTypology,
		Code:    "catalog-algorithms",
		Version: "all",
	})
}

func (c *CachedPublishedModelStore) refCacheKey(ref port.Ref) string {
	return c.keys.BuildPublishedAssessmentModelByRefKey(
		string(ref.Kind),
		string(ref.SubKind),
		string(ref.Algorithm),
		strings.ToLower(ref.Code),
		strings.ToLower(ref.Version),
	)
}

func (c *CachedPublishedModelStore) invalidatePublishedModel(ctx context.Context, model *port.PublishedModel) {
	if model == nil {
		return
	}
	if c.catalogAlgorithms != nil {
		if err := c.catalogAlgorithms.Delete(ctx, c.algorithmsCatalogCacheKey()); err != nil {
			logger.L(ctx).Warnw("failed to invalidate published model algorithms cache", "error", err)
		}
	}
	c.invalidateCatalogListCaches(ctx)
}

func (c *CachedPublishedModelStore) invalidateCatalogListCaches(ctx context.Context) {
	if c.catalogList == nil || !c.catalogList.Available() {
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

var (
	_ port.PublishedModelReader     = (*CachedPublishedModelStore)(nil)
	_ port.PublishedModelLister     = (*CachedPublishedModelStore)(nil)
	_ port.PublishedWriter          = (*CachedPublishedModelStore)(nil)
	_ port.PublishedAlgorithmLister = (*CachedPublishedModelStore)(nil)
)
