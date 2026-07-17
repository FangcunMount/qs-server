package statisticscache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

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
	if err := c.SetQueryCache(ctx, cacheKey, string(data)); err != nil {
		logger.L(ctx).Warnw("写入统计查询缓存失败", "cache_key", cacheKey, "label", label, "error", err)
	}
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
