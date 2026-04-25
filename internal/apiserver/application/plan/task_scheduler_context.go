package plan

import (
	"context"
	"time"
)

type taskScheduleStatsCollectorKey struct{}
type taskSchedulerScopeKey struct{}
type taskSchedulerPlannedAtLowerBoundKey struct{}

// TaskScheduleStats 记录一次任务调度中的统计数据。
type TaskScheduleStats struct {
	PendingCount      int
	OpenedCount       int
	FailedCount       int
	ExpiredCount      int
	ExpireFailedCount int
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

// WithTaskSchedulerPlannedAtLowerBound limits automatic scheduling to tasks at
// or after lowerBound. It is intended for the built-in scheduler so historical
// backfilled pending tasks are not opened immediately.
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

// TaskSchedulerPlannedAtLowerBoundFromContext returns the optional lower bound
// used by automatic scheduling.
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
	collector.ExpiredCount += stats.ExpiredCount
	collector.ExpireFailedCount += stats.ExpireFailedCount
}
