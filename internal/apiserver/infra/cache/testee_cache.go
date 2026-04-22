package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	testeeInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	redis "github.com/redis/go-redis/v9"
)

const (
	defaultTesteeCacheTTL         = 30 * time.Minute
	defaultNegativeTesteeCacheTTL = 5 * time.Minute
)

// CachedTesteeRepository 带缓存的受试者 Repository 装饰器
// 实现 testee.Repository 接口，在原有 Repository 基础上添加 Redis 缓存层
type CachedTesteeRepository struct {
	repo   testee.Repository
	client redis.UniversalClient
	ttl    time.Duration
	mapper *testeeInfra.TesteeMapper
	keys   *rediskey.Builder
	policy cachepolicy.CachePolicy
}

// NewCachedTesteeRepositoryWithBuilderAndPolicy 创建带显式 builder/policy 的受试者缓存 Repository。
func NewCachedTesteeRepositoryWithBuilderAndPolicy(repo testee.Repository, client redis.UniversalClient, builder *rediskey.Builder, policy cachepolicy.CachePolicy) testee.Repository {
	if builder == nil {
		panic("redis builder is required")
	}
	return &CachedTesteeRepository{
		repo:   repo,
		client: client,
		ttl:    policy.TTLOr(defaultTesteeCacheTTL),
		mapper: testeeInfra.NewTesteeMapper(),
		keys:   builder,
		policy: policy,
	}
}

// buildCacheKey 构建缓存键
func (r *CachedTesteeRepository) buildCacheKey(id testee.ID) string {
	return r.keys.BuildTesteeInfoKey(id.Uint64())
}

// FindByID 根据ID查询受试者（优先从缓存读取）
func (r *CachedTesteeRepository) FindByID(ctx context.Context, id testee.ID) (*testee.Testee, error) {
	key := r.buildCacheKey(id)
	domain, err := ReadThrough(ctx, ReadThroughOptions[testee.Testee]{
		PolicyKey: cachepolicy.PolicyTestee,
		CacheKey:  key,
		Policy:    r.policy,
		GetCached: func(ctx context.Context) (*testee.Testee, error) { return r.getCache(ctx, id) },
		Load:      func(ctx context.Context) (*testee.Testee, error) { return r.repo.FindByID(ctx, id) },
		SetCached: func(ctx context.Context, value *testee.Testee) error { return r.setCache(ctx, id, value) },
		SetNegativeCached: func(ctx context.Context) error {
			return r.setNegativeCache(ctx, id)
		},
		AsyncSetCached:   true,
		AsyncSetNegative: true,
	})
	if err != nil {
		return nil, err
	}
	return domain, nil
}

func (r *CachedTesteeRepository) FindByIDs(ctx context.Context, ids []testee.ID) ([]*testee.Testee, error) {
	return r.repo.FindByIDs(ctx, ids)
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

// getCache 从缓存获取
func (r *CachedTesteeRepository) getCache(ctx context.Context, id testee.ID) (*testee.Testee, error) {
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

	if len(cachedData) == 0 {
		return nil, nil // 空值缓存，表示不存在
	}

	data := r.policy.DecompressValue(cachedData)
	observePayload(cachepolicy.PolicyTestee, len(data), len(cachedData))
	var po testeeInfra.TesteePO
	if err := json.Unmarshal(data, &po); err != nil {
		return nil, err
	}

	return r.mapper.ToDomain(&po), nil
}

// setCache 写入缓存
func (r *CachedTesteeRepository) setCache(ctx context.Context, id testee.ID, domain *testee.Testee) error {
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
	observePayload(cachepolicy.PolicyTestee, len(data), len(payload))
	return r.client.Set(ctx, key, payload, r.policy.JitterTTL(r.ttl)).Err()
}

func (r *CachedTesteeRepository) setNegativeCache(ctx context.Context, id testee.ID) error {
	if r.client == nil {
		return nil
	}
	key := r.buildCacheKey(id)
	ttl := r.policy.NegativeTTLOr(defaultNegativeTesteeCacheTTL)
	return r.client.Set(ctx, key, []byte{}, r.policy.JitterTTL(ttl)).Err()
}

// deleteCache 删除缓存
func (r *CachedTesteeRepository) deleteCache(ctx context.Context, id testee.ID) error {
	if r.client == nil {
		return nil
	}

	key := r.buildCacheKey(id)
	err := r.client.Del(ctx, key).Err()
	if err != nil {
		observeInvalidate(cachepolicy.PolicyTestee, "error")
		return err
	}
	observeInvalidate(cachepolicy.PolicyTestee, "ok")
	return nil
}

// 实现其他 Repository 方法（透传，不缓存）
func (r *CachedTesteeRepository) FindByProfile(ctx context.Context, orgID int64, profileID uint64) (*testee.Testee, error) {
	return r.repo.FindByProfile(ctx, orgID, profileID)
}

func (r *CachedTesteeRepository) FindByOrgAndName(ctx context.Context, orgID int64, name string) ([]*testee.Testee, error) {
	return r.repo.FindByOrgAndName(ctx, orgID, name)
}

func (r *CachedTesteeRepository) ListByOrg(ctx context.Context, orgID int64, filter testee.ListFilter, offset, limit int) ([]*testee.Testee, error) {
	return r.repo.ListByOrg(ctx, orgID, filter, offset, limit)
}

func (r *CachedTesteeRepository) ListByOrgAndIDs(ctx context.Context, orgID int64, ids []testee.ID, filter testee.ListFilter, offset, limit int) ([]*testee.Testee, error) {
	return r.repo.ListByOrgAndIDs(ctx, orgID, ids, filter, offset, limit)
}

func (r *CachedTesteeRepository) ListByTags(ctx context.Context, orgID int64, tags []string, offset, limit int) ([]*testee.Testee, error) {
	return r.repo.ListByTags(ctx, orgID, tags, offset, limit)
}

func (r *CachedTesteeRepository) ListKeyFocus(ctx context.Context, orgID int64, offset, limit int) ([]*testee.Testee, error) {
	return r.repo.ListKeyFocus(ctx, orgID, offset, limit)
}

func (r *CachedTesteeRepository) ListByProfileIDs(ctx context.Context, profileIDs []uint64, offset, limit int) ([]*testee.Testee, error) {
	return r.repo.ListByProfileIDs(ctx, profileIDs, offset, limit)
}

func (r *CachedTesteeRepository) Count(ctx context.Context, orgID int64, filter testee.ListFilter) (int64, error) {
	return r.repo.Count(ctx, orgID, filter)
}

func (r *CachedTesteeRepository) CountByOrgAndIDs(ctx context.Context, orgID int64, ids []testee.ID, filter testee.ListFilter) (int64, error) {
	return r.repo.CountByOrgAndIDs(ctx, orgID, ids, filter)
}
