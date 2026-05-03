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

// StatisticsQueryReader 定义读取统计数据的接口，提供了加载系统统计、问卷统计和计划统计的方法。实现该接口的组件负责从数据源中获取统计数据，并返回给调用者。
type StatisticsQueryReader interface {
	LoadSystemStatistics(ctx context.Context, orgID int64) (*domainStatistics.SystemStatistics, bool, error)
	LoadQuestionnaireStatistics(ctx context.Context, orgID int64, questionnaireCode string) (*domainStatistics.QuestionnaireStatistics, bool, error)
	LoadPlanStatistics(ctx context.Context, orgID int64, planID uint64) (*domainStatistics.PlanStatistics, bool, error)
}

// StatisticsRealtimeReader 定义实时构建统计数据的接口，提供了构建系统统计、问卷统计、测试对象统计和计划统计的方法。实现该接口的组件负责根据最新的数据动态计算统计结果，以确保提供给用户的是最新的统计信息。
type StatisticsRealtimeReader interface {
	BuildRealtimeSystemStatistics(ctx context.Context, orgID int64) (*domainStatistics.SystemStatistics, error)
	BuildRealtimeQuestionnaireStatistics(ctx context.Context, orgID int64, questionnaireCode string) (*domainStatistics.QuestionnaireStatistics, error)
	BuildRealtimeTesteeStatistics(ctx context.Context, orgID int64, testeeID uint64) (*domainStatistics.TesteeStatistics, error)
	BuildRealtimePlanStatistics(ctx context.Context, orgID int64, planID uint64) (*domainStatistics.PlanStatistics, error)
}

// StatisticsRebuildWriter 定义重建统计数据的接口，提供了重建日常统计、组织快照统计和计划统计的方法。实现该接口的组件负责根据历史数据重新计算统计结果，以修正可能存在的数据错误或更新统计算法后的结果。
type StatisticsRebuildWriter interface {
	RebuildDailyStatistics(ctx context.Context, orgID int64, startDate, endDate time.Time) error
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
