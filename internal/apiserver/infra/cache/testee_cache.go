package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	testeeInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	redis "github.com/redis/go-redis/v9"
)

const (
	// TesteeCachePrefix 受试者缓存键前缀
	TesteeCachePrefix = "testee:info:"
)

// DefaultTesteeCacheTTL 默认受试者缓存 TTL（可被配置覆盖）
var DefaultTesteeCacheTTL = 2 * time.Hour

// CachedTesteeRepository 带缓存的受试者 Repository 装饰器
// 实现 testee.Repository 接口，在原有 Repository 基础上添加 Redis 缓存层
type CachedTesteeRepository struct {
	repo   testee.Repository
	client redis.UniversalClient
	ttl    time.Duration
	mapper *testeeInfra.TesteeMapper
}

// NewCachedTesteeRepository 创建带缓存的受试者 Repository
// 如果 client 为 nil，则降级为直接调用 repo（不缓存）
func NewCachedTesteeRepository(repo testee.Repository, client redis.UniversalClient) testee.Repository {
	return &CachedTesteeRepository{
		repo:   repo,
		client: client,
		ttl:    DefaultTesteeCacheTTL,
		mapper: testeeInfra.NewTesteeMapper(),
	}
}

// buildCacheKey 构建缓存键
func (r *CachedTesteeRepository) buildCacheKey(id testee.ID) string {
	return addNamespace(fmt.Sprintf("%s%d", TesteeCachePrefix, uint64(id)))
}

// FindByID 根据ID查询受试者（优先从缓存读取）
func (r *CachedTesteeRepository) FindByID(ctx context.Context, id testee.ID) (*testee.Testee, error) {
	// 1. 尝试从缓存读取
	if r.client != nil {
		if cached, err := r.getCache(ctx, id); err == nil {
			if cached != nil {
				logger.L(ctx).Debugw("从Redis缓存获取受试者信息", "testee_id", uint64(id))
				return cached, nil
			}
			return nil, nil
		} else if err != ErrCacheNotFound {
			return nil, err
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
			logger.L(ctx).Warnw("写入受试者缓存失败", "testee_id", uint64(id), "error", err.Error())
		}
	} else if domain == nil && r.client != nil {
		_ = r.setNilCache(ctx, id)
	}

	return domain, nil
}

// Save 保存受试者（同时失效缓存）
func (r *CachedTesteeRepository) Save(ctx context.Context, domain *testee.Testee) error {
	err := r.repo.Save(ctx, domain)
	if err == nil && domain != nil {
		r.deleteCache(ctx, domain.ID())
	}
	return err
}

// Update 更新受试者（同时失效缓存）
func (r *CachedTesteeRepository) Update(ctx context.Context, domain *testee.Testee) error {
	err := r.repo.Update(ctx, domain)
	if err == nil && domain != nil {
		r.deleteCache(ctx, domain.ID())
	}
	return err
}

// Delete 删除受试者（同时失效缓存）
func (r *CachedTesteeRepository) Delete(ctx context.Context, id testee.ID) error {
	err := r.repo.Delete(ctx, id)
	if err == nil {
		r.deleteCache(ctx, id)
	}
	return err
}

// getCache 从缓存获取
func (r *CachedTesteeRepository) getCache(ctx context.Context, id testee.ID) (*testee.Testee, error) {
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

	var po testeeInfra.TesteePO
	if err := json.Unmarshal(cachedData, &po); err != nil {
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

	return r.client.Set(ctx, key, data, JitterTTL(r.ttl)).Err()
}

// deleteCache 删除缓存
func (r *CachedTesteeRepository) deleteCache(ctx context.Context, id testee.ID) error {
	if r.client == nil {
		return nil
	}

	key := r.buildCacheKey(id)
	return r.client.Del(ctx, key).Err()
}

// setNilCache 设置空值缓存，防止穿透
func (r *CachedTesteeRepository) setNilCache(ctx context.Context, id testee.ID) error {
	if r.client == nil {
		return nil
	}
	key := r.buildCacheKey(id)
	return r.client.Set(ctx, key, []byte{}, JitterTTL(5*time.Minute)).Err()
}

// 实现其他 Repository 方法（透传，不缓存）
func (r *CachedTesteeRepository) FindByProfile(ctx context.Context, orgID int64, profileID uint64) (*testee.Testee, error) {
	return r.repo.FindByProfile(ctx, orgID, profileID)
}

func (r *CachedTesteeRepository) FindByOrgAndName(ctx context.Context, orgID int64, name string) ([]*testee.Testee, error) {
	return r.repo.FindByOrgAndName(ctx, orgID, name)
}

func (r *CachedTesteeRepository) ListByOrg(ctx context.Context, orgID int64, offset, limit int) ([]*testee.Testee, error) {
	return r.repo.ListByOrg(ctx, orgID, offset, limit)
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

func (r *CachedTesteeRepository) Count(ctx context.Context, orgID int64) (int64, error) {
	return r.repo.Count(ctx, orgID)
}
