package modelcatalogcache

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FangcunMount/component-base/pkg/logger"
	cachepolicy "github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	cachetarget "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/internal/adapterkit"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	querycache "github.com/FangcunMount/qs-server/internal/pkg/cache/query"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	redis "github.com/redis/go-redis/v9"
)

// publishedModelInner is the delegate for non-cached published-model persistence.
type publishedModelInner interface {
	port.PublishedModelReader
	port.PublishedModelLister
	port.PublishedWriter
	port.PublishedAlgorithmLister
}

// CachedPublishedModelStore decorates the unified snapshot repository with Redis read-through cache on submit hot paths.
type CachedPublishedModelStore struct {
	inner              publishedModelInner
	keys               *keyspace.Builder
	policies           sharedcache.PolicyProvider
	observer           *observability.ComponentObserver
	catalogList        *adapterkit.ObjectCacheStore[publishedModelCatalogListPage]
	catalogListVersion querycache.VersionTokenStore
	catalogAlgorithms  *adapterkit.ObjectCacheStore[publishedModelCatalogAlgorithms]
	latestByCode       *adapterkit.ObjectCacheStore[port.PublishedModel]
	exactByRef         *adapterkit.ObjectCacheStore[port.PublishedModel]
}

const publishedModelCatalogListVersionKind = "modelcatalog:published:list"

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
	policies sharedcache.PolicyProvider,
	observer *observability.ComponentObserver,
) *CachedPublishedModelStore {
	if builder == nil {
		panic("redis builder is required")
	}
	redisCache := adapterkit.NewRedisStoreIfAvailable(client)
	catalogListVersion := querycache.NewStaticVersionTokenStore(0)
	if client != nil {
		catalogListVersion = adapterkit.NewVersionTokenStore(client, cachepolicy.CapabilityModelCatalogPublished, observer)
	}
	return &CachedPublishedModelStore{
		inner:    inner,
		keys:     builder,
		policies: policies,
		observer: observer,
		catalogList: adapterkit.NewObjectCacheStore(adapterkit.ObjectCacheStoreOptions[publishedModelCatalogListPage]{
			Cache:     redisCache,
			PolicyKey: cachepolicy.CapabilityModelCatalogPublished,
			Codec:     newPublishedModelCatalogListCodec(),
		}),
		catalogListVersion: catalogListVersion,
		catalogAlgorithms: adapterkit.NewObjectCacheStore(adapterkit.ObjectCacheStoreOptions[publishedModelCatalogAlgorithms]{
			Cache:     redisCache,
			PolicyKey: cachepolicy.CapabilityModelCatalogPublished,
			Codec:     newPublishedModelCatalogAlgorithmsCodec(),
		}),
		latestByCode: adapterkit.NewObjectCacheStore(adapterkit.ObjectCacheStoreOptions[port.PublishedModel]{
			Cache: redisCache, PolicyKey: cachepolicy.CapabilityModelCatalogPublished,
			Codec: publishedModelCodec(),
		}),
		exactByRef: adapterkit.NewObjectCacheStore(adapterkit.ObjectCacheStoreOptions[port.PublishedModel]{
			Cache: redisCache, PolicyKey: cachepolicy.CapabilityModelCatalogPublished,
			Codec: publishedModelCodec(),
		}),
	}
}

func publishedModelCodec() adapterkit.CacheEntryCodec[port.PublishedModel] {
	return adapterkit.CacheEntryCodec[port.PublishedModel]{
		EncodeFunc: func(model *port.PublishedModel) ([]byte, error) { return json.Marshal(model) },
		DecodeFunc: func(data []byte) (*port.PublishedModel, error) {
			var model port.PublishedModel
			if err := json.Unmarshal(data, &model); err != nil {
				return nil, err
			}
			return &model, nil
		},
	}
}

func newPublishedModelCatalogListCodec() adapterkit.CacheEntryCodec[publishedModelCatalogListPage] {
	return adapterkit.CacheEntryCodec[publishedModelCatalogListPage]{
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

func newPublishedModelCatalogAlgorithmsCodec() adapterkit.CacheEntryCodec[publishedModelCatalogAlgorithms] {
	return adapterkit.CacheEntryCodec[publishedModelCatalogAlgorithms]{
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
	if c.exactByRef == nil || !c.exactByRef.Available() {
		return c.inner.GetPublishedModelByRef(ctx, ref)
	}
	return adapterkit.ReadThroughObject(ctx, adapterkit.ObjectReadThroughOptions[port.PublishedModel]{
		PolicyKey: cachepolicy.CapabilityModelCatalogPublished,
		CacheKey:  c.refCacheKey(ref), PolicyProvider: c.policies,
		Observer: c.observer, Store: c.exactByRef, AsyncSetCached: false,
		Load: func(loadCtx context.Context) (*port.PublishedModel, error) {
			return c.inner.GetPublishedModelByRef(loadCtx, ref)
		},
	})
}

// GetActivePublishedModelByRef deliberately bypasses exact-version payload
// caching: an immutable payload may remain cached after its release is
// archived, so admission must re-check active state against Mongo.
func (c *CachedPublishedModelStore) GetActivePublishedModelByRef(ctx context.Context, ref port.Ref) (*port.PublishedModel, error) {
	if c == nil || c.inner == nil {
		return nil, domain.ErrNotFound
	}
	reader, ok := c.inner.(port.ActivePublishedModelReader)
	if !ok {
		return nil, domain.ErrNotFound
	}
	return reader.GetActivePublishedModelByRef(ctx, ref)
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

func (c *CachedPublishedModelStore) ListPublishedReleaseHistory(ctx context.Context, code string) ([]*port.PublishedModel, error) {
	if c == nil || c.inner == nil {
		return nil, domain.ErrNotFound
	}
	reader, ok := c.inner.(port.PublishedReleaseHistoryReader)
	if !ok {
		return nil, domain.ErrNotFound
	}
	return reader.ListPublishedReleaseHistory(ctx, code)
}

func (c *CachedPublishedModelStore) FindPublishedModelByCode(ctx context.Context, kind domain.Kind, code string) (*port.PublishedModel, error) {
	if c == nil || c.inner == nil {
		return nil, domain.ErrNotFound
	}
	if c.latestByCode == nil || !c.latestByCode.Available() {
		return c.inner.FindPublishedModelByCode(ctx, kind, code)
	}
	return adapterkit.ReadThroughObject(ctx, adapterkit.ObjectReadThroughOptions[port.PublishedModel]{
		PolicyKey: cachepolicy.CapabilityModelCatalogPublished,
		CacheKey:  c.latestByCodeCacheKey(kind, code), PolicyProvider: c.policies,
		Observer: c.observer, Store: c.latestByCode, AsyncSetCached: false,
		Load: func(loadCtx context.Context) (*port.PublishedModel, error) {
			return c.inner.FindPublishedModelByCode(loadCtx, kind, code)
		},
	})
}

func (c *CachedPublishedModelStore) WarmByCode(ctx context.Context, kind cachetarget.WarmupKind, code string) error {
	if c == nil || c.inner == nil || c.latestByCode == nil || !c.latestByCode.Available() {
		return fmt.Errorf("%w: published model cache unavailable", cachetarget.ErrWarmupSkipped)
	}
	effective, ok := c.policies.Resolve(cachepolicy.CapabilityModelCatalogPublished)
	if !ok || !effective.Enabled {
		return fmt.Errorf("%w: modelcatalog.published_model disabled", cachetarget.ErrWarmupSkipped)
	}
	modelKind, ok := publishedModelKindForWarmup(kind)
	if !ok {
		return fmt.Errorf("unsupported published model warmup kind: %s", kind)
	}
	model, err := c.inner.FindPublishedModelByCode(ctx, modelKind, code)
	if err != nil {
		return err
	}
	if model == nil {
		return domain.ErrNotFound
	}
	key := c.latestByCodeCacheKey(modelKind, code)
	if err := c.latestByCode.Set(ctx, key, model, effective.Policy); err != nil {
		return err
	}
	exists, err := c.latestByCode.Exists(ctx, key)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("published model warmup entry is not visible")
	}
	readBack, err := c.latestByCode.Get(ctx, key)
	if err != nil {
		return err
	}
	if readBack == nil {
		return fmt.Errorf("published model warmup read-back is empty")
	}
	return nil
}

func publishedModelKindForWarmup(kind cachetarget.WarmupKind) (domain.Kind, bool) {
	switch kind {
	case cachetarget.WarmupKindStaticScale:
		return domain.KindScale, true
	case cachetarget.WarmupKindStaticTypologyModel:
		return domain.KindTypology, true
	default:
		return "", false
	}
}

func (c *CachedPublishedModelStore) latestByCodeCacheKey(kind domain.Kind, code string) string {
	return c.keys.BuildPublishedAssessmentModelLatestByCodeKey(string(kind), strings.ToLower(strings.TrimSpace(code)))
}

func (c *CachedPublishedModelStore) ListPublishedModels(ctx context.Context, filter port.ListPublishedFilter) ([]*port.PublishedModel, int64, error) {
	if c == nil || c.inner == nil {
		return nil, 0, domain.ErrNotFound
	}
	if c.catalogList == nil || !c.catalogList.Available() {
		return c.inner.ListPublishedModels(ctx, filter)
	}
	cacheKey, keyErr := c.listCatalogCacheKey(ctx, filter)
	if keyErr != nil {
		// A version-token read failure must degrade to the source of truth rather
		// than make a catalogue read unavailable.
		return c.inner.ListPublishedModels(ctx, filter)
	}
	page, err := adapterkit.ReadThroughObject(ctx, adapterkit.ObjectReadThroughOptions[publishedModelCatalogListPage]{
		PolicyKey:      cachepolicy.CapabilityModelCatalogPublished,
		CacheKey:       cacheKey,
		PolicyProvider: c.policies,
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
	payload, err := adapterkit.ReadThroughObject(ctx, adapterkit.ObjectReadThroughOptions[publishedModelCatalogAlgorithms]{
		PolicyKey:      cachepolicy.CapabilityModelCatalogPublished,
		CacheKey:       cacheKey,
		PolicyProvider: c.policies,
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

func (c *CachedPublishedModelStore) listCatalogCacheKey(ctx context.Context, filter port.ListPublishedFilter) (string, error) {
	version := uint64(0)
	if c != nil && c.catalogListVersion != nil {
		value, err := c.catalogListVersion.Current(ctx, c.catalogListVersionKey())
		if err != nil {
			return "", err
		}
		version = value
	}
	return c.listCatalogCacheKeyAtVersion(filter, version), nil
}

func (c *CachedPublishedModelStore) listCatalogCacheKeyAtVersion(filter port.ListPublishedFilter, catalogVersion uint64) string {
	// A catalog-list entry must vary by every query predicate and by the global
	// catalogue version. The version token invalidates every filtered list after
	// a publish without requiring Redis pattern deletion.
	raw := fmt.Sprintf(
		"code=%q&kind=%q&kinds=%q&algorithm=%q&category=%q&keyword=%q&questionnaire_code=%q&questionnaire_version=%q&page=%d&page_size=%d",
		filter.Code,
		filter.Kind,
		filter.Kinds,
		filter.Algorithm,
		filter.Category,
		filter.Keyword,
		filter.QuestionnaireCode,
		filter.QuestionnaireVersion,
		filter.Page,
		filter.PageSize,
	)
	hash := sha256.Sum256([]byte(raw))
	return c.refCacheKey(port.Ref{
		Code:    "catalog-list",
		Version: fmt.Sprintf("v3-%d-%x", catalogVersion, hash[:8]),
	})
}

func (c *CachedPublishedModelStore) catalogListVersionKey() string {
	return c.keys.BuildQueryVersionKey(publishedModelCatalogListVersionKind, "global")
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
	if c.latestByCode != nil {
		if err := c.latestByCode.Delete(ctx, c.latestByCodeCacheKey(model.Kind, model.Code)); err != nil {
			logger.L(ctx).Warnw("failed to invalidate latest published model cache", "error", err)
		}
	}
	// GetPublished is implemented as a code-filtered page with PageSize 100.
	// Clear its exact entry when a model is republished so a previous snapshot
	// cannot survive until the catalog-list TTL expires.
	c.invalidateCatalogListCache(ctx, port.ListPublishedFilter{Code: model.Code, Page: 1, PageSize: 100})
	c.invalidateCatalogListCaches(ctx)
}

// InvalidatePublishedModel removes only mutable-visibility caches. Immutable
// exact-version entries are deliberately retained. Callers invoke this after
// the Mongo transaction commits.
func (c *CachedPublishedModelStore) InvalidatePublishedModel(ctx context.Context, kind domain.Kind, code string) {
	if c == nil || code == "" {
		return
	}
	c.invalidatePublishedModel(ctx, &port.PublishedModel{Kind: kind, Code: code})
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
		c.invalidateCatalogListCache(ctx, filter)
	}
	if c.catalogListVersion == nil {
		return
	}
	if _, err := c.catalogListVersion.Bump(ctx, c.catalogListVersionKey()); err != nil {
		logger.L(ctx).Warnw("failed to invalidate published model catalog list version", "error", err)
	}
}

func (c *CachedPublishedModelStore) invalidateCatalogListCache(ctx context.Context, filter port.ListPublishedFilter) {
	if c.catalogList == nil || !c.catalogList.Available() {
		return
	}
	key, err := c.listCatalogCacheKey(ctx, filter)
	if err != nil {
		logger.L(ctx).Warnw("failed to resolve published model catalog list cache key", "error", err)
		return
	}
	if err := c.catalogList.Delete(ctx, key); err != nil {
		logger.L(ctx).Warnw("failed to invalidate published model catalog list cache",
			"key", key,
			"error", err,
		)
	}
}

var (
	_ port.PublishedModelReader          = (*CachedPublishedModelStore)(nil)
	_ port.ActivePublishedModelReader    = (*CachedPublishedModelStore)(nil)
	_ port.PublishedReleaseHistoryReader = (*CachedPublishedModelStore)(nil)
	_ port.PublishedModelLister          = (*CachedPublishedModelStore)(nil)
	_ port.PublishedWriter               = (*CachedPublishedModelStore)(nil)
	_ port.PublishedAlgorithmLister      = (*CachedPublishedModelStore)(nil)
	_ cachetarget.PublishedModelWarmer   = (*CachedPublishedModelStore)(nil)
)
