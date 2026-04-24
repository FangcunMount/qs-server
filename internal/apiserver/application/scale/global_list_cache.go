package scale

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
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
	entry       cacheentry.Cache
	payload     *cacheentry.PayloadStore
	identitySvc iambridge.IdentityResolver
	keyBuilder  *rediskey.Builder
	policy      cachepolicy.CachePolicy
	pageSize    int
	// 节点内短 TTL 内存缓存，减少 Redis GET/JSON 解码成本
	memory *cachequery.LocalHotCache[*ScaleSummaryListResult]
}

const defaultScaleListLocalMaxEntries = 64

// NewScaleListCacheWithPolicyAndKeyBuilder 创建带显式 key builder/policy 的全局量表列表缓存实例。
func NewScaleListCacheWithPolicyAndKeyBuilder(
	redisClient redis.UniversalClient,
	repo domainScale.Repository,
	identitySvc iambridge.IdentityResolver,
	keyBuilder *rediskey.Builder,
	policy cachepolicy.CachePolicy,
) *ScaleListCache {
	if redisClient == nil || repo == nil {
		return nil
	}
	if keyBuilder == nil {
		panic("cache key builder is required")
	}
	entry := cacheentry.NewRedisCache(redisClient)

	return &ScaleListCache{
		repo:        repo,
		entry:       entry,
		payload:     cacheentry.NewPayloadStore(entry, cachepolicy.PolicyScaleList, policy),
		identitySvc: identitySvc,
		keyBuilder:  keyBuilder,
		policy:      policy,
		pageSize:    defaultScaleListPageSize,
		memory:      cachequery.NewLocalHotCache[*ScaleSummaryListResult](30*time.Second, defaultScaleListLocalMaxEntries),
	}
}

// Rebuild 重新拉取已发布量表列表并写入缓存
// 若列表为空则删除缓存键
func (c *ScaleListCache) Rebuild(ctx context.Context) error {
	if c == nil || c.entry == nil || c.payload == nil || c.repo == nil {
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
		if err := c.entry.Delete(ctx, key); err != nil {
			cacheobservability.ObserveFamilyFailure("apiserver", "static_meta", err)
			return err
		}
		cacheobservability.ObserveFamilySuccess("apiserver", "static_meta")
		return nil
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

	c.resetMemory()

	if err := c.payload.Set(ctx, key, data, c.policy.TTLOr(defaultScaleListCacheTTL)); err != nil {
		cacheobservability.ObserveFamilyFailure("apiserver", "static_meta", err)
		return err
	}
	cacheobservability.ObserveFamilySuccess("apiserver", "static_meta")
	return nil
}

// GetPage 从缓存读取已发布量表列表并按页切片
// 命中返回 result 和 true，未命中/解析失败返回 nil 和 false
func (c *ScaleListCache) GetPage(ctx context.Context, page, pageSize int) (*ScaleSummaryListResult, bool) {
	if c == nil || c.payload == nil {
		return nil, false
	}

	memKey := c.buildMemoryKey(page, pageSize)
	if result, ok := c.getMemory(memKey); ok {
		return result, true
	}

	key := c.keyBuilder.BuildScaleListKey()
	data, err := c.payload.Get(ctx, key)
	if err != nil {
		if err == cacheentry.ErrCacheNotFound {
			cacheobservability.ObserveFamilySuccess("apiserver", "static_meta")
		} else {
			cacheobservability.ObserveFamilyFailure("apiserver", "static_meta", err)
		}
		return nil, false
	}
	cacheobservability.ObserveFamilySuccess("apiserver", "static_meta")

	var payload scaleSummaryListCache
	if err := json.Unmarshal(data, &payload); err != nil {
		cacheobservability.ObserveFamilyFailure("apiserver", "static_meta", err)
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

func (c *ScaleListCache) buildMemoryKey(page, pageSize int) string {
	return fmt.Sprintf("page=%d:page_size=%d", page, pageSize)
}

func (c *ScaleListCache) getMemory(key string) (*ScaleSummaryListResult, bool) {
	if c == nil || c.memory == nil {
		return nil, false
	}
	return c.memory.Get(key)
}

func (c *ScaleListCache) setMemory(key string, result *ScaleSummaryListResult) {
	if c == nil || c.memory == nil {
		return
	}
	c.memory.Set(key, result)
}

func (c *ScaleListCache) resetMemory() {
	if c == nil || c.memory == nil {
		return
	}
	c.memory.Clear()
}
