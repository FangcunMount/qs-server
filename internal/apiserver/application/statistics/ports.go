package statistics

import (
	"context"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

// TesteeAccessValidator 定义了验证测试对象访问权限的接口，确保只有有权限的用户才能访问特定测试对象的统计数据。
type TesteeAccessValidator interface {
	ValidateTesteeAccess(ctx context.Context, orgID int64, operatorUserID int64, testeeID uint64) error
}

// StatisticsRebuildWriter 定义重建统计数据的接口，提供了重建日常统计、组织快照统计和计划统计的方法。实现该接口的组件负责根据历史数据重新计算统计结果，以修正可能存在的数据错误或更新统计算法后的结果。
type StatisticsRebuildWriter interface {
	RebuildDailyStatistics(ctx context.Context, orgID int64, startDate, endDate time.Time) error
	RebuildJourneyDailyWindow(ctx context.Context, orgID int64, startDate, endDate time.Time) error
	RebuildOrgSnapshotStatistics(ctx context.Context, orgID int64, todayStart time.Time) error
	RebuildPlanStatistics(ctx context.Context, orgID int64) error
}

// PeriodicStatsReader 定义获取周期性统计数据的接口，提供了获取测试对象周期性统计的方法。实现该接口的组件负责根据测试对象的历史数据计算周期性统计结果，以帮助用户了解测试对象在不同时间段内的表现和趋势。
type PeriodicStatsReader interface {
	GetPeriodicStats(ctx context.Context, orgID int64, testeeID uint64) (*domainStatistics.TesteePeriodicStatisticsResponse, error)
}

// BehaviorJourneyRepository 定义了行为旅程数据访问的接口，组合了行为足迹写入、评估事件查询和统计旅程查询的方法。实现该接口的组件负责处理与测试对象行为相关的数据操作，包括记录行为足迹、查询评估事件以及获取统计旅程数据，以支持用户对测试对象行为的全面分析和理解。
type BehaviorJourneyRepository interface {
	domainStatistics.BehaviorFootprintWriter
	domainStatistics.AssessmentEpisodeRepository
	domainStatistics.StatisticsJourneyRepository
}
