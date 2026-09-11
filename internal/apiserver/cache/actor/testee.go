package actorcache

import (
	"context"
	"encoding/json"
	"fmt"
	dbctx "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"sync/atomic"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/internal/adapterkit"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	testeeInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	redis "github.com/redis/go-redis/v9"
)

// CachedTesteeRepository 带缓存的受试者 Repository 装饰器
// 实现 testee.Repository 接口，在原有 Repository 基础上添加 Redis 缓存层
type CachedTesteeRepository struct {
	repo     testee.Repository
	keys     *keyspace.Builder
	policies sharedcache.PolicyProvider
	observer *observability.ComponentObserver
	store    *adapterkit.ObjectCacheStore[testee.Testee]
}

func NewCachedTesteeRepositoryWithBuilderProviderAndObserver(repo testee.Repository, client redis.UniversalClient, builder *keyspace.Builder, policies sharedcache.PolicyProvider, observer *observability.ComponentObserver) testee.Repository {
	if builder == nil {
		panic("redis builder is required")
	}
	mapper := testeeInfra.NewTesteeMapper()
	return &CachedTesteeRepository{
		repo:     repo,
		keys:     builder,
		policies: policies,
		observer: observer,
		store: adapterkit.NewObjectCacheStore(adapterkit.ObjectCacheStoreOptions[testee.Testee]{
			Cache:     adapterkit.NewRedisStoreIfAvailable(client),
			PolicyKey: cachepolicy.CapabilityActorTestee,
			Codec:     newTesteeCacheEntryCodec(mapper),
		}),
	}
}

func newTesteeCacheEntryCodec(mapper *testeeInfra.TesteeMapper) adapterkit.CacheEntryCodec[testee.Testee] {
	return adapterkit.CacheEntryCodec[testee.Testee]{
		EncodeFunc: func(domain *testee.Testee) ([]byte, error) {
			return json.Marshal(mapper.ToPO(domain))
		},
		DecodeFunc: func(data []byte) (*testee.Testee, error) {
			var po testeeInfra.TesteePO
			if err := json.Unmarshal(data, &po); err != nil {
				return nil, err
			}
			if po.StoreVersion == 0 {
				return nil, fmt.Errorf("testee cache lacks current ownership version")
			}
			return mapper.ToDomain(&po), nil
		},
	}
}

// buildCacheKey 构建缓存键
func (r *CachedTesteeRepository) buildCacheKey(id testee.ID) string {
	return r.keys.BuildTesteeInfoKey(id.Uint64())
}

// FindByID caches profile information; current store ownership always comes from storage.
func (r *CachedTesteeRepository) FindByID(ctx context.Context, id testee.ID) (*testee.Testee, error) {
	// Transactional decisions must observe the transaction, not cached facts.
	if _, ok := dbctx.TxFromContext(ctx); ok {
		return r.repo.FindByID(ctx, id)
	}
	var loaded atomic.Bool
	domain, err := adapterkit.ReadThroughObject(ctx, adapterkit.ObjectReadThroughOptions[testee.Testee]{
		PolicyKey:        cachepolicy.CapabilityActorTestee,
		CacheKey:         r.buildCacheKey(id),
		PolicyProvider:   r.policies,
		Observer:         r.observer,
		Store:            r.store,
		Load:             func(ctx context.Context) (*testee.Testee, error) { loaded.Store(true); return r.repo.FindByID(ctx, id) },
		CacheNegative:    true,
		AsyncSetCached:   true,
		AsyncSetNegative: true,
	})
	if err != nil {
		return nil, err
	}
	if domain != nil && !loaded.Load() {
		reader, ok := r.repo.(testee.OwnershipReader)
		if !ok {
			return r.repo.FindByID(ctx, id)
		}
		ownership, err := reader.FindCurrentOwnership(ctx, id)
		if err != nil {
			return nil, err
		}
		if ownership.Version == 0 || ownership.OrgID != domain.OrgID() {
			return nil, fmt.Errorf("invalid current testee ownership")
		}
		current := *domain
		current.RestoreStore(ownership.StoreID, ownership.Version)
		domain = &current
	}
	return domain, nil
}

// Save 保存受试者（同时失效缓存）
func (r *CachedTesteeRepository) Save(ctx context.Context, domain *testee.Testee) error {
	err := r.repo.Save(ctx, domain)
	if err == nil && domain != nil {
		_ = r.deleteCache(ctx, domain.ID())
	}
	return err
}

// Update 更新受试者（同时失效缓存）
func (r *CachedTesteeRepository) Update(ctx context.Context, domain *testee.Testee) error {
	err := r.repo.Update(ctx, domain)
	if err == nil && domain != nil {
		_ = r.deleteCache(ctx, domain.ID())
	}
	return err
}

// Delete 删除受试者（同时失效缓存）
func (r *CachedTesteeRepository) Delete(ctx context.Context, id testee.ID) error {
	err := r.repo.Delete(ctx, id)
	if err == nil {
		_ = r.deleteCache(ctx, id)
	}
	return err
}

// deleteCache 删除缓存
func (r *CachedTesteeRepository) deleteCache(ctx context.Context, id testee.ID) error {
	return r.store.Delete(ctx, r.buildCacheKey(id))
}

// 实现其他 Repository 方法（透传，不缓存）
func (r *CachedTesteeRepository) FindByProfile(ctx context.Context, orgID int64, profileID uint64) (*testee.Testee, error) {
	return r.repo.FindByProfile(ctx, orgID, profileID)
}
