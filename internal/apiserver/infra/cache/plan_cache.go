package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	planInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	redis "github.com/redis/go-redis/v9"
)

const defaultPlanCacheTTL = 2 * time.Hour

// CachedPlanRepository 带缓存的计划 Repository 装饰器
// 实现 plan.AssessmentPlanRepository 接口，在原有 Repository 基础上添加 Redis 缓存层
type CachedPlanRepository struct {
	repo   plan.AssessmentPlanRepository
	client redis.UniversalClient
	ttl    time.Duration
	mapper *planInfra.PlanMapper
	keys   *rediskey.Builder
	policy cachepolicy.CachePolicy
}

// NewCachedPlanRepositoryWithBuilderAndPolicy 创建带显式 builder/policy 的计划缓存 Repository。
func NewCachedPlanRepositoryWithBuilderAndPolicy(repo plan.AssessmentPlanRepository, client redis.UniversalClient, builder *rediskey.Builder, policy cachepolicy.CachePolicy) plan.AssessmentPlanRepository {
	if builder == nil {
		panic("redis builder is required")
	}
	return &CachedPlanRepository{
		repo:   repo,
		client: client,
		ttl:    policy.TTLOr(defaultPlanCacheTTL),
		mapper: planInfra.NewPlanMapper(),
		keys:   builder,
		policy: policy,
	}
}

// buildCacheKey 构建缓存键
func (r *CachedPlanRepository) buildCacheKey(id plan.AssessmentPlanID) string {
	return r.keys.BuildPlanInfoKey(id.Uint64())
}

// FindByID 根据ID查询计划（优先从缓存读取）
func (r *CachedPlanRepository) FindByID(ctx context.Context, id plan.AssessmentPlanID) (*plan.AssessmentPlan, error) {
	domain, err := ReadThrough(ctx, ReadThroughOptions[plan.AssessmentPlan]{
		PolicyKey: cachepolicy.PolicyPlan,
		CacheKey:  r.buildCacheKey(id),
		Policy:    r.policy,
		GetCached: func(ctx context.Context) (*plan.AssessmentPlan, error) { return r.getCache(ctx, id) },
		Load:      func(ctx context.Context) (*plan.AssessmentPlan, error) { return r.repo.FindByID(ctx, id) },
		SetCached: func(ctx context.Context, value *plan.AssessmentPlan) error { return r.setCache(ctx, id, value) },
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
		r.deleteCache(ctx, domain.GetID())
	}
	return err
}

// getCache 从缓存获取
func (r *CachedPlanRepository) getCache(ctx context.Context, id plan.AssessmentPlanID) (*plan.AssessmentPlan, error) {
	if r.client == nil {
		return nil, ErrCacheNotFound
	}

	key := r.buildCacheKey(id)
	cachedData, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, ErrCacheNotFound
	}
	if err != nil {
		return nil, err
	}

	data := r.policy.DecompressValue(cachedData)
	observePayload(cachepolicy.PolicyPlan, len(data), len(cachedData))
	var po planInfra.AssessmentPlanPO
	if err := json.Unmarshal(data, &po); err != nil {
		return nil, err
	}

	return r.mapper.ToDomain(&po), nil
}

// setCache 写入缓存
func (r *CachedPlanRepository) setCache(ctx context.Context, id plan.AssessmentPlanID, domain *plan.AssessmentPlan) error {
	if r.client == nil {
		return nil
	}

	key := r.buildCacheKey(id)
	po := r.mapper.ToPO(domain)
	data, err := json.Marshal(po)
	if err != nil {
		return err
	}

	payload := r.policy.CompressValue(data)
	observePayload(cachepolicy.PolicyPlan, len(data), len(payload))
	return r.client.Set(ctx, key, payload, r.policy.JitterTTL(r.ttl)).Err()
}

// deleteCache 删除缓存
func (r *CachedPlanRepository) deleteCache(ctx context.Context, id plan.AssessmentPlanID) error {
	if r.client == nil {
		return nil
	}

	key := r.buildCacheKey(id)
	err := r.client.Del(ctx, key).Err()
	if err != nil {
		observeInvalidate(cachepolicy.PolicyPlan, "error")
		return err
	}
	observeInvalidate(cachepolicy.PolicyPlan, "ok")
	return nil
}

// 实现其他 Repository 方法（透传，不缓存）
func (r *CachedPlanRepository) FindByScaleCode(ctx context.Context, scaleCode string) ([]*plan.AssessmentPlan, error) {
	return r.repo.FindByScaleCode(ctx, scaleCode)
}

func (r *CachedPlanRepository) FindActivePlans(ctx context.Context) ([]*plan.AssessmentPlan, error) {
	return r.repo.FindActivePlans(ctx)
}

func (r *CachedPlanRepository) FindByTesteeID(ctx context.Context, testeeID testee.ID) ([]*plan.AssessmentPlan, error) {
	return r.repo.FindByTesteeID(ctx, testeeID)
}

func (r *CachedPlanRepository) FindList(ctx context.Context, orgID int64, scaleCode string, status string, page, pageSize int) ([]*plan.AssessmentPlan, int64, error) {
	return r.repo.FindList(ctx, orgID, scaleCode, status, page, pageSize)
}
