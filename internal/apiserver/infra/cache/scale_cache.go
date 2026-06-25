package cache

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/authoring/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	scaleInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	redis "github.com/redis/go-redis/v9"
)

const defaultScaleCacheTTL = 24 * time.Hour

// CachedScaleRepository 带缓存的量表 Repository 装饰器
// 实现 scale.Repository 接口，在原有 Repository 基础上添加 Redis 缓存层
type CachedScaleRepository struct {
	repo     scale.Repository
	keys     *keyspace.Builder
	policy   cachepolicy.CachePolicy
	observer *observability.ComponentObserver
	store    *ObjectCacheStore[scale.MedicalScale]
}

// NewCachedScaleRepositoryWithBuilderAndPolicy 创建带显式 builder/policy 的量表缓存 Repository。
func NewCachedScaleRepositoryWithBuilderAndPolicy(repo scale.Repository, client redis.UniversalClient, builder *keyspace.Builder, policy cachepolicy.CachePolicy) scale.Repository {
	return NewCachedScaleRepositoryWithBuilderPolicyAndObserver(repo, client, builder, policy, nil)
}

func NewCachedScaleRepositoryWithBuilderPolicyAndObserver(repo scale.Repository, client redis.UniversalClient, builder *keyspace.Builder, policy cachepolicy.CachePolicy, observer *observability.ComponentObserver) scale.Repository {
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

func (r *CachedScaleRepository) buildVersionCacheKey(code, version string) string {
	return r.keys.BuildScaleVersionKey(strings.ToLower(code), strings.ToLower(version))
}

func (r *CachedScaleRepository) buildPublishedScaleCacheKey(code string) string {
	return r.keys.BuildPublishedScaleKey(strings.ToLower(code))
}

func (r *CachedScaleRepository) buildPublishedScaleByQuestionnaireCacheKey(questionnaireCode string) string {
	return r.keys.BuildPublishedScaleByQuestionnaireKey(strings.ToLower(questionnaireCode))
}

// Create 创建量表（同时写入缓存）
func (r *CachedScaleRepository) Create(ctx context.Context, domain *scale.MedicalScale) error {
	if err := r.repo.Create(ctx, domain); err != nil {
		return err
	}

	// 创建成功后写入缓存
	if r.store.available() {
		if err := r.setCache(ctx, domain.GetCode().String(), domain); err != nil {
			// 缓存写入失败不影响创建，仅记录日志
			logger.L(ctx).Warnw("failed to populate scale cache after create",
				"code", domain.GetCode().String(),
				"error", err,
			)
		}
		if version := domain.GetScaleVersion(); version != "" {
			if err := r.setVersionCache(ctx, domain.GetCode().String(), version, domain); err != nil {
				logger.L(ctx).Warnw("failed to populate scale version cache after create",
					"code", domain.GetCode().String(),
					"scale_version", version,
					"error", err,
				)
			}
		}
	}

	return nil
}

// CreatePublishedSnapshot 创建或更新已发布快照并失效相关缓存。
func (r *CachedScaleRepository) CreatePublishedSnapshot(ctx context.Context, domain *scale.MedicalScale, active bool) error {
	if err := r.repo.CreatePublishedSnapshot(ctx, domain, active); err != nil {
		return err
	}
	r.invalidateScaleFamilyCache(ctx, domain.GetCode().String(), domain.GetScaleVersion(), domain.GetQuestionnaireCode().String())
	return nil
}

// FindByCode 根据编码查询量表（优先从缓存读取）
func (r *CachedScaleRepository) FindByCode(ctx context.Context, code string) (*scale.MedicalScale, error) {
	domain, err := ReadThroughObject(ctx, ObjectReadThroughOptions[scale.MedicalScale]{
		PolicyKey:      cachepolicy.PolicyScale,
		CacheKey:       r.buildCacheKey(code),
		Policy:         r.policy,
		Observer:       r.observer,
		Store:          r.store,
		Load:           func(ctx context.Context) (*scale.MedicalScale, error) { return r.repo.FindByCode(ctx, code) },
		AsyncSetCached: true,
	})
	if err != nil || !isStalePublishedScaleCache(domain) {
		return domain, err
	}
	return r.reloadScaleCacheFromSource(ctx, code)
}

// FindByCodeVersion 根据量表编码和版本查询量表。
func (r *CachedScaleRepository) FindByCodeVersion(ctx context.Context, code, scaleVersion string) (*scale.MedicalScale, error) {
	if scaleVersion == "" {
		return r.FindByCode(ctx, code)
	}
	domain, err := ReadThroughObject(ctx, ObjectReadThroughOptions[scale.MedicalScale]{
		PolicyKey: cachepolicy.PolicyScale,
		CacheKey:  r.buildVersionCacheKey(code, scaleVersion),
		Policy:    r.policy,
		Observer:  r.observer,
		Store:     r.store,
		Load: func(ctx context.Context) (*scale.MedicalScale, error) {
			return r.repo.FindByCodeVersion(ctx, code, scaleVersion)
		},
		AsyncSetCached: true,
	})
	if err != nil || !isStalePublishedScaleCache(domain) {
		return domain, err
	}
	return r.reloadScaleVersionCacheFromSource(ctx, code, scaleVersion)
}

// FindPublishedByCode 根据编码查询当前激活的已发布量表快照。
func (r *CachedScaleRepository) FindPublishedByCode(ctx context.Context, code string) (*scale.MedicalScale, error) {
	domain, err := ReadThroughObject(ctx, ObjectReadThroughOptions[scale.MedicalScale]{
		PolicyKey: cachepolicy.PolicyScale,
		CacheKey:  r.buildPublishedScaleCacheKey(code),
		Policy:    r.policy,
		Observer:  r.observer,
		Store:     r.store,
		Load: func(ctx context.Context) (*scale.MedicalScale, error) {
			return r.repo.FindPublishedByCode(ctx, code)
		},
		AsyncSetCached: true,
	})
	if err != nil || !isStalePublishedScaleCache(domain) {
		if err == nil && domain != nil {
			r.warmPublishedScaleQuestionnaireCacheAlias(ctx, domain)
		}
		return domain, err
	}
	return r.reloadPublishedScaleCacheFromSource(ctx, code)
}

// FindByQuestionnaireCode 根据问卷编码查询量表
func (r *CachedScaleRepository) FindByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*scale.MedicalScale, error) {
	// 问卷编码查询不缓存（使用频率低，且需要维护额外索引）
	return r.repo.FindByQuestionnaireCode(ctx, questionnaireCode)
}

// FindPublishedByQuestionnaireCode 根据问卷编码查询当前激活的已发布量表快照。
func (r *CachedScaleRepository) FindPublishedByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*scale.MedicalScale, error) {
	domain, err := ReadThroughObject(ctx, ObjectReadThroughOptions[scale.MedicalScale]{
		PolicyKey: cachepolicy.PolicyScale,
		CacheKey:  r.buildPublishedScaleByQuestionnaireCacheKey(questionnaireCode),
		Policy:    r.policy,
		Observer:  r.observer,
		Store:     r.store,
		Load: func(ctx context.Context) (*scale.MedicalScale, error) {
			return r.repo.FindPublishedByQuestionnaireCode(ctx, questionnaireCode)
		},
		AsyncSetCached: true,
	})
	if err != nil || !isStalePublishedScaleCache(domain) {
		if err == nil && domain != nil {
			r.warmPublishedScaleCodeCacheAlias(ctx, domain)
		}
		return domain, err
	}
	return r.reloadPublishedScaleByQuestionnaireCacheFromSource(ctx, questionnaireCode)
}

// FindByQuestionnaireRef 根据问卷编码和版本查询量表
func (r *CachedScaleRepository) FindByQuestionnaireRef(ctx context.Context, questionnaireCode, questionnaireVersion string) (*scale.MedicalScale, error) {
	// 问卷引用查询不缓存（使用频率低，且需要维护额外索引）
	return r.repo.FindByQuestionnaireRef(ctx, questionnaireCode, questionnaireVersion)
}

// Update 更新量表（同时失效缓存）
func (r *CachedScaleRepository) Update(ctx context.Context, domain *scale.MedicalScale) error {
	oldCode := domain.GetCode().String()
	var oldVersion string
	var questionnaireCode string
	if existing, err := r.repo.FindByCode(ctx, oldCode); err == nil && existing != nil {
		oldVersion = existing.GetScaleVersion()
		questionnaireCode = existing.GetQuestionnaireCode().String()
	} else if err != nil && !scale.IsNotFound(err) {
		logger.L(ctx).Warnw("failed to load old scale version before cache invalidation",
			"code", oldCode,
			"error", err,
		)
	}

	if err := r.repo.Update(ctx, domain); err != nil {
		return err
	}

	// 更新成功后失效缓存
	if r.store.available() {
		if err := r.deleteCache(ctx, oldCode); err != nil {
			logger.L(ctx).Warnw("failed to invalidate scale cache after update",
				"code", oldCode,
				"error", err,
			)
		}
		versions := make(map[string]struct{}, 2)
		if oldVersion != "" {
			versions[oldVersion] = struct{}{}
		}
		if version := domain.GetScaleVersion(); version != "" {
			versions[version] = struct{}{}
		}
		for version := range versions {
			if err := r.deleteVersionCache(ctx, oldCode, version); err != nil {
				logger.L(ctx).Warnw("failed to invalidate scale version cache after update",
					"code", oldCode,
					"scale_version", version,
					"error", err,
				)
			}
		}
		if err := r.deletePublishedScaleFamilyCache(ctx, oldCode, questionnaireCode); err != nil {
			logger.L(ctx).Warnw("failed to invalidate published scale cache after update",
				"code", oldCode,
				"questionnaire_code", questionnaireCode,
				"error", err,
			)
		}
	}

	return nil
}

// SetActivePublishedVersion 切换当前激活的已发布快照并失效缓存。
func (r *CachedScaleRepository) SetActivePublishedVersion(ctx context.Context, code, scaleVersion string) error {
	if err := r.repo.SetActivePublishedVersion(ctx, code, scaleVersion); err != nil {
		return err
	}
	r.invalidateScaleFamilyCache(ctx, code, scaleVersion, "")
	return nil
}

// ClearActivePublishedVersion 清空当前激活快照并失效缓存。
func (r *CachedScaleRepository) ClearActivePublishedVersion(ctx context.Context, code string) error {
	if err := r.repo.ClearActivePublishedVersion(ctx, code); err != nil {
		return err
	}
	r.invalidateScaleFamilyCache(ctx, code, "", "")
	return nil
}

// Remove 删除量表（同时失效缓存）
func (r *CachedScaleRepository) Remove(ctx context.Context, code string) error {
	var removedVersion string
	var questionnaireCode string
	if existing, err := r.repo.FindByCode(ctx, code); err == nil && existing != nil {
		removedVersion = existing.GetScaleVersion()
		questionnaireCode = existing.GetQuestionnaireCode().String()
	}
	if err := r.repo.Remove(ctx, code); err != nil {
		return err
	}

	// 删除成功后失效缓存
	if r.store.available() {
		if err := r.deleteCache(ctx, code); err != nil {
			logger.L(ctx).Warnw("failed to invalidate scale cache after remove",
				"code", code,
				"error", err,
			)
		}
		if removedVersion != "" {
			if err := r.deleteVersionCache(ctx, code, removedVersion); err != nil {
				logger.L(ctx).Warnw("failed to invalidate scale version cache after remove",
					"code", code,
					"scale_version", removedVersion,
					"error", err,
				)
			}
		}
		if err := r.deletePublishedScaleFamilyCache(ctx, code, questionnaireCode); err != nil {
			logger.L(ctx).Warnw("failed to invalidate published scale cache after remove",
				"code", code,
				"questionnaire_code", questionnaireCode,
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

func (r *CachedScaleRepository) invalidateScaleFamilyCache(ctx context.Context, code, version, questionnaireCode string) {
	if !r.store.available() {
		return
	}
	if err := r.deleteCache(ctx, code); err != nil {
		logger.L(ctx).Warnw("failed to invalidate scale cache",
			"code", code,
			"error", err,
		)
	}
	if err := r.deletePublishedScaleFamilyCache(ctx, code, questionnaireCode); err != nil {
		logger.L(ctx).Warnw("failed to invalidate published scale cache",
			"code", code,
			"questionnaire_code", questionnaireCode,
			"error", err,
		)
	}
	if version == "" {
		return
	}
	if err := r.deleteVersionCache(ctx, code, version); err != nil {
		logger.L(ctx).Warnw("failed to invalidate scale version cache",
			"code", code,
			"scale_version", version,
			"error", err,
		)
	}
}

// ==================== 缓存操作 ====================

// setCache 写入缓存
func (r *CachedScaleRepository) setCache(ctx context.Context, code string, domain *scale.MedicalScale) error {
	return r.store.Set(ctx, r.buildCacheKey(code), domain)
}

func (r *CachedScaleRepository) setVersionCache(ctx context.Context, code, version string, domain *scale.MedicalScale) error {
	return r.store.Set(ctx, r.buildVersionCacheKey(code, version), domain)
}

func (r *CachedScaleRepository) setPublishedScaleCache(ctx context.Context, code string, domain *scale.MedicalScale) error {
	return r.store.Set(ctx, r.buildPublishedScaleCacheKey(code), domain)
}

func (r *CachedScaleRepository) setPublishedScaleByQuestionnaireCache(ctx context.Context, questionnaireCode string, domain *scale.MedicalScale) error {
	return r.store.Set(ctx, r.buildPublishedScaleByQuestionnaireCacheKey(questionnaireCode), domain)
}

func (r *CachedScaleRepository) deletePublishedScaleCache(ctx context.Context, code string) error {
	return r.store.Delete(ctx, r.buildPublishedScaleCacheKey(code))
}

func (r *CachedScaleRepository) deletePublishedScaleByQuestionnaireCache(ctx context.Context, questionnaireCode string) error {
	return r.store.Delete(ctx, r.buildPublishedScaleByQuestionnaireCacheKey(questionnaireCode))
}

func (r *CachedScaleRepository) deletePublishedScaleFamilyCache(ctx context.Context, code, questionnaireCode string) error {
	if !r.store.available() {
		return nil
	}
	if err := r.deletePublishedScaleCache(ctx, code); err != nil {
		return err
	}
	if questionnaireCode == "" {
		return nil
	}
	return r.deletePublishedScaleByQuestionnaireCache(ctx, questionnaireCode)
}

func (r *CachedScaleRepository) warmPublishedScaleCodeCacheAlias(ctx context.Context, domain *scale.MedicalScale) {
	if domain == nil || !r.store.available() {
		return
	}
	code := domain.GetCode().String()
	if code == "" {
		return
	}
	if err := r.setPublishedScaleCache(ctx, code, domain); err != nil {
		logger.L(ctx).Warnw("failed to warm published scale code cache alias",
			"code", code,
			"error", err,
		)
	}
}

func (r *CachedScaleRepository) warmPublishedScaleQuestionnaireCacheAlias(ctx context.Context, domain *scale.MedicalScale) {
	if domain == nil || !r.store.available() {
		return
	}
	questionnaireCode := domain.GetQuestionnaireCode().String()
	if questionnaireCode == "" {
		return
	}
	if err := r.setPublishedScaleByQuestionnaireCache(ctx, questionnaireCode, domain); err != nil {
		logger.L(ctx).Warnw("failed to warm published scale questionnaire cache alias",
			"questionnaire_code", questionnaireCode,
			"error", err,
		)
	}
}

func (r *CachedScaleRepository) warmPublishedScaleAliasCaches(ctx context.Context, domain *scale.MedicalScale) {
	r.warmPublishedScaleCodeCacheAlias(ctx, domain)
	r.warmPublishedScaleQuestionnaireCacheAlias(ctx, domain)
}

// deleteCache 删除缓存
func (r *CachedScaleRepository) deleteCache(ctx context.Context, code string) error {
	return r.store.Delete(ctx, r.buildCacheKey(code))
}

func (r *CachedScaleRepository) deleteVersionCache(ctx context.Context, code, version string) error {
	return r.store.Delete(ctx, r.buildVersionCacheKey(code, version))
}

func isStalePublishedScaleCache(domain *scale.MedicalScale) bool {
	return domain != nil && domain.IsPublished() && domain.FactorCount() == 0
}

func (r *CachedScaleRepository) reloadScaleCacheFromSource(ctx context.Context, code string) (*scale.MedicalScale, error) {
	logger.L(ctx).Warnw("published scale cache has no factors, reloading from source",
		"code", code,
	)
	if r.store.available() {
		if err := r.deleteCache(ctx, code); err != nil {
			logger.L(ctx).Warnw("failed to delete stale scale cache",
				"code", code,
				"error", err,
			)
		}
	}

	domain, err := r.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if r.store.available() && domain != nil {
		if err := r.setCache(ctx, code, domain); err != nil {
			logger.L(ctx).Warnw("failed to refresh scale cache from source",
				"code", code,
				"error", err,
			)
		}
	}
	return domain, nil
}

func (r *CachedScaleRepository) reloadPublishedScaleCacheFromSource(ctx context.Context, code string) (*scale.MedicalScale, error) {
	logger.L(ctx).Warnw("published scale cache has no factors, reloading from source",
		"code", code,
	)
	if r.store.available() {
		if err := r.deletePublishedScaleFamilyCache(ctx, code, ""); err != nil {
			logger.L(ctx).Warnw("failed to delete stale published scale cache",
				"code", code,
				"error", err,
			)
		}
	}

	domain, err := r.repo.FindPublishedByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if r.store.available() && domain != nil {
		if err := r.setPublishedScaleCache(ctx, code, domain); err != nil {
			logger.L(ctx).Warnw("failed to refresh published scale cache from source",
				"code", code,
				"error", err,
			)
		}
		r.warmPublishedScaleAliasCaches(ctx, domain)
	}
	return domain, nil
}

func (r *CachedScaleRepository) reloadPublishedScaleByQuestionnaireCacheFromSource(ctx context.Context, questionnaireCode string) (*scale.MedicalScale, error) {
	logger.L(ctx).Warnw("published scale cache has no factors, reloading from source",
		"questionnaire_code", questionnaireCode,
	)
	if r.store.available() {
		if err := r.deletePublishedScaleByQuestionnaireCache(ctx, questionnaireCode); err != nil {
			logger.L(ctx).Warnw("failed to delete stale published scale cache",
				"questionnaire_code", questionnaireCode,
				"error", err,
			)
		}
	}

	domain, err := r.repo.FindPublishedByQuestionnaireCode(ctx, questionnaireCode)
	if err != nil {
		return nil, err
	}
	if r.store.available() && domain != nil {
		if err := r.setPublishedScaleByQuestionnaireCache(ctx, questionnaireCode, domain); err != nil {
			logger.L(ctx).Warnw("failed to refresh published scale cache from source",
				"questionnaire_code", questionnaireCode,
				"error", err,
			)
		}
		r.warmPublishedScaleAliasCaches(ctx, domain)
	}
	return domain, nil
}

func (r *CachedScaleRepository) reloadScaleVersionCacheFromSource(ctx context.Context, code, version string) (*scale.MedicalScale, error) {
	logger.L(ctx).Warnw("published scale version cache has no factors, reloading from source",
		"code", code,
		"scale_version", version,
	)
	if r.store.available() {
		if err := r.deleteVersionCache(ctx, code, version); err != nil {
			logger.L(ctx).Warnw("failed to delete stale scale version cache",
				"code", code,
				"scale_version", version,
				"error", err,
			)
		}
	}

	domain, err := r.repo.FindByCodeVersion(ctx, code, version)
	if err != nil {
		return nil, err
	}
	if r.store.available() && domain != nil {
		if err := r.setVersionCache(ctx, code, version, domain); err != nil {
			logger.L(ctx).Warnw("failed to refresh scale version cache from source",
				"code", code,
				"scale_version", version,
				"error", err,
			)
		}
	}
	return domain, nil
}

// WarmupCache 预热缓存（批量加载量表）
func (r *CachedScaleRepository) WarmupCache(ctx context.Context, codes []string) error {
	if !r.store.available() {
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
