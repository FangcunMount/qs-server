package statistics

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

type stubSystemQueryReader struct {
	loadFn func(context.Context, int64) (*domainStatistics.SystemStatistics, bool, error)
}

func (s *stubSystemQueryReader) LoadSystemStatistics(ctx context.Context, orgID int64) (*domainStatistics.SystemStatistics, bool, error) {
	if s == nil || s.loadFn == nil {
		return nil, false, nil
	}
	return s.loadFn(ctx, orgID)
}

func (s *stubSystemQueryReader) LoadQuestionnaireStatistics(context.Context, int64, string) (*domainStatistics.QuestionnaireStatistics, bool, error) {
	return nil, false, nil
}

func (s *stubSystemQueryReader) LoadPlanStatistics(context.Context, int64, uint64) (*domainStatistics.PlanStatistics, bool, error) {
	return nil, false, nil
}

type stubSystemRealtimeReader struct {
	buildFn func(context.Context, int64) (*domainStatistics.SystemStatistics, error)
}

func (s *stubSystemRealtimeReader) BuildRealtimeSystemStatistics(ctx context.Context, orgID int64) (*domainStatistics.SystemStatistics, error) {
	if s == nil || s.buildFn == nil {
		return nil, errors.New("realtime not implemented")
	}
	return s.buildFn(ctx, orgID)
}

func (s *stubSystemRealtimeReader) BuildRealtimeQuestionnaireStatistics(context.Context, int64, string) (*domainStatistics.QuestionnaireStatistics, error) {
	return nil, nil
}

func (s *stubSystemRealtimeReader) BuildRealtimeTesteeStatistics(context.Context, int64, uint64) (*domainStatistics.TesteeStatistics, error) {
	return nil, nil
}

func (s *stubSystemRealtimeReader) BuildRealtimePlanStatistics(context.Context, int64, uint64) (*domainStatistics.PlanStatistics, error) {
	return nil, nil
}

type memorySystemStatsCache struct {
	mu    sync.Mutex
	stats map[int64]*domainStatistics.SystemStatistics
}

func newMemorySystemStatsCache() *memorySystemStatsCache {
	return &memorySystemStatsCache{stats: make(map[int64]*domainStatistics.SystemStatistics)}
}

func (c *memorySystemStatsCache) LoadSystemStatistics(_ context.Context, orgID int64) (*domainStatistics.SystemStatistics, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	stats, ok := c.stats[orgID]
	return stats, ok
}

func (c *memorySystemStatsCache) StoreSystemStatistics(_ context.Context, orgID int64, stats *domainStatistics.SystemStatistics) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats[orgID] = stats
}

func (c *memorySystemStatsCache) LoadSystemStatisticsCoalesced(
	ctx context.Context,
	orgID int64,
	loader func(context.Context) (*domainStatistics.SystemStatistics, error),
) (*domainStatistics.SystemStatistics, error) {
	if stats, ok := c.LoadSystemStatistics(ctx, orgID); ok {
		return stats, nil
	}
	stats, err := loader(ctx)
	if err != nil || stats == nil {
		return nil, err
	}
	c.StoreSystemStatistics(ctx, orgID, stats)
	return stats, nil
}

func (*memorySystemStatsCache) LoadQuestionnaireStatistics(context.Context, int64, string) (*domainStatistics.QuestionnaireStatistics, bool) {
	return nil, false
}

func (*memorySystemStatsCache) StoreQuestionnaireStatistics(context.Context, int64, string, *domainStatistics.QuestionnaireStatistics) {
}

func (*memorySystemStatsCache) LoadTesteeStatistics(context.Context, int64, uint64) (*domainStatistics.TesteeStatistics, bool) {
	return nil, false
}

func (*memorySystemStatsCache) StoreTesteeStatistics(context.Context, int64, uint64, *domainStatistics.TesteeStatistics) {
}

func (*memorySystemStatsCache) LoadPlanStatistics(context.Context, int64, uint64) (*domainStatistics.PlanStatistics, bool) {
	return nil, false
}

func (*memorySystemStatsCache) StorePlanStatistics(context.Context, int64, uint64, *domainStatistics.PlanStatistics) {
}

func (*memorySystemStatsCache) LoadOverview(context.Context, int64, domainStatistics.StatisticsTimeRange) (*domainStatistics.StatisticsOverview, bool) {
	return nil, false
}

func (*memorySystemStatsCache) StoreOverview(context.Context, int64, domainStatistics.StatisticsTimeRange, *domainStatistics.StatisticsOverview) {
}

func TestSystemStatisticsServiceSingleflightCoalescesConcurrentMiss(t *testing.T) {
	var loads atomic.Int32
	query := &stubSystemQueryReader{
		loadFn: func(ctx context.Context, orgID int64) (*domainStatistics.SystemStatistics, bool, error) {
			loads.Add(1)
			time.Sleep(30 * time.Millisecond)
			return &domainStatistics.SystemStatistics{OrgID: orgID, AssessmentCount: 7}, true, nil
		},
	}
	realtime := &stubSystemRealtimeReader{
		buildFn: func(context.Context, int64) (*domainStatistics.SystemStatistics, error) {
			t.Fatal("realtime should not be called")
			return nil, nil
		},
	}
	cache := newMemorySystemStatsCache()
	service := NewSystemStatisticsService(
		query,
		realtime,
		cache,
		nil,
		WithSystemStatisticsOptions(SystemStatisticsOptions{
			ServiceSingleflight:     true,
			DisableRealtimeFallback: true,
			StaleOnTimeout:          true,
		}),
	)

	const workers = 16
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			stats, err := service.GetSystemStatistics(context.Background(), 1)
			if err != nil {
				t.Errorf("GetSystemStatistics() error = %v", err)
				return
			}
			if stats == nil || stats.AssessmentCount != 7 {
				t.Errorf("GetSystemStatistics() = %+v, want assessment_count=7", stats)
			}
		}()
	}
	wg.Wait()
	if got := loads.Load(); got != 1 {
		t.Fatalf("query loads = %d, want 1", got)
	}
}

func TestSystemStatisticsServiceDisableRealtimeReturnsStale(t *testing.T) {
	query := &stubSystemQueryReader{
		loadFn: func(context.Context, int64) (*domainStatistics.SystemStatistics, bool, error) {
			return nil, false, nil
		},
	}
	realtime := &stubSystemRealtimeReader{
		buildFn: func(context.Context, int64) (*domainStatistics.SystemStatistics, error) {
			t.Fatal("realtime should not be called when disabled")
			return nil, nil
		},
	}
	cache := newMemorySystemStatsCache()
	service := NewSystemStatisticsService(
		query,
		realtime,
		cache,
		nil,
		WithSystemStatisticsOptions(SystemStatisticsOptions{
			ServiceSingleflight:     false,
			DisableRealtimeFallback: true,
			StaleOnTimeout:          true,
		}),
	).(*systemStatisticsService)

	service.guard.RememberStale(1, &domainStatistics.SystemStatistics{OrgID: 1, AssessmentCount: 42})

	stats, err := service.GetSystemStatistics(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetSystemStatistics() error = %v", err)
	}
	if stats == nil || stats.AssessmentCount != 42 {
		t.Fatalf("GetSystemStatistics() = %+v, want stale assessment_count=42", stats)
	}
}
