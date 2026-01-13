package scale

import (
	"context"
	"encoding/json"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	redis "github.com/redis/go-redis/v9"
)

const defaultScaleListPageSize = 200

// ScaleListCache 量表全局列表缓存重建器
// 用于在数据变更后重建 Redis 中的全局列表键（scale:list:v1）
type ScaleListCache struct {
	repo        domainScale.Repository
	redis       redis.UniversalClient
	identitySvc *iam.IdentityService
	keyBuilder  *cache.CacheKeyBuilder
	pageSize    int
}

// NewScaleListCache 创建全局量表列表缓存实例
func NewScaleListCache(redisClient redis.UniversalClient, repo domainScale.Repository, identitySvc *iam.IdentityService) *ScaleListCache {
	if redisClient == nil || repo == nil {
		return nil
	}

	return &ScaleListCache{
		repo:        repo,
		redis:       redisClient,
		identitySvc: identitySvc,
		keyBuilder:  cache.NewCacheKeyBuilder(),
		pageSize:    defaultScaleListPageSize,
	}
}

// Rebuild 重新拉取已发布量表列表并写入缓存
// 若列表为空则删除缓存键
func (c *ScaleListCache) Rebuild(ctx context.Context) error {
	if c == nil || c.redis == nil || c.repo == nil {
		return nil
	}

	conditions := map[string]interface{}{
		"status": domainScale.StatusPublished.Value(),
	}

	total, err := c.repo.CountWithConditions(ctx, conditions)
	if err != nil {
		return err
	}

	key := c.keyBuilder.BuildScaleListKey()
	if total == 0 {
		return c.redis.Del(ctx, key).Err()
	}

	items, err := c.fetchAll(ctx, conditions, total)
	if err != nil {
		return err
	}

	summary := toSummaryListResult(ctx, items, total, c.identitySvc)
	payload := scaleSummaryListCache{
		Scales:     toScaleSummaryCacheItems(summary.Items),
		TotalCount: total,
		Page:       1,
		PageSize:   len(summary.Items),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return c.redis.Set(ctx, key, data, 0).Err()
}

func (c *ScaleListCache) fetchAll(ctx context.Context, conditions map[string]interface{}, total int64) ([]*domainScale.MedicalScale, error) {
	all := make([]*domainScale.MedicalScale, 0, int(total))
	for page := 1; int64(len(all)) < total; page++ {
		items, err := c.repo.FindSummaryList(ctx, page, c.pageSize, conditions)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		all = append(all, items...)
		if len(items) < c.pageSize {
			break
		}
	}
	return all, nil
}

type scaleSummaryListCache struct {
	Scales     []scaleSummaryCacheItem `json:"scales"`
	TotalCount int64                   `json:"total_count"`
	Page       int                     `json:"page"`
	PageSize   int                     `json:"page_size"`
}

type scaleSummaryCacheItem struct {
	Code              string   `json:"code"`
	Title             string   `json:"title"`
	Description       string   `json:"description"`
	Category          string   `json:"category,omitempty"`
	Stages            []string `json:"stages,omitempty"`
	ApplicableAges    []string `json:"applicable_ages,omitempty"`
	Reporters         []string `json:"reporters,omitempty"`
	Tags              []string `json:"tags,omitempty"`
	QuestionnaireCode string   `json:"questionnaire_code"`
	Status            string   `json:"status"`
	CreatedBy         string   `json:"created_by"`
	CreatedAt         string   `json:"created_at"`
	UpdatedBy         string   `json:"updated_by"`
	UpdatedAt         string   `json:"updated_at"`
}

func toScaleSummaryCacheItems(items []*ScaleSummaryResult) []scaleSummaryCacheItem {
	if len(items) == 0 {
		return []scaleSummaryCacheItem{}
	}

	list := make([]scaleSummaryCacheItem, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		list = append(list, scaleSummaryCacheItem{
			Code:              item.Code,
			Title:             item.Title,
			Description:       item.Description,
			Category:          item.Category,
			Stages:            item.Stages,
			ApplicableAges:    item.ApplicableAges,
			Reporters:         item.Reporters,
			Tags:              item.Tags,
			QuestionnaireCode: item.QuestionnaireCode,
			Status:            item.Status,
			CreatedBy:         item.CreatedBy,
			CreatedAt:         formatTime(item.CreatedAt),
			UpdatedBy:         item.UpdatedBy,
			UpdatedAt:         formatTime(item.UpdatedAt),
		})
	}
	return list
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

func logScaleListCacheError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	logger.L(ctx).Warnw("failed to rebuild scale list cache", "error", err)
}
