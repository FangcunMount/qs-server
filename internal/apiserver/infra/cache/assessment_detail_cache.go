package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	assessmentInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/pkg/event"
	redis "github.com/redis/go-redis/v9"
)

const defaultAssessmentDetailCacheTTL = 2 * time.Hour

// CachedAssessmentRepository 带缓存的测评 Repository 装饰器
// 实现 assessment.Repository 接口，在原有 Repository 基础上添加 Redis 缓存层
type CachedAssessmentRepository struct {
	repo     assessment.Repository
	client   redis.UniversalClient
	ttl      time.Duration
	mapper   *assessmentInfra.AssessmentMapper
	keys     *rediskey.Builder
	policy   cachepolicy.CachePolicy
	observer *Observer
}

// NewCachedAssessmentRepositoryWithBuilderAndPolicy 创建带显式 builder/policy 的测评缓存 Repository。
func NewCachedAssessmentRepositoryWithBuilderAndPolicy(repo assessment.Repository, client redis.UniversalClient, builder *rediskey.Builder, policy cachepolicy.CachePolicy) assessment.Repository {
	return NewCachedAssessmentRepositoryWithBuilderPolicyAndObserver(repo, client, builder, policy, nil)
}

func NewCachedAssessmentRepositoryWithBuilderPolicyAndObserver(repo assessment.Repository, client redis.UniversalClient, builder *rediskey.Builder, policy cachepolicy.CachePolicy, observer *Observer) assessment.Repository {
	if builder == nil {
		panic("redis builder is required")
	}
	return &CachedAssessmentRepository{
		repo:     repo,
		client:   client,
		ttl:      policy.TTLOr(defaultAssessmentDetailCacheTTL),
		mapper:   assessmentInfra.NewAssessmentMapper(),
		keys:     builder,
		policy:   policy,
		observer: observer,
	}
}

// buildCacheKey 构建缓存键
func (r *CachedAssessmentRepository) buildCacheKey(id assessment.ID) string {
	return r.keys.BuildAssessmentDetailKey(id.Uint64())
}

// FindByID 根据ID查询测评（优先从缓存读取）
func (r *CachedAssessmentRepository) FindByID(ctx context.Context, id assessment.ID) (*assessment.Assessment, error) {
	domain, err := readByIDWithCache(
		ctx,
		cachepolicy.PolicyAssessmentDetail,
		r.buildCacheKey(id),
		r.policy,
		r.observer,
		func(ctx context.Context) (*assessment.Assessment, error) { return r.getCache(ctx, id) },
		func(ctx context.Context) (*assessment.Assessment, error) { return r.repo.FindByID(ctx, id) },
		func(ctx context.Context, value *assessment.Assessment) error { return r.setCache(ctx, id, value) },
	)
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

// SaveWithEvents 保存测评并暂存事件（同时失效缓存）。
func (r *CachedAssessmentRepository) SaveWithEvents(ctx context.Context, domain *assessment.Assessment) error {
	err := r.repo.SaveWithEvents(ctx, domain)
	if err == nil && domain != nil {
		_ = r.deleteCache(ctx, domain.ID())
	}
	return err
}

func (r *CachedAssessmentRepository) SaveWithAdditionalEvents(ctx context.Context, domain *assessment.Assessment, additional []event.DomainEvent) error {
	err := r.repo.SaveWithAdditionalEvents(ctx, domain, additional)
	if err == nil && domain != nil {
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

// getCache 从缓存获取
func (r *CachedAssessmentRepository) getCache(ctx context.Context, id assessment.ID) (*assessment.Assessment, error) {
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
	observePayload(cachepolicy.PolicyAssessmentDetail, len(data), len(cachedData))
	var po assessmentInfra.AssessmentPO
	if err := json.Unmarshal(data, &po); err != nil {
		return nil, err
	}

	return r.mapper.ToDomain(&po), nil
}

// setCache 写入缓存
func (r *CachedAssessmentRepository) setCache(ctx context.Context, id assessment.ID, domain *assessment.Assessment) error {
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
	observePayload(cachepolicy.PolicyAssessmentDetail, len(data), len(payload))
	return r.client.Set(ctx, key, payload, r.policy.JitterTTL(r.ttl)).Err()
}

// deleteCache 删除缓存
func (r *CachedAssessmentRepository) deleteCache(ctx context.Context, id assessment.ID) error {
	if r.client == nil {
		return nil
	}

	key := r.buildCacheKey(id)
	err := r.client.Del(ctx, key).Err()
	if err != nil {
		observeInvalidate(cachepolicy.PolicyAssessmentDetail, "error")
		return err
	}
	observeInvalidate(cachepolicy.PolicyAssessmentDetail, "ok")
	return nil
}

// 实现其他 Repository 方法（透传，不缓存）
func (r *CachedAssessmentRepository) FindByAnswerSheetID(ctx context.Context, answerSheetID assessment.AnswerSheetRef) (*assessment.Assessment, error) {
	return r.repo.FindByAnswerSheetID(ctx, answerSheetID)
}

func (r *CachedAssessmentRepository) FindByTesteeID(ctx context.Context, testeeID testee.ID, pagination assessment.Pagination) ([]*assessment.Assessment, int64, error) {
	return r.repo.FindByTesteeID(ctx, testeeID, pagination)
}

func (r *CachedAssessmentRepository) FindByTesteeIDWithFilters(
	ctx context.Context,
	testeeID testee.ID,
	status string,
	scaleCode string,
	riskLevel string,
	dateFrom *time.Time,
	dateTo *time.Time,
	pagination assessment.Pagination,
) ([]*assessment.Assessment, int64, error) {
	return r.repo.FindByTesteeIDWithFilters(ctx, testeeID, status, scaleCode, riskLevel, dateFrom, dateTo, pagination)
}

func (r *CachedAssessmentRepository) FindByTesteeIDAndScaleID(ctx context.Context, testeeID testee.ID, scaleRef assessment.MedicalScaleRef, pagination assessment.Pagination) ([]*assessment.Assessment, int64, error) {
	return r.repo.FindByTesteeIDAndScaleID(ctx, testeeID, scaleRef, pagination)
}

func (r *CachedAssessmentRepository) FindByPlanID(ctx context.Context, planID string, pagination assessment.Pagination) ([]*assessment.Assessment, int64, error) {
	return r.repo.FindByPlanID(ctx, planID, pagination)
}

func (r *CachedAssessmentRepository) CountByStatus(ctx context.Context, status assessment.Status) (int64, error) {
	return r.repo.CountByStatus(ctx, status)
}

func (r *CachedAssessmentRepository) CountByTesteeIDAndStatus(ctx context.Context, testeeID testee.ID, status assessment.Status) (int64, error) {
	return r.repo.CountByTesteeIDAndStatus(ctx, testeeID, status)
}

func (r *CachedAssessmentRepository) CountByOrgIDAndStatus(ctx context.Context, orgID int64, status assessment.Status) (int64, error) {
	return r.repo.CountByOrgIDAndStatus(ctx, orgID, status)
}

func (r *CachedAssessmentRepository) FindByIDs(ctx context.Context, ids []assessment.ID) ([]*assessment.Assessment, error) {
	return r.repo.FindByIDs(ctx, ids)
}

func (r *CachedAssessmentRepository) FindByOrgID(ctx context.Context, orgID int64, status *assessment.Status, pagination assessment.Pagination) ([]*assessment.Assessment, int64, error) {
	return r.repo.FindByOrgID(ctx, orgID, status, pagination)
}

func (r *CachedAssessmentRepository) FindByOrgIDAndTesteeIDs(
	ctx context.Context,
	orgID int64,
	testeeIDs []testee.ID,
	status *assessment.Status,
	pagination assessment.Pagination,
) ([]*assessment.Assessment, int64, error) {
	return r.repo.FindByOrgIDAndTesteeIDs(ctx, orgID, testeeIDs, status, pagination)
}

func (r *CachedAssessmentRepository) FindPendingSubmission(ctx context.Context, pagination assessment.Pagination) ([]*assessment.Assessment, int64, error) {
	return r.repo.FindPendingSubmission(ctx, pagination)
}
