package statistics

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticscache "github.com/FangcunMount/qs-server/internal/apiserver/port/statisticscache"
	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
)

type questionnaireStatisticsService struct {
	query    StatisticsQueryReader
	realtime StatisticsRealtimeReader
	cache    statisticscache.Cache
	hotset   cachetarget.HotsetRecorder
	guard    *loadguard.Guard[string, *statistics.QuestionnaireStatistics]
}

type QuestionnaireStatisticsServiceOption func(*questionnaireStatisticsService)

func WithQuestionnaireStatisticsGuard(opts StatisticsReadGuardOptions) QuestionnaireStatisticsServiceOption {
	return func(s *questionnaireStatisticsService) {
		s.guard = loadguard.New[string, *statistics.QuestionnaireStatistics](opts.ToLoadGuardPolicy(), cloneQuestionnaireStatistics, func() {
			incStatsQuestionnaireStaleServed()
		})
	}
}

func NewQuestionnaireStatisticsService(
	query StatisticsQueryReader,
	realtime StatisticsRealtimeReader,
	cache statisticscache.Cache,
	hotset cachetarget.HotsetRecorder,
	opts ...QuestionnaireStatisticsServiceOption,
) QuestionnaireStatisticsService {
	service := &questionnaireStatisticsService{
		query:    query,
		realtime: realtime,
		cache:    cache,
		hotset:   hotset,
		guard: loadguard.New[string, *statistics.QuestionnaireStatistics](
			DefaultQuestionnaireStatisticsGuardOptions().ToLoadGuardPolicy(),
			cloneQuestionnaireStatistics,
			func() { incStatsQuestionnaireStaleServed() },
		),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
}

func (s *questionnaireStatisticsService) GetQuestionnaireStatistics(
	ctx context.Context,
	orgID int64,
	questionnaireCode string,
) (*statistics.QuestionnaireStatistics, error) {
	l := logger.L(ctx)
	l.Infow("获取问卷统计", "org_id", orgID, "questionnaire_code", questionnaireCode)
	if stats, ok := s.loadCachedQuestionnaireStatistics(ctx, orgID, questionnaireCode); ok {
		s.recordHotset(ctx, cachetarget.NewQueryStatsQuestionnaireWarmupTarget(orgID, questionnaireCode))
		return stats, nil
	}

	key := fmt.Sprintf("questionnaire-stats:%d:%s", orgID, questionnaireCode)
	stats, err := s.guard.Load(ctx, key, func(loadCtx context.Context) (*statistics.QuestionnaireStatistics, error) {
		return s.loadQuestionnaireStatisticsMiss(loadCtx, orgID, questionnaireCode)
	})
	if err != nil {
		return nil, err
	}
	s.cacheQuestionnaireStatistics(ctx, orgID, questionnaireCode, stats)
	s.recordHotset(ctx, cachetarget.NewQueryStatsQuestionnaireWarmupTarget(orgID, questionnaireCode))
	return stats, nil
}

func (s *questionnaireStatisticsService) loadQuestionnaireStatisticsMiss(
	ctx context.Context,
	orgID int64,
	questionnaireCode string,
) (*statistics.QuestionnaireStatistics, error) {
	l := logger.L(ctx)
	if s.query != nil {
		stats, found, err := s.query.LoadQuestionnaireStatistics(ctx, orgID, questionnaireCode)
		if err != nil {
			return nil, err
		}
		if found {
			l.Debugw("从MySQL统计表获取问卷统计")
			return stats, nil
		}
	}

	l.Debugw("从原始表实时聚合问卷统计")
	return s.realtime.BuildRealtimeQuestionnaireStatistics(ctx, orgID, questionnaireCode)
}

func (s *questionnaireStatisticsService) loadCachedQuestionnaireStatistics(
	ctx context.Context,
	orgID int64,
	questionnaireCode string,
) (*statistics.QuestionnaireStatistics, bool) {
	if s.cache == nil {
		return nil, false
	}
	return s.cache.LoadQuestionnaireStatistics(ctx, orgID, questionnaireCode)
}

func (s *questionnaireStatisticsService) cacheQuestionnaireStatistics(
	ctx context.Context,
	orgID int64,
	questionnaireCode string,
	stats *statistics.QuestionnaireStatistics,
) {
	if s.cache == nil || stats == nil {
		return
	}
	s.cache.StoreQuestionnaireStatistics(ctx, orgID, questionnaireCode, stats)
}

func daysAgo(days int) *time.Time {
	t := time.Now().AddDate(0, 0, -days)
	return &t
}

func (s *questionnaireStatisticsService) recordHotset(ctx context.Context, target cachetarget.WarmupTarget) {
	if s == nil || s.hotset == nil {
		return
	}
	_ = s.hotset.Record(ctx, target)
}

func cloneQuestionnaireStatistics(stats *statistics.QuestionnaireStatistics) *statistics.QuestionnaireStatistics {
	if stats == nil {
		return nil
	}
	data, err := json.Marshal(stats)
	if err != nil {
		cloned := *stats
		return &cloned
	}
	var out statistics.QuestionnaireStatistics
	if err := json.Unmarshal(data, &out); err != nil {
		cloned := *stats
		return &cloned
	}
	return &out
}
