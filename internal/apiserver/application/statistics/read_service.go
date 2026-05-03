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

	overview           *overviewQuery
	clinicianStats     *clinicianStatsQuery
	entryStats         *entryStatsQuery
	questionnaireBatch *questionnaireBatchQuery
	cacheHelper        *statisticsCacheHelper
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
	service.cacheHelper = newStatisticsCacheHelper(service.cache, service.hotset)
	service.overview = &overviewQuery{readModel: readModel, cache: service.cacheHelper}
	service.clinicianStats = &clinicianStatsQuery{readModel: readModel}
	service.entryStats = &entryStatsQuery{readModel: readModel}
	service.questionnaireBatch = &questionnaireBatchQuery{readModel: readModel, answerSheetRead: answerSheetRead}
	return service
}

func (s *readService) GetOverview(ctx context.Context, orgID int64, filter QueryFilter) (*domainStatistics.StatisticsOverview, error) {
	return s.overview.GetOverview(ctx, orgID, filter)
}

func (s *readService) ListClinicianStatistics(ctx context.Context, orgID int64, filter QueryFilter, page, pageSize int) (*domainStatistics.ClinicianStatisticsList, error) {
	return s.clinicianStats.ListClinicianStatistics(ctx, orgID, filter, page, pageSize)
}

func (s *readService) GetClinicianStatistics(ctx context.Context, orgID int64, clinicianID uint64, filter QueryFilter) (*domainStatistics.ClinicianStatistics, error) {
	return s.clinicianStats.GetClinicianStatistics(ctx, orgID, clinicianID, filter)
}

func (s *readService) ListAssessmentEntryStatistics(ctx context.Context, orgID int64, clinicianID *uint64, activeOnly *bool, filter QueryFilter, page, pageSize int) (*domainStatistics.AssessmentEntryStatisticsList, error) {
	return s.entryStats.ListAssessmentEntryStatistics(ctx, orgID, clinicianID, activeOnly, filter, page, pageSize)
}

func (s *readService) GetAssessmentEntryStatistics(ctx context.Context, orgID int64, entryID uint64, filter QueryFilter) (*domainStatistics.AssessmentEntryStatistics, error) {
	return s.entryStats.GetAssessmentEntryStatistics(ctx, orgID, entryID, filter)
}

func (s *readService) GetCurrentClinicianStatistics(ctx context.Context, orgID int64, operatorUserID int64, filter QueryFilter) (*domainStatistics.ClinicianStatistics, error) {
	return s.clinicianStats.GetCurrentClinicianStatistics(ctx, orgID, operatorUserID, filter)
}

func (s *readService) ListCurrentClinicianEntryStatistics(ctx context.Context, orgID int64, operatorUserID int64, filter QueryFilter, page, pageSize int) (*domainStatistics.AssessmentEntryStatisticsList, error) {
	return s.entryStats.ListCurrentClinicianEntryStatistics(ctx, orgID, operatorUserID, filter, page, pageSize)
}

func (s *readService) GetCurrentClinicianTesteeSummary(ctx context.Context, orgID int64, operatorUserID int64, filter QueryFilter) (*domainStatistics.ClinicianTesteeSummaryStatistics, error) {
	return s.clinicianStats.GetCurrentClinicianTesteeSummary(ctx, orgID, operatorUserID, filter)
}

func (s *readService) GetQuestionnaireBatchStatistics(ctx context.Context, orgID int64, codes []string) (*domainStatistics.QuestionnaireBatchStatisticsResponse, error) {
	return s.questionnaireBatch.GetQuestionnaireBatchStatistics(ctx, orgID, codes)
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
