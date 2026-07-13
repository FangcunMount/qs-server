package plancache

import (
	"context"
	"encoding/json"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/internal/adapterkit"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	planInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	redis "github.com/redis/go-redis/v9"
)

// CachedPlanRepository 带缓存的计划 Repository 装饰器
// 实现 plan.AssessmentPlanRepository 接口，在原有 Repository 基础上添加 Redis 缓存层
type CachedPlanRepository struct {
	repo     plan.AssessmentPlanRepository
	keys     *keyspace.Builder
	policies sharedcache.PolicyProvider
	observer *observability.ComponentObserver
	store    *adapterkit.ObjectCacheStore[plan.AssessmentPlan]
}

// NewCachedPlanRepositoryWithBuilderAndPolicy 创建带显式 builder/policy 的计划缓存 Repository。
func NewCachedPlanRepositoryWithBuilderAndProvider(repo plan.AssessmentPlanRepository, client redis.UniversalClient, builder *keyspace.Builder, policies sharedcache.PolicyProvider) plan.AssessmentPlanRepository {
	return NewCachedPlanRepositoryWithBuilderProviderAndObserver(repo, client, builder, policies, nil)
}

func NewCachedPlanRepositoryWithBuilderProviderAndObserver(repo plan.AssessmentPlanRepository, client redis.UniversalClient, builder *keyspace.Builder, policies sharedcache.PolicyProvider, observer *observability.ComponentObserver) plan.AssessmentPlanRepository {
	if builder == nil {
		panic("redis builder is required")
	}
	mapper := planInfra.NewPlanMapper()
	return &CachedPlanRepository{
		repo:     repo,
		keys:     builder,
		policies: policies,
		observer: observer,
		store: adapterkit.NewObjectCacheStore(adapterkit.ObjectCacheStoreOptions[plan.AssessmentPlan]{
			Cache:     adapterkit.NewRedisStoreIfAvailable(client),
			PolicyKey: cachepolicy.CapabilityPlanDetail,
			Codec:     newPlanCacheEntryCodec(mapper),
		}),
	}
}

func newPlanCacheEntryCodec(mapper *planInfra.PlanMapper) adapterkit.CacheEntryCodec[plan.AssessmentPlan] {
	return adapterkit.CacheEntryCodec[plan.AssessmentPlan]{
		EncodeFunc: func(domain *plan.AssessmentPlan) ([]byte, error) {
			return json.Marshal(mapper.ToPO(domain))
		},
		DecodeFunc: func(data []byte) (*plan.AssessmentPlan, error) {
			var po planInfra.AssessmentPlanPO
			if err := json.Unmarshal(data, &po); err != nil {
				return nil, err
			}
			return mapper.ToDomain(&po), nil
		},
	}
}

// buildCacheKey 构建缓存键
func (r *CachedPlanRepository) buildCacheKey(id plan.AssessmentPlanID) string {
	return r.keys.BuildPlanInfoKey(id.Uint64())
}

// FindByID 根据ID查询计划（优先从缓存读取）
func (r *CachedPlanRepository) FindByID(ctx context.Context, id plan.AssessmentPlanID) (*plan.AssessmentPlan, error) {
	domain, err := adapterkit.ReadThroughObject(ctx, adapterkit.ObjectReadThroughOptions[plan.AssessmentPlan]{
		PolicyKey:      cachepolicy.CapabilityPlanDetail,
		CacheKey:       r.buildCacheKey(id),
		PolicyProvider: r.policies,
		Observer:       r.observer,
		Store:          r.store,
		Load:           func(ctx context.Context) (*plan.AssessmentPlan, error) { return r.repo.FindByID(ctx, id) },
	})
	if err != nil {
		return nil, err
	}
	return domain, nil
}

// Save 保存计划（同时失效缓存）
func (r *CachedPlanRepository) Save(ctx context.Context, domain *plan.AssessmentPlan) error {
	err := r.repo.Save(ctx, domain)
	if err == nil && domain != nil {
		_ = r.deleteCache(ctx, domain.GetID())
	}
	return err
}

// deleteCache 删除缓存
func (r *CachedPlanRepository) deleteCache(ctx context.Context, id plan.AssessmentPlanID) error {
	return r.store.Delete(ctx, r.buildCacheKey(id))
}
