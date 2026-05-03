package statistics

import (
	"context"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

type entryStatsQuery struct {
	readModel StatisticsReadModel
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

	items := make([]*domainStatistics.AssessmentEntryStatistics, 0, len(metas))
	for i := range metas {
		item, err := q.buildEntryStatistics(ctx, orgID, metas[i], timeRange)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
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
	return q.buildEntryStatistics(ctx, orgID, *metaItem, timeRange)
}

func (q *entryStatsQuery) ListCurrentClinicianEntryStatistics(ctx context.Context, orgID int64, operatorUserID int64, filter QueryFilter, page, pageSize int) (*domainStatistics.AssessmentEntryStatisticsList, error) {
	subject, err := q.readModel.GetCurrentClinicianSubject(ctx, orgID, operatorUserID)
	if err != nil {
		return nil, err
	}
	clinicianID := subject.ID.Uint64()
	return q.ListAssessmentEntryStatistics(ctx, orgID, &clinicianID, nil, filter, page, pageSize)
}

func (q *entryStatsQuery) buildEntryStatistics(ctx context.Context, orgID int64, entry domainStatistics.AssessmentEntryStatisticsMeta, timeRange domainStatistics.StatisticsTimeRange) (*domainStatistics.AssessmentEntryStatistics, error) {
	snapshot, err := q.readModel.GetAssessmentEntryCounts(ctx, orgID, entry.ID.Uint64(), nil, nil)
	if err != nil {
		return nil, err
	}
	window, err := q.readModel.GetAssessmentEntryCounts(ctx, orgID, entry.ID.Uint64(), &timeRange.From, &timeRange.To)
	if err != nil {
		return nil, err
	}
	lastResolvedAt, err := q.readModel.GetAssessmentEntryLastEventTime(ctx, orgID, entry.ID.Uint64(), domainStatistics.BehaviorEventEntryOpened)
	if err != nil {
		return nil, err
	}
	lastIntakeAt, err := q.readModel.GetAssessmentEntryLastEventTime(ctx, orgID, entry.ID.Uint64(), domainStatistics.BehaviorEventIntakeConfirmed)
	if err != nil {
		return nil, err
	}

	return &domainStatistics.AssessmentEntryStatistics{
		TimeRange:      timeRange,
		Entry:          entry,
		Snapshot:       snapshot,
		Window:         window,
		LastResolvedAt: lastResolvedAt,
		LastIntakeAt:   lastIntakeAt,
	}, nil
}
