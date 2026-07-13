package evaluationcache

import (
	"context"
	"encoding/json"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/internal/adapterkit"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	assessmentInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	redis "github.com/redis/go-redis/v9"
)

// CachedAssessmentRepository 带缓存的测评 Repository 装饰器
// 实现 assessment.Repository 接口，在原有 Repository 基础上添加 Redis 缓存层
type CachedAssessmentRepository struct {
	repo     assessment.Repository
	keys     *keyspace.Builder
	policies sharedcache.PolicyProvider
	observer *observability.ComponentObserver
	store    *adapterkit.ObjectCacheStore[assessment.Assessment]
}

// NewCachedAssessmentRepositoryWithBuilderAndPolicy 创建带显式 builder/policy 的测评缓存 Repository。
func NewCachedAssessmentRepositoryWithBuilderAndProvider(repo assessment.Repository, client redis.UniversalClient, builder *keyspace.Builder, policies sharedcache.PolicyProvider) assessment.Repository {
	return NewCachedAssessmentRepositoryWithBuilderProviderAndObserver(repo, client, builder, policies, nil)
}

func NewCachedAssessmentRepositoryWithBuilderProviderAndObserver(repo assessment.Repository, client redis.UniversalClient, builder *keyspace.Builder, policies sharedcache.PolicyProvider, observer *observability.ComponentObserver) assessment.Repository {
	if builder == nil {
		panic("redis builder is required")
	}
	mapper := assessmentInfra.NewAssessmentMapper()
	return &CachedAssessmentRepository{
		repo:     repo,
		keys:     builder,
		policies: policies,
		observer: observer,
		store: adapterkit.NewObjectCacheStore(adapterkit.ObjectCacheStoreOptions[assessment.Assessment]{
			Cache:     adapterkit.NewRedisStoreIfAvailable(client),
			PolicyKey: cachepolicy.CapabilityEvaluationAssessmentDetail,
			Codec:     newAssessmentCacheEntryCodec(mapper),
		}),
	}
}

func newAssessmentCacheEntryCodec(mapper *assessmentInfra.AssessmentMapper) adapterkit.CacheEntryCodec[assessment.Assessment] {
	return adapterkit.CacheEntryCodec[assessment.Assessment]{
		EncodeFunc: func(domain *assessment.Assessment) ([]byte, error) {
			return json.Marshal(mapper.ToPO(domain))
		},
		DecodeFunc: func(data []byte) (*assessment.Assessment, error) {
			var po assessmentInfra.AssessmentPO
			if err := json.Unmarshal(data, &po); err != nil {
				return nil, err
			}
			return mapper.ToDomain(&po), nil
		},
	}
}

// buildCacheKey 构建缓存键
func (r *CachedAssessmentRepository) buildCacheKey(id assessment.ID) string {
	return r.keys.BuildAssessmentDetailKey(id.Uint64())
}

// FindByID 根据ID查询测评（优先从缓存读取）
func (r *CachedAssessmentRepository) FindByID(ctx context.Context, id assessment.ID) (*assessment.Assessment, error) {
	domain, err := adapterkit.ReadThroughObject(ctx, adapterkit.ObjectReadThroughOptions[assessment.Assessment]{
		PolicyKey:      cachepolicy.CapabilityEvaluationAssessmentDetail,
		CacheKey:       r.buildCacheKey(id),
		PolicyProvider: r.policies,
		Observer:       r.observer,
		Store:          r.store,
		Load:           func(ctx context.Context) (*assessment.Assessment, error) { return r.repo.FindByID(ctx, id) },
	})
	if err != nil {
		return nil, err
	}
	return domain, nil
}

// Save 保存测评（同时失效缓存）
func (r *CachedAssessmentRepository) Save(ctx context.Context, domain *assessment.Assessment) error {
	err := r.repo.Save(ctx, domain)
	if err == nil && domain != nil {
		// 保存成功后失效缓存，确保下次读取最新数据
		_ = r.deleteCache(ctx, domain.ID())
	}
	return err
}

// Delete 删除测评（同时失效缓存）
func (r *CachedAssessmentRepository) Delete(ctx context.Context, id assessment.ID) error {
	err := r.repo.Delete(ctx, id)
	if err == nil {
		_ = r.deleteCache(ctx, id)
	}
	return err
}

// deleteCache 删除缓存
func (r *CachedAssessmentRepository) deleteCache(ctx context.Context, id assessment.ID) error {
	return r.store.Delete(ctx, r.buildCacheKey(id))
}

// 实现其他 Repository 方法（透传，不缓存）
func (r *CachedAssessmentRepository) FindByAnswerSheetID(ctx context.Context, answerSheetID assessment.AnswerSheetRef) (*assessment.Assessment, error) {
	return r.repo.FindByAnswerSheetID(ctx, answerSheetID)
}
