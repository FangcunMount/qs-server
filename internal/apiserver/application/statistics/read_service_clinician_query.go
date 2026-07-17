package statistics

import (
	"context"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

type clinicianStatsQuery struct {
	readModel ClinicianStatisticsReader
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
	clinicianIDs := make([]uint64, 0, len(subjects))
	for i := range subjects {
		clinicianIDs = append(clinicianIDs, subjects[i].ID.Uint64())
	}
	details, err := q.readModel.GetClinicianStatisticsDetails(ctx, orgID, clinicianIDs, timeRange.From, timeRange.To)
	if err != nil {
		return nil, err
	}

	items := make([]*domainStatistics.ClinicianStatistics, 0, len(subjects))
	for i := range subjects {
		items = append(items, buildClinicianStatistics(subjects[i], timeRange, details[subjects[i].ID.Uint64()]))
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
	details, err := q.readModel.GetClinicianStatisticsDetails(ctx, orgID, []uint64{clinicianID}, timeRange.From, timeRange.To)
	if err != nil {
		return nil, err
	}
	return buildClinicianStatistics(*subject, timeRange, details[clinicianID]), nil
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
	clinicianID := subject.ID.Uint64()
	details, err := q.readModel.GetClinicianStatisticsDetails(ctx, orgID, []uint64{clinicianID}, timeRange.From, timeRange.To)
	if err != nil {
		return nil, err
	}
	return buildClinicianStatistics(*subject, timeRange, details[clinicianID]), nil
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

func buildClinicianStatistics(subject domainStatistics.ClinicianStatisticsSubject, timeRange domainStatistics.StatisticsTimeRange, detail ClinicianStatisticsDetail) *domainStatistics.ClinicianStatistics {
	return &domainStatistics.ClinicianStatistics{
		TimeRange: timeRange,
		Clinician: subject,
		Snapshot:  detail.Snapshot,
		Window:    detail.Window,
		Funnel:    detail.Funnel,
	}
}
