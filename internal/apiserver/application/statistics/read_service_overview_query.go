package statistics

import (
	"context"
	"encoding/json"
	"fmt"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

type overviewQuery struct {
	readModel StatisticsReadModel
	cache     *statisticsCacheHelper
	guard     *readGuard[*domainStatistics.StatisticsOverview]
}

func newOverviewQuery(
	readModel StatisticsReadModel,
	cache *statisticsCacheHelper,
	opts StatisticsReadGuardOptions,
) *overviewQuery {
	return &overviewQuery{
		readModel: readModel,
		cache:     cache,
		guard: newReadGuard(opts, cloneStatisticsOverview, func() {
			incStatsOverviewStaleServed()
		}),
	}
}

func overviewGuardKey(orgID int64, timeRange domainStatistics.StatisticsTimeRange) string {
	return fmt.Sprintf("overview:%d:%s:%s:%s", orgID, timeRange.Preset, timeRange.From.Format(timeLayout), timeRange.To.Format(timeLayout))
}

const timeLayout = "2006-01-02T15:04:05Z07:00"

func cloneStatisticsOverview(stats *domainStatistics.StatisticsOverview) *domainStatistics.StatisticsOverview {
	if stats == nil {
		return nil
	}
	data, err := json.Marshal(stats)
	if err != nil {
		cloned := *stats
		return &cloned
	}
	var out domainStatistics.StatisticsOverview
	if err := json.Unmarshal(data, &out); err != nil {
		cloned := *stats
		return &cloned
	}
	return &out
}

func (q *overviewQuery) GetOverview(ctx context.Context, orgID int64, filter QueryFilter) (*domainStatistics.StatisticsOverview, error) {
	timeRange, err := normalizeQueryFilter(filter)
	if err != nil {
		return nil, err
	}

	if stats, ok := q.cache.loadOverview(ctx, orgID, timeRange); ok {
		q.cache.recordOverviewHotset(ctx, orgID, timeRange)
		return stats, nil
	}

	stats, err := q.guard.Load(ctx, overviewGuardKey(orgID, timeRange), func(loadCtx context.Context) (*domainStatistics.StatisticsOverview, error) {
		return q.buildOverview(loadCtx, orgID, timeRange)
	})
	if err != nil {
		return nil, err
	}
	q.cache.storeOverview(ctx, orgID, timeRange, stats)
	q.cache.recordOverviewHotset(ctx, orgID, timeRange)
	return stats, nil
}

func (q *overviewQuery) buildOverview(ctx context.Context, orgID int64, timeRange domainStatistics.StatisticsTimeRange) (*domainStatistics.StatisticsOverview, error) {
	organizationOverview, err := q.readModel.GetOrganizationOverview(ctx, orgID)
	if err != nil {
		return nil, err
	}
	accessWindow, err := q.readModel.GetAccessFunnel(ctx, orgID, timeRange.From, timeRange.To)
	if err != nil {
		return nil, err
	}
	assessmentWindow, err := q.readModel.GetAssessmentService(ctx, orgID, timeRange.From, timeRange.To)
	if err != nil {
		return nil, err
	}
	dimensionAnalysis, err := q.readModel.GetDimensionAnalysisSummary(ctx, orgID)
	if err != nil {
		return nil, err
	}
	planWindow, err := q.readModel.GetPlanTaskOverview(ctx, orgID, timeRange.From, timeRange.To)
	if err != nil {
		return nil, err
	}
	accessTrend, err := q.readModel.GetAccessFunnelTrend(ctx, orgID, timeRange.From, timeRange.To)
	if err != nil {
		return nil, err
	}
	assessmentTrend, err := q.readModel.GetAssessmentServiceTrend(ctx, orgID, timeRange.From, timeRange.To)
	if err != nil {
		return nil, err
	}
	planTrend, err := q.readModel.GetPlanTaskTrend(ctx, orgID, nil, timeRange.From, timeRange.To)
	if err != nil {
		return nil, err
	}
	planFulfillmentWindow, err := q.readModel.GetPlanTaskFulfillment(ctx, orgID, nil, timeRange.From, timeRange.To)
	if err != nil {
		return nil, err
	}
	planFulfillmentTrend, err := q.readModel.GetPlanTaskFulfillmentTrend(ctx, orgID, nil, timeRange.From, timeRange.To)
	if err != nil {
		return nil, err
	}

	planActivityTrend := domainStatistics.PlanTaskActivityTrend{
		TaskCreated:   fillMissingDailyCounts(timeRange.From, timeRange.To, planTrend.TaskCreated),
		TaskOpened:    fillMissingDailyCounts(timeRange.From, timeRange.To, planTrend.TaskOpened),
		TaskCompleted: fillMissingDailyCounts(timeRange.From, timeRange.To, planTrend.TaskCompleted),
		TaskExpired:   fillMissingDailyCounts(timeRange.From, timeRange.To, planTrend.TaskExpired),
	}
	planFulfillmentFilledTrend := domainStatistics.PlanTaskFulfillmentTrend{
		Planned:   fillMissingDailyCounts(timeRange.From, timeRange.To, planFulfillmentTrend.Planned),
		Due:       fillMissingDailyCounts(timeRange.From, timeRange.To, planFulfillmentTrend.Due),
		Completed: fillMissingDailyCounts(timeRange.From, timeRange.To, planFulfillmentTrend.Completed),
		Overdue:   fillMissingDailyCounts(timeRange.From, timeRange.To, planFulfillmentTrend.Overdue),
	}

	return &domainStatistics.StatisticsOverview{
		OrgID:                orgID,
		TimeRange:            timeRange,
		OrganizationOverview: organizationOverview,
		AccessFunnel: domainStatistics.AccessFunnelStatistics{
			Window: accessWindow,
			Trend: domainStatistics.AccessFunnelTrend{
				EntryOpened:                 fillMissingDailyCounts(timeRange.From, timeRange.To, accessTrend.EntryOpened),
				IntakeConfirmed:             fillMissingDailyCounts(timeRange.From, timeRange.To, accessTrend.IntakeConfirmed),
				TesteeCreated:               fillMissingDailyCounts(timeRange.From, timeRange.To, accessTrend.TesteeCreated),
				CareRelationshipEstablished: fillMissingDailyCounts(timeRange.From, timeRange.To, accessTrend.CareRelationshipEstablished),
			},
		},
		AssessmentService: domainStatistics.AssessmentServiceStatistics{
			Window: assessmentWindow,
			Trend: domainStatistics.AssessmentServiceTrend{
				AnswerSheetSubmitted: fillMissingDailyCounts(timeRange.From, timeRange.To, assessmentTrend.AnswerSheetSubmitted),
				AssessmentCreated:    fillMissingDailyCounts(timeRange.From, timeRange.To, assessmentTrend.AssessmentCreated),
				ReportGenerated:      fillMissingDailyCounts(timeRange.From, timeRange.To, assessmentTrend.ReportGenerated),
				AssessmentFailed:     fillMissingDailyCounts(timeRange.From, timeRange.To, assessmentTrend.AssessmentFailed),
			},
		},
		DimensionAnalysis: dimensionAnalysis,
		Plan: domainStatistics.PlanDomainStatistics{
			Activity: domainStatistics.PlanTaskActivityStatistics{
				Window: planWindow,
				Trend:  planActivityTrend,
			},
			Fulfillment: domainStatistics.PlanTaskFulfillmentStatistics{
				Window: planFulfillmentWindow,
				Trend:  planFulfillmentFilledTrend,
			},
			Window: planWindow,
			Trend:  planActivityTrend,
		},
	}, nil
}
