package statistics

import (
	"context"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticscache "github.com/FangcunMount/qs-server/internal/apiserver/port/statisticscache"
)

type statisticsCacheHelper struct {
	cache  statisticscache.Cache
	hotset cachetarget.HotsetRecorder
}

func newStatisticsCacheHelper(cache statisticscache.Cache, hotset cachetarget.HotsetRecorder) *statisticsCacheHelper {
	return &statisticsCacheHelper{cache: cache, hotset: hotset}
}

func (h *statisticsCacheHelper) loadOverview(ctx context.Context, orgID int64, timeRange domainStatistics.StatisticsTimeRange) (*domainStatistics.StatisticsOverview, bool) {
	if h == nil || h.cache == nil {
		return nil, false
	}
	return h.cache.LoadOverview(ctx, orgID, timeRange)
}

func (h *statisticsCacheHelper) storeOverview(ctx context.Context, orgID int64, timeRange domainStatistics.StatisticsTimeRange, stats *domainStatistics.StatisticsOverview) {
	if h == nil || h.cache == nil || stats == nil {
		return
	}
	h.cache.StoreOverview(ctx, orgID, timeRange, stats)
}

func (h *statisticsCacheHelper) recordOverviewHotset(ctx context.Context, orgID int64, timeRange domainStatistics.StatisticsTimeRange) {
	if h == nil || h.hotset == nil {
		return
	}
	preset, ok := overviewWarmupPreset(timeRange)
	if !ok {
		return
	}
	_ = h.hotset.Record(ctx, cachetarget.NewQueryStatsOverviewWarmupTarget(orgID, preset))
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
