package scale

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	redis "github.com/redis/go-redis/v9"
)

const (
	defaultScaleListPageSize = 200
	defaultScaleListCacheTTL = 10 * time.Minute
)

// ScaleListCache 量表全局列表缓存重建器
// 用于在数据变更后重建 Redis 中的全局列表键（scale:list:v1）
type ScaleListCache struct {
	repo        domainScale.Repository
	redis       redis.UniversalClient
	identitySvc *iam.IdentityService
	keyBuilder  *cache.CacheKeyBuilder
	pageSize    int
	// 节点内短 TTL 内存缓存，减少 Redis GET/JSON 解码成本
	memory      map[string]memoryEntry
	memoryTTL   time.Duration
	memoryMutex sync.RWMutex
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
		memory:      make(map[string]memoryEntry),
		memoryTTL:   30 * time.Second,
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

	// 重建后清空节点内缓存
	c.resetMemory()

	return c.redis.Set(ctx, key, data, cache.JitterTTL(defaultScaleListCacheTTL)).Err()
}

// GetPage 从缓存读取已发布量表列表并按页切片
// 命中返回 result 和 true，未命中/解析失败返回 nil 和 false
func (c *ScaleListCache) GetPage(ctx context.Context, page, pageSize int) (*ScaleSummaryListResult, bool) {
	if c == nil || c.redis == nil {
		return nil, false
	}

	memKey := c.buildMemoryKey(page, pageSize)
	if result, ok := c.getMemory(memKey); ok {
		return result, true
	}

	key := c.keyBuilder.BuildScaleListKey()
	data, err := c.redis.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}

	var payload scaleSummaryListCache
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, false
	}

	if page <= 0 || pageSize <= 0 {
		return nil, false
	}

	start := (page - 1) * pageSize
	if start >= len(payload.Scales) {
		return &ScaleSummaryListResult{
			Items: []*ScaleSummaryResult{},
			Total: payload.TotalCount,
		}, true
	}

	end := start + pageSize
	if end > len(payload.Scales) {
		end = len(payload.Scales)
	}

	items := make([]*ScaleSummaryResult, 0, end-start)
	for _, item := range payload.Scales[start:end] {
		items = append(items, &ScaleSummaryResult{
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
			CreatedAt:         parseTime(item.CreatedAt),
			UpdatedBy:         item.UpdatedBy,
			UpdatedAt:         parseTime(item.UpdatedAt),
		})
	}

	result := &ScaleSummaryListResult{
		Items: items,
		Total: payload.TotalCount,
	}

	c.setMemory(memKey, result)

	return result, true
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

func parseTime(val string) time.Time {
	if val == "" {
		return time.Time{}
	}
	parsed, err := time.Parse("2006-01-02 15:04:05", val)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func logScaleListCacheError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	logger.L(ctx).Warnw("failed to rebuild scale list cache", "error", err)
}

// ==================== 节点内缓存 ====================

type memoryEntry struct {
	result *ScaleSummaryListResult
	expire time.Time
}

func (c *ScaleListCache) buildMemoryKey(page, pageSize int) string {
	return fmt.Sprintf("page=%d:page_size=%d", page, pageSize)
}

func (c *ScaleListCache) getMemory(key string) (*ScaleSummaryListResult, bool) {
	c.memoryMutex.RLock()
	entry, ok := c.memory[key]
	c.memoryMutex.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expire) {
		c.memoryMutex.Lock()
		delete(c.memory, key)
		c.memoryMutex.Unlock()
		return nil, false
	}
	return entry.result, true
}

func (c *ScaleListCache) setMemory(key string, result *ScaleSummaryListResult) {
	c.memoryMutex.Lock()
	c.memory[key] = memoryEntry{
		result: result,
		expire: time.Now().Add(c.memoryTTL),
	}
	c.memoryMutex.Unlock()
}

func (c *ScaleListCache) resetMemory() {
	c.memoryMutex.Lock()
	c.memory = make(map[string]memoryEntry)
	c.memoryMutex.Unlock()
}
