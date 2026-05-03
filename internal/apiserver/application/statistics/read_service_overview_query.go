package statistics

import (
	"context"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

type overviewQuery struct {
	readModel StatisticsReadModel
	cache     *statisticsCacheHelper
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

	stats, err := q.buildOverview(ctx, orgID, timeRange)
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
			Window: planWindow,
			Trend: domainStatistics.PlanTaskTrend{
				TaskCreated:   fillMissingDailyCounts(timeRange.From, timeRange.To, planTrend.TaskCreated),
				TaskOpened:    fillMissingDailyCounts(timeRange.From, timeRange.To, planTrend.TaskOpened),
				TaskCompleted: fillMissingDailyCounts(timeRange.From, timeRange.To, planTrend.TaskCompleted),
				TaskExpired:   fillMissingDailyCounts(timeRange.From, timeRange.To, planTrend.TaskExpired),
			},
		},
	}, nil
}
