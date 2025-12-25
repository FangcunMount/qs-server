package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

const (
	AssessmentStatusCacheTTL = 30 * time.Minute
)

// AssessmentStatusCache 测评状态缓存
// 使用 Write-Through 模式，确保强一致性
type AssessmentStatusCache struct {
	cache      Cache
	keyBuilder *CacheKeyBuilder
}

// NewAssessmentStatusCache 创建测评状态缓存
func NewAssessmentStatusCache(cache Cache) *AssessmentStatusCache {
	return &AssessmentStatusCache{
		cache:      cache,
		keyBuilder: NewCacheKeyBuilder(),
	}
}

// StatusCacheValue 状态缓存值
type StatusCacheValue struct {
	Status        string     `json:"status"`
	SubmittedAt   *time.Time `json:"submitted_at,omitempty"`
	InterpretedAt *time.Time `json:"interpreted_at,omitempty"`
	FailedAt      *time.Time `json:"failed_at,omitempty"`
	TotalScore    *float64   `json:"total_score,omitempty"`
	RiskLevel     *string    `json:"risk_level,omitempty"`
}

// Get 获取测评状态（Read-Through）
func (c *AssessmentStatusCache) Get(ctx context.Context, id assessment.ID, loadFunc func() (*assessment.Assessment, error)) (*StatusCacheValue, error) {
	key := c.keyBuilder.BuildAssessmentStatusKey(id.Uint64())

	// 先查缓存
	if c.cache != nil {
		cachedData, err := c.cache.Get(ctx, key)
		if err == nil {
			var value StatusCacheValue
			if err := json.Unmarshal(cachedData, &value); err == nil {
				logger.L(ctx).Debugw("从缓存获取测评状态", "assessment_id", id.Uint64())
				return &value, nil
			}
			logger.L(ctx).Warnw("反序列化测评状态缓存失败", "assessment_id", id.Uint64(), "error", err.Error())
		} else if err != ErrCacheNotFound {
			logger.L(ctx).Warnw("从缓存获取测评状态失败", "assessment_id", id.Uint64(), "error", err.Error())
		}
	}

	// 缓存未命中，从数据库加载
	assessmentObj, err := loadFunc()
	if err != nil {
		return nil, err
	}
	if assessmentObj == nil {
		return nil, fmt.Errorf("assessment not found")
	}

	// 构建缓存值
	value := &StatusCacheValue{
		Status:        assessmentObj.Status().String(),
		SubmittedAt:   assessmentObj.SubmittedAt(),
		InterpretedAt: assessmentObj.InterpretedAt(),
		FailedAt:      assessmentObj.FailedAt(),
		TotalScore:    assessmentObj.TotalScore(),
	}
	if assessmentObj.RiskLevel() != nil {
		riskLevel := assessmentObj.RiskLevel().String()
		value.RiskLevel = &riskLevel
	}

	// 写入缓存（Write-Through）
	if err := c.Set(ctx, id, value); err != nil {
		logger.L(ctx).Warnw("写入测评状态缓存失败", "assessment_id", id.Uint64(), "error", err.Error())
	}

	return value, nil
}

// Set 设置测评状态（Write-Through）
func (c *AssessmentStatusCache) Set(ctx context.Context, id assessment.ID, value *StatusCacheValue) error {
	if c.cache == nil {
		return nil // 缓存未启用，直接返回
	}

	key := c.keyBuilder.BuildAssessmentStatusKey(id.Uint64())

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal status cache value: %w", err)
	}

	return c.cache.Set(ctx, key, data, AssessmentStatusCacheTTL)
}

// Update 更新测评状态（Write-Through）
// 在更新数据库后调用，确保缓存与数据库一致
func (c *AssessmentStatusCache) Update(ctx context.Context, assessment *assessment.Assessment) error {
	value := &StatusCacheValue{
		Status:        assessment.Status().String(),
		SubmittedAt:   assessment.SubmittedAt(),
		InterpretedAt: assessment.InterpretedAt(),
		FailedAt:      assessment.FailedAt(),
		TotalScore:    assessment.TotalScore(),
	}
	if assessment.RiskLevel() != nil {
		riskLevel := assessment.RiskLevel().String()
		value.RiskLevel = &riskLevel
	}

	return c.Set(ctx, assessment.ID(), value)
}

// Delete 删除测评状态缓存
func (c *AssessmentStatusCache) Delete(ctx context.Context, id assessment.ID) error {
	if c.cache == nil {
		return nil // 缓存未启用，直接返回
	}

	key := c.keyBuilder.BuildAssessmentStatusKey(id.Uint64())
	return c.cache.Delete(ctx, key)
}
