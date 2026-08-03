package plan

import (
	"context"
	"time"
)

type taskScheduleStatsCollectorKey struct{}
type taskSchedulerScopeKey struct{}
type taskSchedulerPlannedAtLowerBoundKey struct{}
type taskSchedulerMissedExpirationEnabledKey struct{}
type taskSchedulerScanLimitsKey struct{}

const (
	defaultTaskSchedulerBatchSize       = 200
	defaultTaskSchedulerMaxTasksPerTick = 2000
)

type taskSchedulerScanLimits struct {
	batchSize       int
	maxTasksPerTick int
}

// TaskScheduleStats 记录一次任务调度中的统计数据。
type TaskScheduleStats struct {
	PendingCount            int
	OpenedCount             int
	FailedCount             int
	OpenedOverdueCount      int
	ExpiredCount            int
	ExpireFailedCount       int
	MissedCandidateCount    int
	MissedExpiredCount      int
	MissedExpireFailedCount int
	MissedBacklogCount      int
	OldestPendingAgeSeconds int64
	OldestOpenedAgeSeconds  int64
}

// WithTaskSchedulerMissedExpirationEnabled controls the third scheduler phase.
// Absence means enabled, which is the steady-state default after one-off repair.
func WithTaskSchedulerMissedExpirationEnabled(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, taskSchedulerMissedExpirationEnabledKey{}, enabled)
}

func TaskSchedulerMissedExpirationEnabled(ctx context.Context) bool {
	if ctx == nil {
		return true
	}
	enabled, ok := ctx.Value(taskSchedulerMissedExpirationEnabledKey{}).(bool)
	if !ok {
		return true
	}
	return enabled
}

// WithTaskSchedulerScanLimits bounds every scheduler phase for one
// organization. Invalid values fall back to the safe defaults so non-runner
// callers retain bounded behavior.
func WithTaskSchedulerScanLimits(ctx context.Context, batchSize, maxTasksPerTick int) context.Context {
	if batchSize <= 0 {
		batchSize = defaultTaskSchedulerBatchSize
	}
	if maxTasksPerTick <= 0 {
		maxTasksPerTick = defaultTaskSchedulerMaxTasksPerTick
	}
	if batchSize > maxTasksPerTick {
		batchSize = maxTasksPerTick
	}
	return context.WithValue(ctx, taskSchedulerScanLimitsKey{}, taskSchedulerScanLimits{
		batchSize:       batchSize,
		maxTasksPerTick: maxTasksPerTick,
	})
}

func taskSchedulerLimitsFromContext(ctx context.Context) (int, int) {
	if ctx != nil {
		if limits, ok := ctx.Value(taskSchedulerScanLimitsKey{}).(taskSchedulerScanLimits); ok && limits.batchSize > 0 && limits.maxTasksPerTick > 0 {
			return limits.batchSize, limits.maxTasksPerTick
		}
	}
	return defaultTaskSchedulerBatchSize, defaultTaskSchedulerMaxTasksPerTick
}

// TaskSchedulerScope 表示一次调度的可选过滤范围。
type TaskSchedulerScope struct {
	PlanID    string
	TesteeIDs []string
}

// WithTaskSchedulerScope 为调度上下文附加计划/受试者范围。
func WithTaskSchedulerScope(ctx context.Context, planID string, testeeIDs []string) context.Context {
	if planID == "" && len(testeeIDs) == 0 {
		return ctx
	}
	scope := &TaskSchedulerScope{PlanID: planID}
	if len(testeeIDs) > 0 {
		scope.TesteeIDs = append([]string(nil), testeeIDs...)
	}
	return context.WithValue(ctx, taskSchedulerScopeKey{}, scope)
}

// WithTaskSchedulerPlannedAtLowerBound 设置自动调度的开放窗口下界，
// 防止历史回填的 pending Task 被批量开放。
func WithTaskSchedulerPlannedAtLowerBound(ctx context.Context, lowerBound time.Time) context.Context {
	if lowerBound.IsZero() {
		return ctx
	}
	return context.WithValue(ctx, taskSchedulerPlannedAtLowerBoundKey{}, lowerBound)
}

// WithTaskScheduleStatsCollector 为调度上下文附加统计收集器。
func WithTaskScheduleStatsCollector(ctx context.Context, collector *TaskScheduleStats) context.Context {
	if collector == nil {
		return ctx
	}
	return context.WithValue(ctx, taskScheduleStatsCollectorKey{}, collector)
}

func taskSchedulerScopeFromContext(ctx context.Context) *TaskSchedulerScope {
	if ctx == nil {
		return nil
	}
	scope, _ := ctx.Value(taskSchedulerScopeKey{}).(*TaskSchedulerScope)
	return scope
}

// TaskSchedulerPlannedAtLowerBoundFromContext 返回自动调度的可选窗口下界。
func TaskSchedulerPlannedAtLowerBoundFromContext(ctx context.Context) (time.Time, bool) {
	if ctx == nil {
		return time.Time{}, false
	}
	lowerBound, ok := ctx.Value(taskSchedulerPlannedAtLowerBoundKey{}).(time.Time)
	if !ok || lowerBound.IsZero() {
		return time.Time{}, false
	}
	return lowerBound, true
}

// CollectTaskScheduleStats 将调度统计累加到上下文收集器中。
func CollectTaskScheduleStats(ctx context.Context, stats TaskScheduleStats) {
	if ctx == nil {
		return
	}
	collector, _ := ctx.Value(taskScheduleStatsCollectorKey{}).(*TaskScheduleStats)
	if collector == nil {
		return
	}
	collector.PendingCount += stats.PendingCount
	collector.OpenedCount += stats.OpenedCount
	collector.FailedCount += stats.FailedCount
	collector.OpenedOverdueCount += stats.OpenedOverdueCount
	collector.ExpiredCount += stats.ExpiredCount
	collector.ExpireFailedCount += stats.ExpireFailedCount
	collector.MissedCandidateCount += stats.MissedCandidateCount
	collector.MissedExpiredCount += stats.MissedExpiredCount
	collector.MissedExpireFailedCount += stats.MissedExpireFailedCount
	collector.MissedBacklogCount += stats.MissedBacklogCount
	if stats.OldestPendingAgeSeconds > collector.OldestPendingAgeSeconds {
		collector.OldestPendingAgeSeconds = stats.OldestPendingAgeSeconds
	}
	if stats.OldestOpenedAgeSeconds > collector.OldestOpenedAgeSeconds {
		collector.OldestOpenedAgeSeconds = stats.OldestOpenedAgeSeconds
	}
}
