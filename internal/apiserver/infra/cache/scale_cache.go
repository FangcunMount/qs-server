package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	scaleInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/scale"
	redis "github.com/redis/go-redis/v9"
)

const (
	// ScaleCachePrefix 量表缓存键前缀
	ScaleCachePrefix = "scale:"
)

// DefaultScaleCacheTTL 默认量表缓存 TTL（可被配置覆盖）
var DefaultScaleCacheTTL = 24 * time.Hour

// CachedScaleRepository 带缓存的量表 Repository 装饰器
// 实现 scale.Repository 接口，在原有 Repository 基础上添加 Redis 缓存层
type CachedScaleRepository struct {
	repo   scale.Repository
	client redis.UniversalClient
	ttl    time.Duration
	mapper *scaleInfra.ScaleMapper
}

// NewCachedScaleRepository 创建带缓存的量表 Repository
// 如果 client 为 nil，则降级为直接调用 repo（不缓存）
func NewCachedScaleRepository(repo scale.Repository, client redis.UniversalClient) scale.Repository {
	return &CachedScaleRepository{
		repo:   repo,
		client: client,
		ttl:    DefaultScaleCacheTTL,
		mapper: scaleInfra.NewScaleMapper(),
	}
}

// WithTTL 设置缓存 TTL
func (r *CachedScaleRepository) WithTTL(ttl time.Duration) *CachedScaleRepository {
	r.ttl = ttl
	return r
}

// buildCacheKey 构建缓存键
func (r *CachedScaleRepository) buildCacheKey(code string) string {
	return addNamespace(fmt.Sprintf("%s%s", ScaleCachePrefix, strings.ToLower(code)))
}

// Create 创建量表（同时写入缓存）
func (r *CachedScaleRepository) Create(ctx context.Context, domain *scale.MedicalScale) error {
	if err := r.repo.Create(ctx, domain); err != nil {
		return err
	}

	// 创建成功后写入缓存
	if r.client != nil {
		if err := r.setCache(ctx, domain.GetCode().String(), domain); err != nil {
			// 缓存写入失败不影响创建，仅记录日志
			// 这里不返回错误，因为业务已成功
		}
	}

	return nil
}

// FindByCode 根据编码查询量表（优先从缓存读取）
func (r *CachedScaleRepository) FindByCode(ctx context.Context, code string) (*scale.MedicalScale, error) {
	// 1. 尝试从缓存读取
	if r.client != nil {
		if cached, err := r.getCache(ctx, code); err == nil {
			if cached != nil {
				return cached, nil
			}
			return nil, nil
		} else if err != ErrCacheNotFound {
			return nil, err
		}
	}

	// 2. 缓存未命中，从数据库查询
	val, err, _ := Group.Do("scale:"+code, func() (interface{}, error) {
		return r.repo.FindByCode(ctx, code)
	})
	if err != nil {
		return nil, err
	}
	domain, _ := val.(*scale.MedicalScale)
	if domain == nil {
		return nil, nil
	}

	// 3. 写入缓存（异步，不阻塞）
	if r.client != nil {
		go func() {
			_ = r.setCache(context.Background(), code, domain)
		}()
	} else if r.client != nil && domain == nil {
		_ = r.setNilCache(context.Background(), code)
	}

	return domain, nil
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
	if r.client != nil {
		if err := r.deleteCache(ctx, oldCode); err != nil {
			// 缓存删除失败不影响更新
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
	if r.client != nil {
		if err := r.deleteCache(ctx, code); err != nil {
			// 缓存删除失败不影响删除
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
	// 1. 构建缓存键
	key := r.buildCacheKey(code)
	// 2. 获取缓存
	result := r.client.Get(ctx, key)
	if result.Err() == redis.Nil {
		return nil, ErrCacheNotFound // 缓存未命中，返回 ErrCacheNotFound
	}
	if result.Err() != nil {
		return nil, result.Err()
	}

	// 3. 解压数据
	dataBytes, err := result.Bytes()
	if err != nil {
		return nil, err
	}
	// 4. 反序列化为 PO
	data := decompressIfNeeded(dataBytes)
	var po scaleInfra.ScalePO
	if err := json.Unmarshal(data, &po); err != nil {
		logger.L(ctx).Warnw("failed to unmarshal cached scale", "code", code, "error", err)
		return nil, err
	}

	// 5. 通过 mapper 转换为 domain
	domain := r.mapper.ToDomain(ctx, &po)
	return domain, nil
}

// setCache 写入缓存
func (r *CachedScaleRepository) setCache(ctx context.Context, code string, domain *scale.MedicalScale) error {
	key := r.buildCacheKey(code)
	// 通过 mapper 转换为 PO
	po := r.mapper.ToPO(domain)
	// 序列化 PO（PO 有 JSON tag）
	data, err := json.Marshal(po)
	if err != nil {
		return err
	}

	return r.client.Set(ctx, key, compressIfEnabled(data), JitterTTL(r.ttl)).Err()
}

// setNilCache 设置空值缓存，防止穿透，短 TTL
func (r *CachedScaleRepository) setNilCache(ctx context.Context, code string) error {
	key := r.buildCacheKey(code)
	return r.client.Set(ctx, key, []byte{}, JitterTTL(NegativeCacheTTL)).Err()
}

// deleteCache 删除缓存
func (r *CachedScaleRepository) deleteCache(ctx context.Context, code string) error {
	key := r.buildCacheKey(code)
	return r.client.Del(ctx, key).Err()
}

// WarmupCache 预热缓存（批量加载量表）
func (r *CachedScaleRepository) WarmupCache(ctx context.Context, codes []string) error {
	if r.client == nil {
		return nil // Redis 不可用时跳过
	}

	for _, code := range codes {
		// 检查缓存是否已存在
		key := r.buildCacheKey(code)
		if r.client.Exists(ctx, key).Val() > 0 {
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
