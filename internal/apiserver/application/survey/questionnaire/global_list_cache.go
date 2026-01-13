package questionnaire

import (
	"context"
	"encoding/json"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	redis "github.com/redis/go-redis/v9"
)

const defaultQuestionnaireListPageSize = 200

// QuestionnaireListCache 问卷全局列表缓存重建器
// 用于在数据变更后重建 Redis 中的全局列表键（questionnaire:list:v1）
type QuestionnaireListCache struct {
	repo        domainQuestionnaire.Repository
	redis       redis.UniversalClient
	identitySvc *iam.IdentityService
	keyBuilder  *cache.CacheKeyBuilder
	pageSize    int
}

// NewQuestionnaireListCache 创建问卷全局列表缓存实例
func NewQuestionnaireListCache(redisClient redis.UniversalClient, repo domainQuestionnaire.Repository, identitySvc *iam.IdentityService) *QuestionnaireListCache {
	if redisClient == nil || repo == nil {
		return nil
	}

	return &QuestionnaireListCache{
		repo:        repo,
		redis:       redisClient,
		identitySvc: identitySvc,
		keyBuilder:  cache.NewCacheKeyBuilder(),
		pageSize:    defaultQuestionnaireListPageSize,
	}
}

// Rebuild 重新拉取已发布问卷列表并写入缓存
// 若列表为空则删除缓存键
func (c *QuestionnaireListCache) Rebuild(ctx context.Context) error {
	if c == nil || c.redis == nil || c.repo == nil {
		return nil
	}

	conditions := map[string]interface{}{
		"status": domainQuestionnaire.STATUS_PUBLISHED.String(),
	}

	total, err := c.repo.CountWithConditions(ctx, conditions)
	if err != nil {
		return err
	}

	key := c.keyBuilder.BuildQuestionnaireListKey()
	if total == 0 {
		return c.redis.Del(ctx, key).Err()
	}

	items, err := c.fetchAll(ctx, conditions, total)
	if err != nil {
		return err
	}

	summary := toQuestionnaireSummaryListResult(ctx, items, total, c.identitySvc)
	payload := questionnaireSummaryListCache{
		Questionnaires: toQuestionnaireSummaryCacheItems(summary.Items),
		TotalCount:     total,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return c.redis.Set(ctx, key, data, 0).Err()
}

func (c *QuestionnaireListCache) fetchAll(ctx context.Context, conditions map[string]interface{}, total int64) ([]*domainQuestionnaire.Questionnaire, error) {
	all := make([]*domainQuestionnaire.Questionnaire, 0, int(total))
	for page := 1; int64(len(all)) < total; page++ {
		items, err := c.repo.FindBaseList(ctx, page, c.pageSize, conditions)
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

type questionnaireSummaryListCache struct {
	Questionnaires []questionnaireSummaryCacheItem `json:"questionnaires"`
	TotalCount     int64                           `json:"total_count"`
}

type questionnaireSummaryCacheItem struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ImgUrl      string `json:"img_url"`
	Version     string `json:"version"`
	Status      string `json:"status"`
	Type        string `json:"type"`
	CreatedBy   string `json:"created_by"`
	CreatedAt   string `json:"created_at"`
	UpdatedBy   string `json:"updated_by"`
	UpdatedAt   string `json:"updated_at"`
}

func toQuestionnaireSummaryCacheItems(items []*QuestionnaireSummaryResult) []questionnaireSummaryCacheItem {
	if len(items) == 0 {
		return []questionnaireSummaryCacheItem{}
	}

	list := make([]questionnaireSummaryCacheItem, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		list = append(list, questionnaireSummaryCacheItem{
			Code:        item.Code,
			Title:       item.Title,
			Description: item.Description,
			ImgUrl:      item.ImgUrl,
			Version:     item.Version,
			Status:      item.Status,
			Type:        item.Type,
			CreatedBy:   item.CreatedBy,
			CreatedAt:   formatTime(item.CreatedAt),
			UpdatedBy:   item.UpdatedBy,
			UpdatedAt:   formatTime(item.UpdatedAt),
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

func logQuestionnaireListCacheError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	logger.L(ctx).Warnw("failed to rebuild questionnaire list cache", "error", err)
}
