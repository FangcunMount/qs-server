package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	planInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	redis "github.com/redis/go-redis/v9"
)

const (
	// PlanCachePrefix 计划缓存键前缀
	PlanCachePrefix = "plan:info:"
)

// DefaultPlanCacheTTL 默认计划缓存 TTL（可被配置覆盖）
var DefaultPlanCacheTTL = 2 * time.Hour

// CachedPlanRepository 带缓存的计划 Repository 装饰器
// 实现 plan.AssessmentPlanRepository 接口，在原有 Repository 基础上添加 Redis 缓存层
type CachedPlanRepository struct {
	repo   plan.AssessmentPlanRepository
	client redis.UniversalClient
	ttl    time.Duration
	mapper *planInfra.PlanMapper
}

// NewCachedPlanRepository 创建带缓存的计划 Repository
// 如果 client 为 nil，则降级为直接调用 repo（不缓存）
func NewCachedPlanRepository(repo plan.AssessmentPlanRepository, client redis.UniversalClient) plan.AssessmentPlanRepository {
	return &CachedPlanRepository{
		repo:   repo,
		client: client,
		ttl:    DefaultPlanCacheTTL,
		mapper: planInfra.NewPlanMapper(),
	}
}

// buildCacheKey 构建缓存键
func (r *CachedPlanRepository) buildCacheKey(id plan.AssessmentPlanID) string {
	return fmt.Sprintf("%s%s", PlanCachePrefix, id.String())
}

// FindByID 根据ID查询计划（优先从缓存读取）
func (r *CachedPlanRepository) FindByID(ctx context.Context, id plan.AssessmentPlanID) (*plan.AssessmentPlan, error) {
	// 1. 尝试从缓存读取
	if r.client != nil {
		if cached, err := r.getCache(ctx, id); err == nil && cached != nil {
			logger.L(ctx).Debugw("从Redis缓存获取计划信息", "plan_id", id.String())
			return cached, nil
		}
	}

	// 2. 缓存未命中，从数据库查询
	domain, err := r.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 3. 写入缓存（异步，不阻塞）
	if domain != nil && r.client != nil {
		if err := r.setCache(ctx, id, domain); err != nil {
			logger.L(ctx).Warnw("写入计划缓存失败", "plan_id", id.String(), "error", err.Error())
		}
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
		return nil, nil
	}

	key := r.buildCacheKey(id)
	cachedData, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, ErrCacheNotFound
	}
	if err != nil {
		return nil, err
	}

	var po planInfra.AssessmentPlanPO
	if err := json.Unmarshal(cachedData, &po); err != nil {
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

	return r.client.Set(ctx, key, data, JitterTTL(r.ttl)).Err()
}

// deleteCache 删除缓存
func (r *CachedPlanRepository) deleteCache(ctx context.Context, id plan.AssessmentPlanID) error {
	if r.client == nil {
		return nil
	}

	key := r.buildCacheKey(id)
	return r.client.Del(ctx, key).Err()
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
