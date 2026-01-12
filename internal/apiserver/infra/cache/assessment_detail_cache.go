package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	assessmentInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	redis "github.com/redis/go-redis/v9"
)

const (
	// AssessmentDetailCachePrefix 测评详情缓存键前缀
	AssessmentDetailCachePrefix = "assessment:detail:"
)

// DefaultAssessmentDetailCacheTTL 默认测评详情缓存 TTL（可被配置覆盖）
var DefaultAssessmentDetailCacheTTL = 2 * time.Hour

// CachedAssessmentRepository 带缓存的测评 Repository 装饰器
// 实现 assessment.Repository 接口，在原有 Repository 基础上添加 Redis 缓存层
type CachedAssessmentRepository struct {
	repo   assessment.Repository
	client redis.UniversalClient
	ttl    time.Duration
	mapper *assessmentInfra.AssessmentMapper
}

// NewCachedAssessmentRepository 创建带缓存的测评 Repository
// 如果 client 为 nil，则降级为直接调用 repo（不缓存）
func NewCachedAssessmentRepository(repo assessment.Repository, client redis.UniversalClient) assessment.Repository {
	return &CachedAssessmentRepository{
		repo:   repo,
		client: client,
		ttl:    DefaultAssessmentDetailCacheTTL,
		mapper: assessmentInfra.NewAssessmentMapper(),
	}
}

// buildCacheKey 构建缓存键
func (r *CachedAssessmentRepository) buildCacheKey(id assessment.ID) string {
	return fmt.Sprintf("%s%d", AssessmentDetailCachePrefix, id.Uint64())
}

// FindByID 根据ID查询测评（优先从缓存读取）
func (r *CachedAssessmentRepository) FindByID(ctx context.Context, id assessment.ID) (*assessment.Assessment, error) {
	// 1. 尝试从缓存读取
	if r.client != nil {
		if cached, err := r.getCache(ctx, id); err == nil && cached != nil {
			logger.L(ctx).Debugw("从Redis缓存获取测评详情", "assessment_id", id.Uint64())
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
			logger.L(ctx).Warnw("写入测评详情缓存失败", "assessment_id", id.Uint64(), "error", err.Error())
		}
	}

	return domain, nil
}

// Save 保存测评（同时失效缓存）
func (r *CachedAssessmentRepository) Save(ctx context.Context, domain *assessment.Assessment) error {
	err := r.repo.Save(ctx, domain)
	if err == nil && domain != nil {
		// 保存成功后失效缓存，确保下次读取最新数据
		r.deleteCache(ctx, domain.ID())
	}
	return err
}

// Delete 删除测评（同时失效缓存）
func (r *CachedAssessmentRepository) Delete(ctx context.Context, id assessment.ID) error {
	err := r.repo.Delete(ctx, id)
	if err == nil {
		r.deleteCache(ctx, id)
	}
	return err
}

// getCache 从缓存获取
func (r *CachedAssessmentRepository) getCache(ctx context.Context, id assessment.ID) (*assessment.Assessment, error) {
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

	var po assessmentInfra.AssessmentPO
	if err := json.Unmarshal(cachedData, &po); err != nil {
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

	return r.client.Set(ctx, key, data, JitterTTL(r.ttl)).Err()
}

// deleteCache 删除缓存
func (r *CachedAssessmentRepository) deleteCache(ctx context.Context, id assessment.ID) error {
	if r.client == nil {
		return nil
	}

	key := r.buildCacheKey(id)
	return r.client.Del(ctx, key).Err()
}

// 实现其他 Repository 方法（透传，不缓存）
func (r *CachedAssessmentRepository) FindByAnswerSheetID(ctx context.Context, answerSheetID assessment.AnswerSheetRef) (*assessment.Assessment, error) {
	return r.repo.FindByAnswerSheetID(ctx, answerSheetID)
}

func (r *CachedAssessmentRepository) FindByTesteeID(ctx context.Context, testeeID testee.ID, pagination assessment.Pagination) ([]*assessment.Assessment, int64, error) {
	return r.repo.FindByTesteeID(ctx, testeeID, pagination)
}

func (r *CachedAssessmentRepository) FindByTesteeIDAndScaleID(ctx context.Context, testeeID testee.ID, scaleRef assessment.MedicalScaleRef, pagination assessment.Pagination) ([]*assessment.Assessment, int64, error) {
	return r.repo.FindByTesteeIDAndScaleID(ctx, testeeID, scaleRef, pagination)
}

func (r *CachedAssessmentRepository) FindByPlanID(ctx context.Context, planID string, pagination assessment.Pagination) ([]*assessment.Assessment, int64, error) {
	return r.repo.FindByPlanID(ctx, planID, pagination)
}

func (r *CachedAssessmentRepository) FindByScreeningProjectID(ctx context.Context, screeningProjectID string, pagination assessment.Pagination) ([]*assessment.Assessment, int64, error) {
	return r.repo.FindByScreeningProjectID(ctx, screeningProjectID, pagination)
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

func (r *CachedAssessmentRepository) FindPendingSubmission(ctx context.Context, pagination assessment.Pagination) ([]*assessment.Assessment, int64, error) {
	return r.repo.FindPendingSubmission(ctx, pagination)
}
