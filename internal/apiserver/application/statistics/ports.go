package statistics

import (
	"context"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

type StatisticsQueryReader interface {
	LoadSystemStatistics(ctx context.Context, orgID int64) (*domainStatistics.SystemStatistics, bool, error)
	LoadQuestionnaireStatistics(ctx context.Context, orgID int64, questionnaireCode string) (*domainStatistics.QuestionnaireStatistics, bool, error)
	LoadPlanStatistics(ctx context.Context, orgID int64, planID uint64) (*domainStatistics.PlanStatistics, bool, error)
}

type StatisticsRealtimeReader interface {
	BuildRealtimeSystemStatistics(ctx context.Context, orgID int64) (*domainStatistics.SystemStatistics, error)
	BuildRealtimeQuestionnaireStatistics(ctx context.Context, orgID int64, questionnaireCode string) (*domainStatistics.QuestionnaireStatistics, error)
	BuildRealtimeTesteeStatistics(ctx context.Context, orgID int64, testeeID uint64) (*domainStatistics.TesteeStatistics, error)
	BuildRealtimePlanStatistics(ctx context.Context, orgID int64, planID uint64) (*domainStatistics.PlanStatistics, error)
}

type StatisticsRebuildWriter interface {
	RebuildDailyStatistics(ctx context.Context, orgID int64, startDate, endDate time.Time) error
	RebuildOrgSnapshotStatistics(ctx context.Context, orgID int64, todayStart time.Time) error
	RebuildPlanStatistics(ctx context.Context, orgID int64) error
}

type PeriodicStatsReader interface {
	GetPeriodicStats(ctx context.Context, orgID int64, testeeID uint64) (*domainStatistics.TesteePeriodicStatisticsResponse, error)
}

type BehaviorJourneyRepository interface {
	domainStatistics.BehaviorFootprintWriter
	domainStatistics.AssessmentEpisodeRepository
	domainStatistics.StatisticsJourneyRepository
}
