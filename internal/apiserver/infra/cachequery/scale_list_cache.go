package cachequery

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

const (
	defaultScaleListPageSize        = 200
	defaultScaleListCacheTTL        = 10 * time.Minute
	defaultScaleListLocalMaxEntries = 64
)

// PublishedScaleListCache 用 Redis 存储已发布量表全量列表，并提供分页读取端口。
type PublishedScaleListCache struct {
	repo        domainScale.Repository
	entry       cacheentry.Cache
	payload     *cacheentry.PayloadStore
	identitySvc iambridge.IdentityResolver
	keyBuilder  *keyspace.Builder
	policy      cachepolicy.CachePolicy
	pageSize    int
	memory      *LocalHotCache[*scalelistcache.Page]
}

func NewPublishedScaleListCacheWithPolicyAndKeyBuilder(
	entry cacheentry.Cache,
	repo domainScale.Repository,
	identitySvc iambridge.IdentityResolver,
	keyBuilder *keyspace.Builder,
	policy cachepolicy.CachePolicy,
) *PublishedScaleListCache {
	if entry == nil || repo == nil {
		return nil
	}
	if keyBuilder == nil {
		panic("cache key builder is required")
	}

	return &PublishedScaleListCache{
		repo:        repo,
		entry:       entry,
		payload:     cacheentry.NewPayloadStore(entry, cachepolicy.PolicyScaleList, policy),
		identitySvc: identitySvc,
		keyBuilder:  keyBuilder,
		policy:      policy,
		pageSize:    defaultScaleListPageSize,
		memory:      NewLocalHotCache[*scalelistcache.Page](30*time.Second, defaultScaleListLocalMaxEntries),
	}
}

func (c *PublishedScaleListCache) Rebuild(ctx context.Context) error {
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
			observability.ObserveFamilyFailure("apiserver", "static_meta", err)
			return err
		}
		observability.ObserveFamilySuccess("apiserver", "static_meta")
		return nil
	}

	items, err := c.fetchAll(ctx, conditions, total)
	if err != nil {
		return err
	}

	page := c.toPortPage(ctx, items, total)
	payload := scaleSummaryListCache{
		Scales:     toScaleSummaryCacheItems(page.Items),
		TotalCount: total,
		Page:       1,
		PageSize:   len(page.Items),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	c.resetMemory()

	if err := c.payload.Set(ctx, key, data, c.policy.TTLOr(defaultScaleListCacheTTL)); err != nil {
		observability.ObserveFamilyFailure("apiserver", "static_meta", err)
		return err
	}
	observability.ObserveFamilySuccess("apiserver", "static_meta")
	return nil
}

func (c *PublishedScaleListCache) GetPage(ctx context.Context, page, pageSize int) (*scalelistcache.Page, bool) {
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
			observability.ObserveFamilySuccess("apiserver", "static_meta")
		} else {
			observability.ObserveFamilyFailure("apiserver", "static_meta", err)
		}
		return nil, false
	}
	observability.ObserveFamilySuccess("apiserver", "static_meta")

	var payload scaleSummaryListCache
	if err := json.Unmarshal(data, &payload); err != nil {
		observability.ObserveFamilyFailure("apiserver", "static_meta", err)
		return nil, false
	}

	if page <= 0 || pageSize <= 0 {
		return nil, false
	}

	start := (page - 1) * pageSize
	if start >= len(payload.Scales) {
		return &scalelistcache.Page{
			Items: []scalelistcache.Summary{},
			Total: payload.TotalCount,
		}, true
	}

	end := start + pageSize
	if end > len(payload.Scales) {
		end = len(payload.Scales)
	}

	items := make([]scalelistcache.Summary, 0, end-start)
	for _, item := range payload.Scales[start:end] {
		items = append(items, scalelistcache.Summary{
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
			CreatedAt:         parseScaleListCacheTime(item.CreatedAt),
			UpdatedBy:         item.UpdatedBy,
			UpdatedAt:         parseScaleListCacheTime(item.UpdatedAt),
		})
	}

	result := &scalelistcache.Page{
		Items: items,
		Total: payload.TotalCount,
	}

	c.setMemory(memKey, result)

	return result, true
}

func (c *PublishedScaleListCache) fetchAll(ctx context.Context, conditions map[string]interface{}, total int64) ([]*domainScale.MedicalScale, error) {
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

func (c *PublishedScaleListCache) toPortPage(ctx context.Context, items []*domainScale.MedicalScale, total int64) *scalelistcache.Page {
	userNames := resolveScaleListUserNames(ctx, items, c.identitySvc)
	result := &scalelistcache.Page{
		Items: make([]scalelistcache.Summary, 0, len(items)),
		Total: total,
	}

	for _, item := range items {
		if item == nil {
			continue
		}
		result.Items = append(result.Items, scalelistcache.Summary{
			Code:              item.GetCode().String(),
			Title:             item.GetTitle(),
			Description:       item.GetDescription(),
			Category:          item.GetCategory().String(),
			Stages:            scaleListStageStrings(item),
			ApplicableAges:    scaleListApplicableAgeStrings(item),
			Reporters:         scaleListReporterStrings(item),
			Tags:              scaleListTagStrings(item),
			QuestionnaireCode: item.GetQuestionnaireCode().String(),
			Status:            item.GetStatus().String(),
			CreatedBy:         displayScaleListIdentityName(item.GetCreatedBy(), userNames),
			CreatedAt:         item.GetCreatedAt(),
			UpdatedBy:         displayScaleListIdentityName(item.GetUpdatedBy(), userNames),
			UpdatedAt:         item.GetUpdatedAt(),
		})
	}

	return result
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

func toScaleSummaryCacheItems(items []scalelistcache.Summary) []scaleSummaryCacheItem {
	if len(items) == 0 {
		return []scaleSummaryCacheItem{}
	}

	list := make([]scaleSummaryCacheItem, 0, len(items))
	for _, item := range items {
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
			CreatedAt:         formatScaleListCacheTime(item.CreatedAt),
			UpdatedBy:         item.UpdatedBy,
			UpdatedAt:         formatScaleListCacheTime(item.UpdatedAt),
		})
	}
	return list
}

func formatScaleListCacheTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

func parseScaleListCacheTime(val string) time.Time {
	if val == "" {
		return time.Time{}
	}
	parsed, err := time.Parse("2006-01-02 15:04:05", val)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func (c *PublishedScaleListCache) buildMemoryKey(page, pageSize int) string {
	return fmt.Sprintf("page=%d:page_size=%d", page, pageSize)
}

func (c *PublishedScaleListCache) getMemory(key string) (*scalelistcache.Page, bool) {
	if c == nil || c.memory == nil {
		return nil, false
	}
	return c.memory.Get(key)
}

func (c *PublishedScaleListCache) setMemory(key string, result *scalelistcache.Page) {
	if c == nil || c.memory == nil {
		return
	}
	c.memory.Set(key, result)
}

func (c *PublishedScaleListCache) resetMemory() {
	if c == nil || c.memory == nil {
		return
	}
	c.memory.Clear()
}

func resolveScaleListUserNames(ctx context.Context, items []*domainScale.MedicalScale, identitySvc iambridge.IdentityResolver) map[string]string {
	if identitySvc == nil || !identitySvc.IsEnabled() {
		return nil
	}
	userIDs := make([]meta.ID, 0, len(items)*2)
	for _, item := range items {
		if item == nil {
			continue
		}
		userIDs = append(userIDs, item.GetCreatedBy(), item.GetUpdatedBy())
	}
	return identitySvc.ResolveUserNames(ctx, userIDs)
}

func displayScaleListIdentityName(id meta.ID, userNames map[string]string) string {
	if id.IsZero() {
		return ""
	}
	if userNames != nil {
		if name, ok := userNames[id.String()]; ok && name != "" {
			return name
		}
	}
	return id.String()
}

func scaleListTagStrings(item *domainScale.MedicalScale) []string {
	tags := make([]string, 0, len(item.GetTags()))
	for _, tag := range item.GetTags() {
		tags = append(tags, tag.String())
	}
	return tags
}

func scaleListReporterStrings(item *domainScale.MedicalScale) []string {
	reporters := make([]string, 0, len(item.GetReporters()))
	for _, reporter := range item.GetReporters() {
		reporters = append(reporters, reporter.String())
	}
	return reporters
}

func scaleListStageStrings(item *domainScale.MedicalScale) []string {
	stages := make([]string, 0, len(item.GetStages()))
	for _, stage := range item.GetStages() {
		stages = append(stages, stage.String())
	}
	return stages
}

func scaleListApplicableAgeStrings(item *domainScale.MedicalScale) []string {
	ages := make([]string, 0, len(item.GetApplicableAges()))
	for _, age := range item.GetApplicableAges() {
		ages = append(ages, age.String())
	}
	return ages
}
