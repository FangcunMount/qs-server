package statistics

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

func (c *StatisticsCache) LoadSystemStatistics(ctx context.Context, orgID int64) (*domainStatistics.SystemStatistics, bool) {
	var stats domainStatistics.SystemStatistics
	if ok := c.loadJSON(ctx, systemStatsCacheKey(orgID), "系统统计", &stats); !ok {
		return nil, false
	}
	return &stats, true
}

func (c *StatisticsCache) StoreSystemStatistics(ctx context.Context, orgID int64, stats *domainStatistics.SystemStatistics) {
	c.storeJSON(ctx, systemStatsCacheKey(orgID), "系统统计", stats)
}

func (c *StatisticsCache) LoadQuestionnaireStatistics(ctx context.Context, orgID int64, questionnaireCode string) (*domainStatistics.QuestionnaireStatistics, bool) {
	var stats domainStatistics.QuestionnaireStatistics
	if ok := c.loadJSON(ctx, questionnaireStatsCacheKey(orgID, questionnaireCode), "问卷统计", &stats); !ok {
		return nil, false
	}
	return &stats, true
}

func (c *StatisticsCache) StoreQuestionnaireStatistics(ctx context.Context, orgID int64, questionnaireCode string, stats *domainStatistics.QuestionnaireStatistics) {
	c.storeJSON(ctx, questionnaireStatsCacheKey(orgID, questionnaireCode), "问卷统计", stats)
}

func (c *StatisticsCache) LoadTesteeStatistics(ctx context.Context, orgID int64, testeeID uint64) (*domainStatistics.TesteeStatistics, bool) {
	var stats domainStatistics.TesteeStatistics
	if ok := c.loadJSON(ctx, testeeStatsCacheKey(orgID, testeeID), "受试者统计", &stats); !ok {
		return nil, false
	}
	return &stats, true
}

func (c *StatisticsCache) StoreTesteeStatistics(ctx context.Context, orgID int64, testeeID uint64, stats *domainStatistics.TesteeStatistics) {
	c.storeJSON(ctx, testeeStatsCacheKey(orgID, testeeID), "受试者统计", stats)
}

func (c *StatisticsCache) LoadPlanStatistics(ctx context.Context, orgID int64, planID uint64) (*domainStatistics.PlanStatistics, bool) {
	var stats domainStatistics.PlanStatistics
	if ok := c.loadJSON(ctx, planStatsCacheKey(orgID, planID), "计划统计", &stats); !ok {
		return nil, false
	}
	return &stats, true
}

func (c *StatisticsCache) StorePlanStatistics(ctx context.Context, orgID int64, planID uint64, stats *domainStatistics.PlanStatistics) {
	c.storeJSON(ctx, planStatsCacheKey(orgID, planID), "计划统计", stats)
}

func (c *StatisticsCache) LoadOverview(ctx context.Context, orgID int64, timeRange domainStatistics.StatisticsTimeRange) (*domainStatistics.StatisticsOverview, bool) {
	var stats domainStatistics.StatisticsOverview
	if ok := c.loadJSON(ctx, overviewStatsCacheKey(orgID, timeRange), "统计概览", &stats); !ok {
		return nil, false
	}
	return &stats, true
}

func (c *StatisticsCache) StoreOverview(ctx context.Context, orgID int64, timeRange domainStatistics.StatisticsTimeRange, stats *domainStatistics.StatisticsOverview) {
	c.storeJSON(ctx, overviewStatsCacheKey(orgID, timeRange), "统计概览", stats)
}

func (c *StatisticsCache) loadJSON(ctx context.Context, cacheKey, label string, target interface{}) bool {
	if c == nil || strings.TrimSpace(cacheKey) == "" {
		return false
	}
	cached, err := c.GetQueryCache(ctx, cacheKey)
	if err != nil || cached == "" {
		return false
	}
	if err := json.Unmarshal([]byte(cached), target); err != nil {
		logger.L(ctx).Warnw("解析统计查询缓存失败", "cache_key", cacheKey, "label", label, "error", err)
		return false
	}
	logger.L(ctx).Debugw("从Redis缓存获取统计查询结果", "cache_key", cacheKey, "label", label)
	return true
}

func (c *StatisticsCache) storeJSON(ctx context.Context, cacheKey, label string, value interface{}) {
	if c == nil || value == nil || strings.TrimSpace(cacheKey) == "" {
		return
	}
	data, err := json.Marshal(value)
	if err != nil {
		logger.L(ctx).Warnw("序列化统计查询缓存失败", "cache_key", cacheKey, "label", label, "error", err)
		return
	}
	if err := c.SetQueryCache(ctx, cacheKey, string(data), 0); err != nil {
		logger.L(ctx).Warnw("写入统计查询缓存失败", "cache_key", cacheKey, "label", label, "error", err)
	}
}

func systemStatsCacheKey(orgID int64) string {
	return fmt.Sprintf("system:%d", orgID)
}

func questionnaireStatsCacheKey(orgID int64, questionnaireCode string) string {
	return fmt.Sprintf("questionnaire:%d:%s", orgID, questionnaireCode)
}

func testeeStatsCacheKey(orgID int64, testeeID uint64) string {
	return fmt.Sprintf("testee:%d:%d", orgID, testeeID)
}

func planStatsCacheKey(orgID int64, planID uint64) string {
	return fmt.Sprintf("plan:%d:%d", orgID, planID)
}

func overviewStatsCacheKey(orgID int64, timeRange domainStatistics.StatisticsTimeRange) string {
	from := normalizeLocalDay(timeRange.From).Format("2006-01-02")
	to := normalizeLocalDay(timeRange.To).Format("2006-01-02")
	if preset, ok := overviewWarmupPreset(timeRange); ok {
		return fmt.Sprintf("overview:%d:preset:%s:%s:%s", orgID, preset, from, to)
	}
	return fmt.Sprintf("overview:%d:range:%s:%s", orgID, from, to)
}

func overviewWarmupPreset(timeRange domainStatistics.StatisticsTimeRange) (string, bool) {
	preset := strings.TrimSpace(string(timeRange.Preset))
	toDay := normalizeLocalDay(timeRange.To)
	fromDay := normalizeLocalDay(timeRange.From)
	switch domainStatistics.TimeRangePreset(preset) {
	case domainStatistics.TimeRangePresetToday:
		return preset, fromDay.Equal(toDay)
	case domainStatistics.TimeRangePreset7D:
		return preset, fromDay.Equal(toDay.AddDate(0, 0, -6))
	case domainStatistics.TimeRangePreset30D:
		return preset, fromDay.Equal(toDay.AddDate(0, 0, -29))
	default:
		return "", false
	}
}

func normalizeLocalDay(value time.Time) time.Time {
	local := value.In(time.Local)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
}
