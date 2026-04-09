package plan

import "context"

const (
	// TaskSchedulerSourceBuiltin 表示内建 plan scheduler 调用。
	TaskSchedulerSourceBuiltin = "builtin_scheduler"
	// TaskSchedulerSourceInternalAPI 表示手工触发的内部接口调用。
	TaskSchedulerSourceInternalAPI = "internal_api"
	// TaskSchedulerSourceSeedData 表示 seeddata 工具调用。
	TaskSchedulerSourceSeedData = "seeddata"
)

type taskSchedulerSourceKey struct{}
type taskScheduleStatsCollectorKey struct{}
type taskSchedulerScopeKey struct{}

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

// WithTaskSchedulerSource 为调度上下文写入调用来源。
func WithTaskSchedulerSource(ctx context.Context, source string) context.Context {
	if source == "" {
		return ctx
	}
	return context.WithValue(ctx, taskSchedulerSourceKey{}, source)
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

// WithTaskScheduleStatsCollector 为调度上下文附加统计收集器。
func WithTaskScheduleStatsCollector(ctx context.Context, collector *TaskScheduleStats) context.Context {
	if collector == nil {
		return ctx
	}
	return context.WithValue(ctx, taskScheduleStatsCollectorKey{}, collector)
}

func taskSchedulerSourceFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	source, _ := ctx.Value(taskSchedulerSourceKey{}).(string)
	return source
}

func taskSchedulerScopeFromContext(ctx context.Context) *TaskSchedulerScope {
	if ctx == nil {
		return nil
	}
	scope, _ := ctx.Value(taskSchedulerScopeKey{}).(*TaskSchedulerScope)
	return scope
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
