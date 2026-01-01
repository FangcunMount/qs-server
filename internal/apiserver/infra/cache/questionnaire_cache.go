package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	questionnaireInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	redis "github.com/redis/go-redis/v9"
)

const (
	// QuestionnaireCachePrefix 问卷缓存键前缀
	QuestionnaireCachePrefix = "questionnaire:"
	// DefaultQuestionnaireCacheTTL 默认问卷缓存 TTL
	DefaultQuestionnaireCacheTTL = 12 * time.Hour
)

// CachedQuestionnaireRepository 带缓存的问卷 Repository 装饰器
// 实现 questionnaire.Repository 接口，在原有 Repository 基础上添加 Redis 缓存层
type CachedQuestionnaireRepository struct {
	repo   questionnaire.Repository
	client redis.UniversalClient
	ttl    time.Duration
	mapper *questionnaireInfra.QuestionnaireMapper
}

// NewCachedQuestionnaireRepository 创建带缓存的问卷 Repository
// 如果 client 为 nil，则降级为直接调用 repo（不缓存）
func NewCachedQuestionnaireRepository(repo questionnaire.Repository, client redis.UniversalClient) questionnaire.Repository {
	return &CachedQuestionnaireRepository{
		repo:   repo,
		client: client,
		ttl:    DefaultQuestionnaireCacheTTL,
		mapper: questionnaireInfra.NewQuestionnaireMapper(),
	}
}

// WithTTL 设置缓存 TTL
func (r *CachedQuestionnaireRepository) WithTTL(ttl time.Duration) *CachedQuestionnaireRepository {
	r.ttl = ttl
	return r
}

// buildCacheKey 构建缓存键
func (r *CachedQuestionnaireRepository) buildCacheKey(code, version string) string {
	if version != "" {
		return fmt.Sprintf("%s%s:%s", QuestionnaireCachePrefix, code, version)
	}
	return fmt.Sprintf("%s%s", QuestionnaireCachePrefix, code)
}

// Create 创建问卷（同时写入缓存）
func (r *CachedQuestionnaireRepository) Create(ctx context.Context, qDomain *questionnaire.Questionnaire) error {
	if err := r.repo.Create(ctx, qDomain); err != nil {
		return err
	}

	// 创建成功后写入缓存
	if r.client != nil {
		code := qDomain.GetCode().Value()
		version := qDomain.GetVersion().Value()
		if err := r.setCache(ctx, code, version, qDomain); err != nil {
			// 缓存写入失败不影响创建，仅记录日志
		}
	}

	return nil
}

// FindByCode 根据编码查询问卷（优先从缓存读取）
func (r *CachedQuestionnaireRepository) FindByCode(ctx context.Context, code string) (*questionnaire.Questionnaire, error) {
	// 1. 尝试从缓存读取（使用 code 作为 key，version 为空）
	if r.client != nil {
		if cached, err := r.getCache(ctx, code, ""); err == nil && cached != nil {
			return cached, nil
		}
	}

	// 2. 缓存未命中，从数据库查询
	qDomain, err := r.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if qDomain == nil {
		return nil, nil
	}

	// 3. 写入缓存（异步，不阻塞）
	if r.client != nil {
		go func() {
			_ = r.setCache(context.Background(), code, qDomain.GetVersion().Value(), qDomain)
		}()
	}

	return qDomain, nil
}

// FindByCodeVersion 根据编码和版本查询问卷（优先从缓存读取）
func (r *CachedQuestionnaireRepository) FindByCodeVersion(ctx context.Context, code, version string) (*questionnaire.Questionnaire, error) {
	// 1. 尝试从缓存读取
	if r.client != nil {
		if cached, err := r.getCache(ctx, code, version); err == nil && cached != nil {
			return cached, nil
		}
	}

	// 2. 缓存未命中，从数据库查询
	qDomain, err := r.repo.FindByCodeVersion(ctx, code, version)
	if err != nil {
		return nil, err
	}
	if qDomain == nil {
		return nil, nil
	}

	// 3. 写入缓存（异步，不阻塞）
	if r.client != nil {
		go func() {
			_ = r.setCache(context.Background(), code, version, qDomain)
		}()
	}

	return qDomain, nil
}

// FindBaseByCode 根据编码查询问卷基础信息
func (r *CachedQuestionnaireRepository) FindBaseByCode(ctx context.Context, code string) (*questionnaire.Questionnaire, error) {
	return r.repo.FindBaseByCode(ctx, code)
}

// FindBaseByCodeVersion 根据编码和版本查询问卷基础信息
func (r *CachedQuestionnaireRepository) FindBaseByCodeVersion(ctx context.Context, code, version string) (*questionnaire.Questionnaire, error) {
	return r.repo.FindBaseByCodeVersion(ctx, code, version)
}

// LoadQuestions 加载问卷问题详情
func (r *CachedQuestionnaireRepository) LoadQuestions(ctx context.Context, qDomain *questionnaire.Questionnaire) error {
	return r.repo.LoadQuestions(ctx, qDomain)
}

// FindBaseList 查询问卷基础列表
func (r *CachedQuestionnaireRepository) FindBaseList(ctx context.Context, page, pageSize int, conditions map[string]interface{}) ([]*questionnaire.Questionnaire, error) {
	// 列表查询不缓存（条件多样，缓存命中率低）
	return r.repo.FindBaseList(ctx, page, pageSize, conditions)
}

// CountWithConditions 统计问卷数量
func (r *CachedQuestionnaireRepository) CountWithConditions(ctx context.Context, conditions map[string]interface{}) (int64, error) {
	return r.repo.CountWithConditions(ctx, conditions)
}

// Update 更新问卷（同时失效缓存）
func (r *CachedQuestionnaireRepository) Update(ctx context.Context, qDomain *questionnaire.Questionnaire) error {
	if err := r.repo.Update(ctx, qDomain); err != nil {
		return err
	}

	// 更新成功后失效缓存（删除所有版本的缓存）
	if r.client != nil {
		code := qDomain.GetCode().Value()
		if err := r.deleteCacheByCode(ctx, code); err != nil {
			// 缓存删除失败不影响更新
		}
	}

	return nil
}

// Remove 删除问卷（同时失效缓存）
func (r *CachedQuestionnaireRepository) Remove(ctx context.Context, code string) error {
	if err := r.repo.Remove(ctx, code); err != nil {
		return err
	}

	// 删除成功后失效缓存
	if r.client != nil {
		if err := r.deleteCacheByCode(ctx, code); err != nil {
			// 缓存删除失败不影响删除
		}
	}

	return nil
}

// HardDelete 物理删除问卷（同时失效缓存）
func (r *CachedQuestionnaireRepository) HardDelete(ctx context.Context, code string) error {
	if err := r.repo.HardDelete(ctx, code); err != nil {
		return err
	}

	// 删除成功后失效缓存
	if r.client != nil {
		if err := r.deleteCacheByCode(ctx, code); err != nil {
			// 缓存删除失败不影响删除
		}
	}

	return nil
}

// ExistsByCode 检查编码是否存在
func (r *CachedQuestionnaireRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	return r.repo.ExistsByCode(ctx, code)
}

// ==================== 缓存操作 ====================

// getCache 从缓存获取问卷
func (r *CachedQuestionnaireRepository) getCache(ctx context.Context, code, version string) (*questionnaire.Questionnaire, error) {
	key := r.buildCacheKey(code, version)
	result := r.client.Get(ctx, key)
	if result.Err() == redis.Nil {
		return nil, nil // 缓存未命中，返回 nil
	}
	if result.Err() != nil {
		return nil, result.Err()
	}

	data := result.Val()
	// 反序列化为 PO
	var po questionnaireInfra.QuestionnairePO
	if err := json.Unmarshal([]byte(data), &po); err != nil {
		logger.L(ctx).Warnw("failed to unmarshal cached questionnaire", "code", code, "version", version, "error", err)
		return nil, err
	}

	// 通过 mapper 转换为 domain
	domain := r.mapper.ToBO(&po)
	return domain, nil
}

// setCache 写入缓存
func (r *CachedQuestionnaireRepository) setCache(ctx context.Context, code, version string, qDomain *questionnaire.Questionnaire) error {
	key := r.buildCacheKey(code, version)
	// 通过 mapper 转换为 PO
	po := r.mapper.ToPO(qDomain)
	// 序列化 PO（PO 有 JSON tag）
	data, err := json.Marshal(po)
	if err != nil {
		return err
	}

	return r.client.Set(ctx, key, data, r.ttl).Err()
}

// deleteCacheByCode 删除指定编码的所有版本缓存
func (r *CachedQuestionnaireRepository) deleteCacheByCode(ctx context.Context, code string) error {
	pattern := fmt.Sprintf("%s%s:*", QuestionnaireCachePrefix, code)
	// 也删除不带版本的 key
	keyWithoutVersion := r.buildCacheKey(code, "")
	
	// 删除不带版本的 key
	if err := r.client.Del(ctx, keyWithoutVersion).Err(); err != nil {
		logger.L(ctx).Warnw("failed to delete cache key", "key", keyWithoutVersion, "error", err)
	}

	// 使用 SCAN 删除所有匹配的 key
	iter := r.client.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		if err := r.client.Del(ctx, key).Err(); err != nil {
			logger.L(ctx).Warnw("failed to delete cache key", "key", key, "error", err)
		}
	}
	if err := iter.Err(); err != nil {
		return err
	}

	return nil
}

// WarmupCache 预热缓存（批量加载问卷）
func (r *CachedQuestionnaireRepository) WarmupCache(ctx context.Context, codes []string) error {
	if r.client == nil {
		return nil // Redis 不可用时跳过
	}

	for _, code := range codes {
		// 检查缓存是否已存在
		key := r.buildCacheKey(code, "")
		if r.client.Exists(ctx, key).Val() > 0 {
			continue // 已缓存，跳过
		}

		// 从数据库加载并写入缓存
		qDomain, err := r.repo.FindByCode(ctx, code)
		if err != nil {
			// 记录错误但继续处理其他问卷
			continue
		}
		if qDomain == nil {
			continue
		}

		if err := r.setCache(ctx, code, qDomain.GetVersion().Value(), qDomain); err != nil {
			// 记录错误但继续处理
			continue
		}
	}

	return nil
}
