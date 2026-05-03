package statistics

import (
	"context"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

type clinicianStatsQuery struct {
	readModel StatisticsReadModel
}

func (q *clinicianStatsQuery) ListClinicianStatistics(ctx context.Context, orgID int64, filter QueryFilter, page, pageSize int) (*domainStatistics.ClinicianStatisticsList, error) {
	timeRange, err := normalizeQueryFilter(filter)
	if err != nil {
		return nil, err
	}
	page, pageSize = normalizePage(page, pageSize)

	total, err := q.readModel.CountClinicianSubjects(ctx, orgID)
	if err != nil {
		return nil, err
	}
	subjects, err := q.readModel.ListClinicianSubjects(ctx, orgID, page, pageSize)
	if err != nil {
		return nil, err
	}

	items := make([]*domainStatistics.ClinicianStatistics, 0, len(subjects))
	for i := range subjects {
		item, err := q.buildClinicianStatistics(ctx, orgID, subjects[i], timeRange)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return &domainStatistics.ClinicianStatisticsList{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: calcTotalPages(total, pageSize),
	}, nil
}

func (q *clinicianStatsQuery) GetClinicianStatistics(ctx context.Context, orgID int64, clinicianID uint64, filter QueryFilter) (*domainStatistics.ClinicianStatistics, error) {
	timeRange, err := normalizeQueryFilter(filter)
	if err != nil {
		return nil, err
	}

	subject, err := q.readModel.GetClinicianSubject(ctx, orgID, clinicianID)
	if err != nil {
		return nil, err
	}
	return q.buildClinicianStatistics(ctx, orgID, *subject, timeRange)
}

func (q *clinicianStatsQuery) GetCurrentClinicianStatistics(ctx context.Context, orgID int64, operatorUserID int64, filter QueryFilter) (*domainStatistics.ClinicianStatistics, error) {
	timeRange, err := normalizeQueryFilter(filter)
	if err != nil {
		return nil, err
	}

	subject, err := q.readModel.GetCurrentClinicianSubject(ctx, orgID, operatorUserID)
	if err != nil {
		return nil, err
	}
	return q.buildClinicianStatistics(ctx, orgID, *subject, timeRange)
}

func (q *clinicianStatsQuery) GetCurrentClinicianTesteeSummary(ctx context.Context, orgID int64, operatorUserID int64, filter QueryFilter) (*domainStatistics.ClinicianTesteeSummaryStatistics, error) {
	timeRange, err := normalizeQueryFilter(filter)
	if err != nil {
		return nil, err
	}

	subject, err := q.readModel.GetCurrentClinicianSubject(ctx, orgID, operatorUserID)
	if err != nil {
		return nil, err
	}

	snapshot, err := q.readModel.GetClinicianSnapshot(ctx, orgID, subject.ID.Uint64())
	if err != nil {
		return nil, err
	}
	keyFocusCount, assessedInWindowCount, err := q.readModel.GetClinicianTesteeSummaryCounts(ctx, orgID, subject.ID.Uint64(), timeRange.From, timeRange.To)
	if err != nil {
		return nil, err
	}

	return &domainStatistics.ClinicianTesteeSummaryStatistics{
		TimeRange:               timeRange,
		TotalAccessibleTestees:  snapshot.TotalAccessibleTestees,
		PrimaryTesteeCount:      snapshot.PrimaryTesteeCount,
		AttendingTesteeCount:    snapshot.AttendingTesteeCount,
		CollaboratorTesteeCount: snapshot.CollaboratorTesteeCount,
		KeyFocusTesteeCount:     keyFocusCount,
		AssessedInWindowCount:   assessedInWindowCount,
	}, nil
}

func (q *clinicianStatsQuery) buildClinicianStatistics(ctx context.Context, orgID int64, subject domainStatistics.ClinicianStatisticsSubject, timeRange domainStatistics.StatisticsTimeRange) (*domainStatistics.ClinicianStatistics, error) {
	snapshot, err := q.readModel.GetClinicianSnapshot(ctx, orgID, subject.ID.Uint64())
	if err != nil {
		return nil, err
	}
	window, funnel, err := q.readModel.GetClinicianJourneyStats(ctx, orgID, subject.ID.Uint64(), timeRange.From, timeRange.To)
	if err != nil {
		return nil, err
	}

	return &domainStatistics.ClinicianStatistics{
		TimeRange: timeRange,
		Clinician: subject,
		Snapshot:  snapshot,
		Window:    window,
		Funnel:    funnel,
	}, nil
}
