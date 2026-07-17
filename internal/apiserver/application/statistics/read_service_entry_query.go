package statistics

import (
	"context"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

type entryStatsQuery struct {
	readModel EntryStatisticsReader
}

func (q *entryStatsQuery) ListAssessmentEntryStatistics(ctx context.Context, orgID int64, clinicianID *uint64, activeOnly *bool, filter QueryFilter, page, pageSize int) (*domainStatistics.AssessmentEntryStatisticsList, error) {
	timeRange, err := normalizeQueryFilter(filter)
	if err != nil {
		return nil, err
	}
	page, pageSize = normalizePage(page, pageSize)

	total, err := q.readModel.CountAssessmentEntries(ctx, orgID, clinicianID, activeOnly)
	if err != nil {
		return nil, err
	}
	metas, err := q.readModel.ListAssessmentEntryMetas(ctx, orgID, clinicianID, activeOnly, page, pageSize)
	if err != nil {
		return nil, err
	}
	entryIDs := make([]uint64, 0, len(metas))
	for i := range metas {
		entryIDs = append(entryIDs, metas[i].ID.Uint64())
	}
	details, err := q.readModel.GetAssessmentEntryStatisticsDetails(ctx, orgID, entryIDs, timeRange.From, timeRange.To)
	if err != nil {
		return nil, err
	}

	items := make([]*domainStatistics.AssessmentEntryStatistics, 0, len(metas))
	for i := range metas {
		items = append(items, buildEntryStatistics(metas[i], timeRange, details[metas[i].ID.Uint64()]))
	}

	return &domainStatistics.AssessmentEntryStatisticsList{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: calcTotalPages(total, pageSize),
	}, nil
}

func (q *entryStatsQuery) GetAssessmentEntryStatistics(ctx context.Context, orgID int64, entryID uint64, filter QueryFilter) (*domainStatistics.AssessmentEntryStatistics, error) {
	timeRange, err := normalizeQueryFilter(filter)
	if err != nil {
		return nil, err
	}

	metaItem, err := q.readModel.GetAssessmentEntryMeta(ctx, orgID, entryID)
	if err != nil {
		return nil, err
	}
	details, err := q.readModel.GetAssessmentEntryStatisticsDetails(ctx, orgID, []uint64{entryID}, timeRange.From, timeRange.To)
	if err != nil {
		return nil, err
	}
	return buildEntryStatistics(*metaItem, timeRange, details[entryID]), nil
}

func (q *entryStatsQuery) ListCurrentClinicianEntryStatistics(ctx context.Context, orgID int64, operatorUserID int64, filter QueryFilter, page, pageSize int) (*domainStatistics.AssessmentEntryStatisticsList, error) {
	subject, err := q.readModel.GetCurrentClinicianSubject(ctx, orgID, operatorUserID)
	if err != nil {
		return nil, err
	}
	clinicianID := subject.ID.Uint64()
	return q.ListAssessmentEntryStatistics(ctx, orgID, &clinicianID, nil, filter, page, pageSize)
}

func buildEntryStatistics(entry domainStatistics.AssessmentEntryStatisticsMeta, timeRange domainStatistics.StatisticsTimeRange, detail AssessmentEntryStatisticsDetail) *domainStatistics.AssessmentEntryStatistics {
	return &domainStatistics.AssessmentEntryStatistics{
		TimeRange:      timeRange,
		Entry:          entry,
		Snapshot:       detail.Snapshot,
		Window:         detail.Window,
		LastResolvedAt: detail.LastResolvedAt,
		LastIntakeAt:   detail.LastIntakeAt,
	}
}
