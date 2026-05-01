package statisticscache

import (
	"context"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

// Cache exposes typed statistics query-cache operations to application services.
type Cache interface {
	LoadSystemStatistics(ctx context.Context, orgID int64) (*domainStatistics.SystemStatistics, bool)
	StoreSystemStatistics(ctx context.Context, orgID int64, stats *domainStatistics.SystemStatistics)
	LoadQuestionnaireStatistics(ctx context.Context, orgID int64, questionnaireCode string) (*domainStatistics.QuestionnaireStatistics, bool)
	StoreQuestionnaireStatistics(ctx context.Context, orgID int64, questionnaireCode string, stats *domainStatistics.QuestionnaireStatistics)
	LoadTesteeStatistics(ctx context.Context, orgID int64, testeeID uint64) (*domainStatistics.TesteeStatistics, bool)
	StoreTesteeStatistics(ctx context.Context, orgID int64, testeeID uint64, stats *domainStatistics.TesteeStatistics)
	LoadPlanStatistics(ctx context.Context, orgID int64, planID uint64) (*domainStatistics.PlanStatistics, bool)
	StorePlanStatistics(ctx context.Context, orgID int64, planID uint64, stats *domainStatistics.PlanStatistics)
	LoadOverview(ctx context.Context, orgID int64, timeRange domainStatistics.StatisticsTimeRange) (*domainStatistics.StatisticsOverview, bool)
	StoreOverview(ctx context.Context, orgID int64, timeRange domainStatistics.StatisticsTimeRange, stats *domainStatistics.StatisticsOverview)
}
