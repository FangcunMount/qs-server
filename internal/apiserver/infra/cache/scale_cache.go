package cache

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	scaleInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	redis "github.com/redis/go-redis/v9"
)

const defaultScaleCacheTTL = 24 * time.Hour

// CachedScaleRepository 带缓存的量表 Repository 装饰器
// 实现 scale.Repository 接口，在原有 Repository 基础上添加 Redis 缓存层
type CachedScaleRepository struct {
	repo     scale.Repository
	keys     *rediskey.Builder
	policy   cachepolicy.CachePolicy
	observer *Observer
	store    *ObjectCacheStore[scale.MedicalScale]
}

// NewCachedScaleRepositoryWithBuilderAndPolicy 创建带显式 builder/policy 的量表缓存 Repository。
func NewCachedScaleRepositoryWithBuilderAndPolicy(repo scale.Repository, client redis.UniversalClient, builder *rediskey.Builder, policy cachepolicy.CachePolicy) scale.Repository {
	return NewCachedScaleRepositoryWithBuilderPolicyAndObserver(repo, client, builder, policy, nil)
}

func NewCachedScaleRepositoryWithBuilderPolicyAndObserver(repo scale.Repository, client redis.UniversalClient, builder *rediskey.Builder, policy cachepolicy.CachePolicy, observer *Observer) scale.Repository {
	if builder == nil {
		panic("redis builder is required")
	}
	mapper := scaleInfra.NewScaleMapper()
	return &CachedScaleRepository{
		repo:     repo,
		keys:     builder,
		policy:   policy,
		observer: observer,
		store: NewObjectCacheStore(ObjectCacheStoreOptions[scale.MedicalScale]{
			Cache:     newRedisCacheIfAvailable(client),
			PolicyKey: cachepolicy.PolicyScale,
			Policy:    policy,
			TTL:       policy.TTLOr(defaultScaleCacheTTL),
			Codec:     newScaleCacheEntryCodec(mapper),
		}),
	}
}

func newScaleCacheEntryCodec(mapper *scaleInfra.ScaleMapper) CacheEntryCodec[scale.MedicalScale] {
	return CacheEntryCodec[scale.MedicalScale]{
		EncodeFunc: func(domain *scale.MedicalScale) ([]byte, error) {
			return json.Marshal(mapper.ToPO(domain))
		},
		DecodeFunc: func(data []byte) (*scale.MedicalScale, error) {
			var po scaleInfra.ScalePO
			if err := json.Unmarshal(data, &po); err != nil {
				return nil, err
			}
			return mapper.ToDomain(context.Background(), &po), nil
		},
	}
}

// WithTTL 设置缓存 TTL
func (r *CachedScaleRepository) WithTTL(ttl time.Duration) *CachedScaleRepository {
	if r.store != nil {
		r.store.ttl = ttl
	}
	return r
}

// buildCacheKey 构建缓存键
func (r *CachedScaleRepository) buildCacheKey(code string) string {
	return r.keys.BuildScaleKey(strings.ToLower(code))
}

// Create 创建量表（同时写入缓存）
func (r *CachedScaleRepository) Create(ctx context.Context, domain *scale.MedicalScale) error {
	if err := r.repo.Create(ctx, domain); err != nil {
		return err
	}

	// 创建成功后写入缓存
	if r.store != nil && r.store.cache != nil {
		if err := r.setCache(ctx, domain.GetCode().String(), domain); err != nil {
			// 缓存写入失败不影响创建，仅记录日志
			logger.L(ctx).Warnw("failed to populate scale cache after create",
				"code", domain.GetCode().String(),
				"error", err,
			)
		}
	}

	return nil
}

// FindByCode 根据编码查询量表（优先从缓存读取）
func (r *CachedScaleRepository) FindByCode(ctx context.Context, code string) (*scale.MedicalScale, error) {
	key := r.buildCacheKey(code)
	return ReadThrough(ctx, ReadThroughOptions[scale.MedicalScale]{
		PolicyKey:      cachepolicy.PolicyScale,
		CacheKey:       key,
		Policy:         r.policy,
		Observer:       r.observer,
		GetCached:      func(ctx context.Context) (*scale.MedicalScale, error) { return r.getCache(ctx, code) },
		Load:           func(ctx context.Context) (*scale.MedicalScale, error) { return r.repo.FindByCode(ctx, code) },
		SetCached:      func(ctx context.Context, value *scale.MedicalScale) error { return r.setCache(ctx, code, value) },
		AsyncSetCached: true,
	})
}

// FindByQuestionnaireCode 根据问卷编码查询量表
func (r *CachedScaleRepository) FindByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*scale.MedicalScale, error) {
	// 问卷编码查询不缓存（使用频率低，且需要维护额外索引）
	return r.repo.FindByQuestionnaireCode(ctx, questionnaireCode)
}

// FindSummaryList 查询量表摘要列表
func (r *CachedScaleRepository) FindSummaryList(ctx context.Context, page, pageSize int, conditions map[string]interface{}) ([]*scale.MedicalScale, error) {
	// 列表查询不缓存（条件多样，缓存命中率低）
	return r.repo.FindSummaryList(ctx, page, pageSize, conditions)
}

// CountWithConditions 统计量表数量
func (r *CachedScaleRepository) CountWithConditions(ctx context.Context, conditions map[string]interface{}) (int64, error) {
	return r.repo.CountWithConditions(ctx, conditions)
}

// Update 更新量表（同时失效缓存）
func (r *CachedScaleRepository) Update(ctx context.Context, domain *scale.MedicalScale) error {
	oldCode := domain.GetCode().String()

	if err := r.repo.Update(ctx, domain); err != nil {
		return err
	}

	// 更新成功后失效缓存
	if r.store != nil && r.store.cache != nil {
		if err := r.deleteCache(ctx, oldCode); err != nil {
			logger.L(ctx).Warnw("failed to invalidate scale cache after update",
				"code", oldCode,
				"error", err,
			)
		}
	}

	return nil
}

// Remove 删除量表（同时失效缓存）
func (r *CachedScaleRepository) Remove(ctx context.Context, code string) error {
	if err := r.repo.Remove(ctx, code); err != nil {
		return err
	}

	// 删除成功后失效缓存
	if r.store != nil && r.store.cache != nil {
		if err := r.deleteCache(ctx, code); err != nil {
			logger.L(ctx).Warnw("failed to invalidate scale cache after remove",
				"code", code,
				"error", err,
			)
		}
	}

	return nil
}

// ExistsByCode 检查编码是否存在
func (r *CachedScaleRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	return r.repo.ExistsByCode(ctx, code)
}

// ==================== 缓存操作 ====================

// getCache 从缓存获取量表
func (r *CachedScaleRepository) getCache(ctx context.Context, code string) (*scale.MedicalScale, error) {
	return r.store.Get(ctx, r.buildCacheKey(code))
}

// setCache 写入缓存
func (r *CachedScaleRepository) setCache(ctx context.Context, code string, domain *scale.MedicalScale) error {
	return r.store.Set(ctx, r.buildCacheKey(code), domain)
}

// deleteCache 删除缓存
func (r *CachedScaleRepository) deleteCache(ctx context.Context, code string) error {
	return r.store.Delete(ctx, r.buildCacheKey(code))
}

// WarmupCache 预热缓存（批量加载量表）
func (r *CachedScaleRepository) WarmupCache(ctx context.Context, codes []string) error {
	if r.store == nil || r.store.cache == nil {
		return nil // Redis 不可用时跳过
	}

	for _, code := range codes {
		// 检查缓存是否已存在
		key := r.buildCacheKey(code)
		exists, err := r.store.Exists(ctx, key)
		if err == nil && exists {
			continue // 已缓存，跳过
		}

		// 从数据库加载并写入缓存
		domain, err := r.repo.FindByCode(ctx, code)
		if err != nil {
			// 记录错误但继续处理其他量表
			continue
		}

		if err := r.setCache(ctx, code, domain); err != nil {
			// 记录错误但继续处理
			continue
		}
	}

	return nil
}
