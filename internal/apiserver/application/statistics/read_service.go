package statistics

import (
	"context"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticscache "github.com/FangcunMount/qs-server/internal/apiserver/port/statisticscache"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type readService struct {
	readModel       StatisticsReadModel
	answerSheetRead surveyreadmodel.AnswerSheetReader
	cache           statisticscache.Cache
	hotset          cachetarget.HotsetRecorder
}

type ReadServiceOption func(*readService)

func WithReadServiceCache(cache statisticscache.Cache) ReadServiceOption {
	return func(s *readService) {
		s.cache = cache
	}
}

func WithReadServiceHotset(hotset cachetarget.HotsetRecorder) ReadServiceOption {
	return func(s *readService) {
		s.hotset = hotset
	}
}

// NewReadService 创建统一统计读服务。
func NewReadService(readModel StatisticsReadModel, answerSheetRead surveyreadmodel.AnswerSheetReader, opts ...ReadServiceOption) ReadService {
	service := &readService{readModel: readModel, answerSheetRead: answerSheetRead}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
}

func (s *readService) GetOverview(ctx context.Context, orgID int64, filter QueryFilter) (*domainStatistics.StatisticsOverview, error) {
	timeRange, err := normalizeQueryFilter(filter)
	if err != nil {
		return nil, err
	}

	if stats, ok := s.loadCachedOverview(ctx, orgID, timeRange); ok {
		s.recordOverviewHotset(ctx, orgID, timeRange)
		return stats, nil
	}

	stats, err := s.buildOverview(ctx, orgID, timeRange)
	if err != nil {
		return nil, err
	}
	s.cacheOverview(ctx, orgID, timeRange, stats)
	s.recordOverviewHotset(ctx, orgID, timeRange)
	return stats, nil
}

func (s *readService) buildOverview(ctx context.Context, orgID int64, timeRange domainStatistics.StatisticsTimeRange) (*domainStatistics.StatisticsOverview, error) {
	organizationOverview, err := s.readModel.GetOrganizationOverview(ctx, orgID)
	if err != nil {
		return nil, err
	}
	accessWindow, err := s.readModel.GetAccessFunnel(ctx, orgID, timeRange.From, timeRange.To)
	if err != nil {
		return nil, err
	}
	assessmentWindow, err := s.readModel.GetAssessmentService(ctx, orgID, timeRange.From, timeRange.To)
	if err != nil {
		return nil, err
	}
	dimensionAnalysis, err := s.readModel.GetDimensionAnalysisSummary(ctx, orgID)
	if err != nil {
		return nil, err
	}
	planWindow, err := s.readModel.GetPlanTaskOverview(ctx, orgID, timeRange.From, timeRange.To)
	if err != nil {
		return nil, err
	}
	accessTrend, err := s.readModel.GetAccessFunnelTrend(ctx, orgID, timeRange.From, timeRange.To)
	if err != nil {
		return nil, err
	}
	assessmentTrend, err := s.readModel.GetAssessmentServiceTrend(ctx, orgID, timeRange.From, timeRange.To)
	if err != nil {
		return nil, err
	}
	planTrend, err := s.readModel.GetPlanTaskTrend(ctx, orgID, nil, timeRange.From, timeRange.To)
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

func (s *readService) ListClinicianStatistics(ctx context.Context, orgID int64, filter QueryFilter, page, pageSize int) (*domainStatistics.ClinicianStatisticsList, error) {
	timeRange, err := normalizeQueryFilter(filter)
	if err != nil {
		return nil, err
	}
	page, pageSize = normalizePage(page, pageSize)

	total, err := s.readModel.CountClinicianSubjects(ctx, orgID)
	if err != nil {
		return nil, err
	}
	subjects, err := s.readModel.ListClinicianSubjects(ctx, orgID, page, pageSize)
	if err != nil {
		return nil, err
	}

	items := make([]*domainStatistics.ClinicianStatistics, 0, len(subjects))
	for i := range subjects {
		item, err := s.buildClinicianStatistics(ctx, orgID, subjects[i], timeRange)
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

func (s *readService) GetClinicianStatistics(ctx context.Context, orgID int64, clinicianID uint64, filter QueryFilter) (*domainStatistics.ClinicianStatistics, error) {
	timeRange, err := normalizeQueryFilter(filter)
	if err != nil {
		return nil, err
	}

	subject, err := s.readModel.GetClinicianSubject(ctx, orgID, clinicianID)
	if err != nil {
		return nil, err
	}
	return s.buildClinicianStatistics(ctx, orgID, *subject, timeRange)
}

func (s *readService) ListAssessmentEntryStatistics(ctx context.Context, orgID int64, clinicianID *uint64, activeOnly *bool, filter QueryFilter, page, pageSize int) (*domainStatistics.AssessmentEntryStatisticsList, error) {
	timeRange, err := normalizeQueryFilter(filter)
	if err != nil {
		return nil, err
	}
	page, pageSize = normalizePage(page, pageSize)

	total, err := s.readModel.CountAssessmentEntries(ctx, orgID, clinicianID, activeOnly)
	if err != nil {
		return nil, err
	}
	metas, err := s.readModel.ListAssessmentEntryMetas(ctx, orgID, clinicianID, activeOnly, page, pageSize)
	if err != nil {
		return nil, err
	}

	items := make([]*domainStatistics.AssessmentEntryStatistics, 0, len(metas))
	for i := range metas {
		item, err := s.buildEntryStatistics(ctx, orgID, metas[i], timeRange)
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

func (s *readService) GetAssessmentEntryStatistics(ctx context.Context, orgID int64, entryID uint64, filter QueryFilter) (*domainStatistics.AssessmentEntryStatistics, error) {
	timeRange, err := normalizeQueryFilter(filter)
	if err != nil {
		return nil, err
	}

	metaItem, err := s.readModel.GetAssessmentEntryMeta(ctx, orgID, entryID)
	if err != nil {
		return nil, err
	}
	return s.buildEntryStatistics(ctx, orgID, *metaItem, timeRange)
}

func (s *readService) GetCurrentClinicianStatistics(ctx context.Context, orgID int64, operatorUserID int64, filter QueryFilter) (*domainStatistics.ClinicianStatistics, error) {
	timeRange, err := normalizeQueryFilter(filter)
	if err != nil {
		return nil, err
	}

	subject, err := s.readModel.GetCurrentClinicianSubject(ctx, orgID, operatorUserID)
	if err != nil {
		return nil, err
	}
	return s.buildClinicianStatistics(ctx, orgID, *subject, timeRange)
}

func (s *readService) ListCurrentClinicianEntryStatistics(ctx context.Context, orgID int64, operatorUserID int64, filter QueryFilter, page, pageSize int) (*domainStatistics.AssessmentEntryStatisticsList, error) {
	subject, err := s.readModel.GetCurrentClinicianSubject(ctx, orgID, operatorUserID)
	if err != nil {
		return nil, err
	}
	clinicianID := subject.ID.Uint64()
	return s.ListAssessmentEntryStatistics(ctx, orgID, &clinicianID, nil, filter, page, pageSize)
}

func (s *readService) GetCurrentClinicianTesteeSummary(ctx context.Context, orgID int64, operatorUserID int64, filter QueryFilter) (*domainStatistics.ClinicianTesteeSummaryStatistics, error) {
	timeRange, err := normalizeQueryFilter(filter)
	if err != nil {
		return nil, err
	}

	subject, err := s.readModel.GetCurrentClinicianSubject(ctx, orgID, operatorUserID)
	if err != nil {
		return nil, err
	}

	snapshot, err := s.readModel.GetClinicianSnapshot(ctx, orgID, subject.ID.Uint64())
	if err != nil {
		return nil, err
	}
	keyFocusCount, assessedInWindowCount, err := s.readModel.GetClinicianTesteeSummaryCounts(ctx, orgID, subject.ID.Uint64(), timeRange.From, timeRange.To)
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

func (s *readService) GetQuestionnaireBatchStatistics(ctx context.Context, orgID int64, codes []string) (*domainStatistics.QuestionnaireBatchStatisticsResponse, error) {
	cleanCodes := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, codeValue := range codes {
		codeValue = strings.TrimSpace(codeValue)
		if codeValue == "" {
			continue
		}
		if _, exists := seen[codeValue]; exists {
			continue
		}
		seen[codeValue] = struct{}{}
		cleanCodes = append(cleanCodes, codeValue)
	}

	items := make([]*domainStatistics.QuestionnaireBatchStatisticsItem, 0, len(cleanCodes))
	if len(cleanCodes) == 0 {
		return &domainStatistics.QuestionnaireBatchStatisticsResponse{Items: items}, nil
	}

	totals, err := s.readModel.GetQuestionnaireBatchTotals(ctx, orgID, cleanCodes)
	if err != nil {
		return nil, err
	}

	resultByCode := make(map[string]*domainStatistics.QuestionnaireBatchStatisticsItem, len(cleanCodes))
	for _, codeValue := range cleanCodes {
		resultByCode[codeValue] = &domainStatistics.QuestionnaireBatchStatisticsItem{Code: codeValue}
	}
	for _, total := range totals {
		item := resultByCode[total.Code]
		if item == nil {
			item = &domainStatistics.QuestionnaireBatchStatisticsItem{Code: total.Code}
			resultByCode[total.Code] = item
		}
		item.TotalSubmissions = total.TotalSubmissions
		item.TotalCompletions = total.TotalCompletions
		if item.TotalSubmissions > 0 {
			item.CompletionRate = float64(item.TotalCompletions) / float64(item.TotalSubmissions) * 100
		}
	}

	for _, codeValue := range cleanCodes {
		items = append(items, resultByCode[codeValue])
	}

	if s.answerSheetRead != nil {
		for _, item := range items {
			if item.TotalSubmissions > 0 {
				continue
			}
			count, err := s.answerSheetRead.CountAnswerSheets(ctx, surveyreadmodel.AnswerSheetFilter{QuestionnaireCode: item.Code})
			if err != nil {
				return nil, err
			}
			if count <= 0 {
				continue
			}
			item.TotalSubmissions = count
			item.TotalCompletions = count
			item.CompletionRate = 100
		}
	}

	return &domainStatistics.QuestionnaireBatchStatisticsResponse{Items: items}, nil
}

func (s *readService) buildClinicianStatistics(ctx context.Context, orgID int64, subject domainStatistics.ClinicianStatisticsSubject, timeRange domainStatistics.StatisticsTimeRange) (*domainStatistics.ClinicianStatistics, error) {
	snapshot, err := s.readModel.GetClinicianSnapshot(ctx, orgID, subject.ID.Uint64())
	if err != nil {
		return nil, err
	}
	window, funnel, err := s.readModel.GetClinicianJourneyStats(ctx, orgID, subject.ID.Uint64(), timeRange.From, timeRange.To)
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

func (s *readService) buildEntryStatistics(ctx context.Context, orgID int64, entry domainStatistics.AssessmentEntryStatisticsMeta, timeRange domainStatistics.StatisticsTimeRange) (*domainStatistics.AssessmentEntryStatistics, error) {
	snapshot, err := s.readModel.GetAssessmentEntryCounts(ctx, orgID, entry.ID.Uint64(), nil, nil)
	if err != nil {
		return nil, err
	}
	window, err := s.readModel.GetAssessmentEntryCounts(ctx, orgID, entry.ID.Uint64(), &timeRange.From, &timeRange.To)
	if err != nil {
		return nil, err
	}
	lastResolvedAt, err := s.readModel.GetAssessmentEntryLastEventTime(ctx, orgID, entry.ID.Uint64(), domainStatistics.BehaviorEventEntryOpened)
	if err != nil {
		return nil, err
	}
	lastIntakeAt, err := s.readModel.GetAssessmentEntryLastEventTime(ctx, orgID, entry.ID.Uint64(), domainStatistics.BehaviorEventIntakeConfirmed)
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

func normalizeQueryFilter(filter QueryFilter) (domainStatistics.StatisticsTimeRange, error) {
	now := time.Now()
	preset := strings.TrimSpace(filter.Preset)
	if preset == "" {
		preset = string(domainStatistics.TimeRangePreset30D)
	}

	if strings.TrimSpace(filter.From) != "" || strings.TrimSpace(filter.To) != "" {
		from, err := parseFlexibleTime(filter.From, false)
		if err != nil {
			return domainStatistics.StatisticsTimeRange{}, errors.WithCode(code.ErrInvalidArgument, "invalid from: %v", err)
		}
		to, err := parseFlexibleTime(filter.To, true)
		if err != nil {
			return domainStatistics.StatisticsTimeRange{}, errors.WithCode(code.ErrInvalidArgument, "invalid to: %v", err)
		}
		if !from.Before(to) {
			return domainStatistics.StatisticsTimeRange{}, errors.WithCode(code.ErrInvalidArgument, "from must be before to")
		}
		return domainStatistics.StatisticsTimeRange{
			Preset: domainStatistics.TimeRangePreset(preset),
			From:   from,
			To:     to,
		}, nil
	}

	switch domainStatistics.TimeRangePreset(preset) {
	case domainStatistics.TimeRangePresetToday:
		from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return domainStatistics.StatisticsTimeRange{
			Preset: domainStatistics.TimeRangePresetToday,
			From:   from,
			To:     now,
		}, nil
	case domainStatistics.TimeRangePreset7D:
		from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -6)
		return domainStatistics.StatisticsTimeRange{
			Preset: domainStatistics.TimeRangePreset7D,
			From:   from,
			To:     now,
		}, nil
	case domainStatistics.TimeRangePreset30D:
		from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -29)
		return domainStatistics.StatisticsTimeRange{
			Preset: domainStatistics.TimeRangePreset30D,
			From:   from,
			To:     now,
		}, nil
	default:
		return domainStatistics.StatisticsTimeRange{}, errors.WithCode(code.ErrInvalidArgument, "unsupported preset: %s", preset)
	}
}

func parseFlexibleTime(raw string, end bool) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		if end {
			return time.Now(), nil
		}
		return time.Time{}, errors.WithCode(code.ErrInvalidArgument, "time is required")
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	var lastErr error
	for _, layout := range layouts {
		t, err := time.ParseInLocation(layout, value, time.Local)
		if err != nil {
			lastErr = err
			continue
		}
		if layout == "2006-01-02" && end {
			return t.AddDate(0, 0, 1), nil
		}
		return t, nil
	}
	return time.Time{}, lastErr
}

func fillMissingDailyCounts(from, to time.Time, counts []domainStatistics.DailyCount) []domainStatistics.DailyCount {
	if from.IsZero() || !from.Before(to) {
		return counts
	}

	countMap := make(map[string]int64, len(counts))
	for _, item := range counts {
		countMap[item.Date.Format("2006-01-02")] = item.Count
	}

	filled := make([]domainStatistics.DailyCount, 0)
	cursor := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	endDate := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, to.Location())
	if !cursor.Before(endDate) {
		endDate = endDate.AddDate(0, 0, 1)
	}

	for cursor.Before(endDate) {
		key := cursor.Format("2006-01-02")
		filled = append(filled, domainStatistics.DailyCount{
			Date:  cursor,
			Count: countMap[key],
		})
		cursor = cursor.AddDate(0, 0, 1)
	}

	return filled
}

func (s *readService) loadCachedOverview(ctx context.Context, orgID int64, timeRange domainStatistics.StatisticsTimeRange) (*domainStatistics.StatisticsOverview, bool) {
	if s == nil || s.cache == nil {
		return nil, false
	}
	return s.cache.LoadOverview(ctx, orgID, timeRange)
}

func (s *readService) cacheOverview(ctx context.Context, orgID int64, timeRange domainStatistics.StatisticsTimeRange, stats *domainStatistics.StatisticsOverview) {
	if s == nil || s.cache == nil || stats == nil {
		return
	}
	s.cache.StoreOverview(ctx, orgID, timeRange, stats)
}

func (s *readService) recordOverviewHotset(ctx context.Context, orgID int64, timeRange domainStatistics.StatisticsTimeRange) {
	if s == nil || s.hotset == nil {
		return
	}
	preset, ok := overviewWarmupPreset(timeRange)
	if !ok {
		return
	}
	_ = s.hotset.Record(ctx, cachetarget.NewQueryStatsOverviewWarmupTarget(orgID, preset))
}

func overviewWarmupPreset(timeRange domainStatistics.StatisticsTimeRange) (string, bool) {
	preset := strings.TrimSpace(string(timeRange.Preset))
	toDay := normalizeLocalDay(timeRange.To)
	fromDay := normalizeLocalDay(timeRange.From)
	switch domainStatistics.TimeRangePreset(preset) {
	case domainStatistics.TimeRangePresetToday:
		return preset, fromDay.Equal(toDay)
	case domainStatistics.TimeRangePreset7D:
		return preset, fromDay.Equal(toDay.AddDate(0, 0, -6))
	case domainStatistics.TimeRangePreset30D:
		return preset, fromDay.Equal(toDay.AddDate(0, 0, -29))
	default:
		return "", false
	}
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func calcTotalPages(total int64, pageSize int) int {
	if total == 0 {
		return 0
	}
	return int((total + int64(pageSize) - 1) / int64(pageSize))
}

func ptrMetaIDFromUint64(v *uint64) *meta.ID {
	if v == nil {
		return nil
	}
	id := meta.FromUint64(*v)
	return &id
}
