package statistics

import (
	"context"
	"testing"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	cachepolicy "github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

type statisticsReadModelStub struct {
	lastOverviewOrgID int64
	lastOverviewFrom  time.Time
	lastOverviewTo    time.Time
	overviewReadCalls int

	lastTrendMetrics []OrgOverviewMetric
	lastTrendFrom    []time.Time
	lastTrendTo      []time.Time

	lastListEntryPage     int
	lastListEntryPageSize int

	lastBatchCodes []string

	orgOverviewSnapshot      domainStatistics.OrgOverviewSnapshot
	orgOverviewWindow        domainStatistics.OrgOverviewWindow
	trendByMetric            map[OrgOverviewMetric][]domainStatistics.DailyCount
	organizationOverview     domainStatistics.OrganizationOverview
	accessFunnelWindow       domainStatistics.AccessFunnelWindow
	accessTrendByMetric      map[AccessFunnelMetric][]domainStatistics.DailyCount
	assessmentServiceWindow  domainStatistics.AssessmentServiceWindow
	assessmentTrendByMetric  map[AssessmentServiceMetric][]domainStatistics.DailyCount
	dimensionAnalysisSummary domainStatistics.DimensionAnalysisSummary
	planTaskWindow           domainStatistics.PlanTaskWindow
	planTrendByMetric        map[PlanTaskMetric][]domainStatistics.DailyCount

	countAssessmentEntriesResult int64
	listAssessmentEntryMetas     []domainStatistics.AssessmentEntryStatisticsMeta

	questionnaireBatchTotals []QuestionnaireBatchTotal
}

func (s *statisticsReadModelStub) GetOrgOverviewSnapshot(context.Context, int64) (domainStatistics.OrgOverviewSnapshot, error) {
	return s.orgOverviewSnapshot, nil
}

func (s *statisticsReadModelStub) GetOrgOverviewWindow(_ context.Context, orgID int64, from, to time.Time) (domainStatistics.OrgOverviewWindow, error) {
	s.lastOverviewOrgID = orgID
	s.lastOverviewFrom = from
	s.lastOverviewTo = to
	return s.orgOverviewWindow, nil
}

func (s *statisticsReadModelStub) ListOrgOverviewTrend(_ context.Context, _ int64, metric OrgOverviewMetric, from, to time.Time) []domainStatistics.DailyCount {
	s.lastTrendMetrics = append(s.lastTrendMetrics, metric)
	s.lastTrendFrom = append(s.lastTrendFrom, from)
	s.lastTrendTo = append(s.lastTrendTo, to)
	return append([]domainStatistics.DailyCount(nil), s.trendByMetric[metric]...)
}

func (s *statisticsReadModelStub) GetOrganizationOverview(context.Context, int64) (domainStatistics.OrganizationOverview, error) {
	s.overviewReadCalls++
	return s.organizationOverview, nil
}

func (s *statisticsReadModelStub) GetAccessFunnel(_ context.Context, orgID int64, from, to time.Time) (domainStatistics.AccessFunnelWindow, error) {
	s.lastOverviewOrgID = orgID
	s.lastOverviewFrom = from
	s.lastOverviewTo = to
	return s.accessFunnelWindow, nil
}

func (s *statisticsReadModelStub) ListAccessFunnelTrend(_ context.Context, _ int64, metric AccessFunnelMetric, from, to time.Time) []domainStatistics.DailyCount {
	s.lastTrendFrom = append(s.lastTrendFrom, from)
	s.lastTrendTo = append(s.lastTrendTo, to)
	return append([]domainStatistics.DailyCount(nil), s.accessTrendByMetric[metric]...)
}

func (s *statisticsReadModelStub) GetAccessFunnelTrend(_ context.Context, _ int64, from, to time.Time) (domainStatistics.AccessFunnelTrend, error) {
	s.lastTrendFrom = append(s.lastTrendFrom, from)
	s.lastTrendTo = append(s.lastTrendTo, to)
	return domainStatistics.AccessFunnelTrend{
		EntryOpened:                 append([]domainStatistics.DailyCount(nil), s.accessTrendByMetric[AccessFunnelMetricEntryOpened]...),
		IntakeConfirmed:             append([]domainStatistics.DailyCount(nil), s.accessTrendByMetric[AccessFunnelMetricIntakeConfirmed]...),
		TesteeCreated:               append([]domainStatistics.DailyCount(nil), s.accessTrendByMetric[AccessFunnelMetricTesteeCreated]...),
		CareRelationshipEstablished: append([]domainStatistics.DailyCount(nil), s.accessTrendByMetric[AccessFunnelMetricCareRelationshipEstablished]...),
	}, nil
}

func (s *statisticsReadModelStub) GetAssessmentService(context.Context, int64, time.Time, time.Time) (domainStatistics.AssessmentServiceWindow, error) {
	return s.assessmentServiceWindow, nil
}

func (s *statisticsReadModelStub) ListAssessmentServiceTrend(_ context.Context, _ int64, metric AssessmentServiceMetric, from, to time.Time) []domainStatistics.DailyCount {
	s.lastTrendFrom = append(s.lastTrendFrom, from)
	s.lastTrendTo = append(s.lastTrendTo, to)
	return append([]domainStatistics.DailyCount(nil), s.assessmentTrendByMetric[metric]...)
}

func (s *statisticsReadModelStub) GetAssessmentServiceTrend(_ context.Context, _ int64, from, to time.Time) (domainStatistics.AssessmentServiceTrend, error) {
	s.lastTrendFrom = append(s.lastTrendFrom, from)
	s.lastTrendTo = append(s.lastTrendTo, to)
	return domainStatistics.AssessmentServiceTrend{
		AnswerSheetSubmitted: append([]domainStatistics.DailyCount(nil), s.assessmentTrendByMetric[AssessmentServiceMetricAnswerSheetSubmitted]...),
		AssessmentCreated:    append([]domainStatistics.DailyCount(nil), s.assessmentTrendByMetric[AssessmentServiceMetricAssessmentCreated]...),
		ReportGenerated:      append([]domainStatistics.DailyCount(nil), s.assessmentTrendByMetric[AssessmentServiceMetricReportGenerated]...),
		AssessmentFailed:     append([]domainStatistics.DailyCount(nil), s.assessmentTrendByMetric[AssessmentServiceMetricAssessmentFailed]...),
	}, nil
}

func (s *statisticsReadModelStub) GetDimensionAnalysisSummary(context.Context, int64) (domainStatistics.DimensionAnalysisSummary, error) {
	return s.dimensionAnalysisSummary, nil
}

func (s *statisticsReadModelStub) GetPlanTaskOverview(context.Context, int64, time.Time, time.Time) (domainStatistics.PlanTaskWindow, error) {
	return s.planTaskWindow, nil
}

func (s *statisticsReadModelStub) GetPlanTaskOverviewByPlan(context.Context, int64, uint64, time.Time, time.Time) (domainStatistics.PlanTaskWindow, error) {
	return s.planTaskWindow, nil
}

func (s *statisticsReadModelStub) ListPlanTaskTrend(_ context.Context, _ int64, _ *uint64, metric PlanTaskMetric, from, to time.Time) []domainStatistics.DailyCount {
	s.lastTrendFrom = append(s.lastTrendFrom, from)
	s.lastTrendTo = append(s.lastTrendTo, to)
	return append([]domainStatistics.DailyCount(nil), s.planTrendByMetric[metric]...)
}

func (s *statisticsReadModelStub) GetPlanTaskTrend(_ context.Context, _ int64, _ *uint64, from, to time.Time) (domainStatistics.PlanTaskTrend, error) {
	s.lastTrendFrom = append(s.lastTrendFrom, from)
	s.lastTrendTo = append(s.lastTrendTo, to)
	return domainStatistics.PlanTaskTrend{
		TaskCreated:   append([]domainStatistics.DailyCount(nil), s.planTrendByMetric[PlanTaskMetricCreated]...),
		TaskOpened:    append([]domainStatistics.DailyCount(nil), s.planTrendByMetric[PlanTaskMetricOpened]...),
		TaskCompleted: append([]domainStatistics.DailyCount(nil), s.planTrendByMetric[PlanTaskMetricCompleted]...),
		TaskExpired:   append([]domainStatistics.DailyCount(nil), s.planTrendByMetric[PlanTaskMetricExpired]...),
	}, nil
}

func (*statisticsReadModelStub) CountClinicianSubjects(context.Context, int64) (int64, error) {
	return 0, nil
}

func (*statisticsReadModelStub) ListClinicianSubjects(context.Context, int64, int, int) ([]domainStatistics.ClinicianStatisticsSubject, error) {
	return nil, nil
}

func (*statisticsReadModelStub) GetClinicianSubject(context.Context, int64, uint64) (*domainStatistics.ClinicianStatisticsSubject, error) {
	return nil, nil
}

func (*statisticsReadModelStub) GetCurrentClinicianSubject(context.Context, int64, int64) (*domainStatistics.ClinicianStatisticsSubject, error) {
	return nil, nil
}

func (*statisticsReadModelStub) GetClinicianSnapshot(context.Context, int64, uint64) (domainStatistics.ClinicianStatisticsSnapshot, error) {
	return domainStatistics.ClinicianStatisticsSnapshot{}, nil
}

func (*statisticsReadModelStub) GetClinicianProjection(context.Context, int64, uint64, time.Time, time.Time) (domainStatistics.ClinicianStatisticsWindow, domainStatistics.ClinicianStatisticsFunnel, error) {
	return domainStatistics.ClinicianStatisticsWindow{}, domainStatistics.ClinicianStatisticsFunnel{}, nil
}

func (*statisticsReadModelStub) GetClinicianTesteeSummaryCounts(context.Context, int64, uint64, time.Time, time.Time) (int64, int64, error) {
	return 0, 0, nil
}

func (s *statisticsReadModelStub) CountAssessmentEntries(context.Context, int64, *uint64, *bool) (int64, error) {
	return s.countAssessmentEntriesResult, nil
}

func (s *statisticsReadModelStub) ListAssessmentEntryMetas(_ context.Context, _ int64, _ *uint64, _ *bool, page, pageSize int) ([]domainStatistics.AssessmentEntryStatisticsMeta, error) {
	s.lastListEntryPage = page
	s.lastListEntryPageSize = pageSize
	return append([]domainStatistics.AssessmentEntryStatisticsMeta(nil), s.listAssessmentEntryMetas...), nil
}

func (*statisticsReadModelStub) GetAssessmentEntryMeta(context.Context, int64, uint64) (*domainStatistics.AssessmentEntryStatisticsMeta, error) {
	return nil, nil
}

func (*statisticsReadModelStub) GetAssessmentEntryCounts(context.Context, int64, uint64, *time.Time, *time.Time) (domainStatistics.AssessmentEntryStatisticsCounts, error) {
	return domainStatistics.AssessmentEntryStatisticsCounts{}, nil
}

func (*statisticsReadModelStub) GetAssessmentEntryLastEventTime(context.Context, int64, uint64, domainStatistics.BehaviorEventName) (*time.Time, error) {
	return nil, nil
}

func (s *statisticsReadModelStub) GetQuestionnaireBatchTotals(_ context.Context, _ int64, codes []string) ([]QuestionnaireBatchTotal, error) {
	s.lastBatchCodes = append([]string(nil), codes...)
	return append([]QuestionnaireBatchTotal(nil), s.questionnaireBatchTotals...), nil
}

type answerSheetRepoStub struct {
	countsByQuestionnaire map[string]int64
}

func (*answerSheetRepoStub) Create(context.Context, *domainAnswerSheet.AnswerSheet) error { return nil }

func (*answerSheetRepoStub) Update(context.Context, *domainAnswerSheet.AnswerSheet) error { return nil }

func (*answerSheetRepoStub) FindByID(context.Context, meta.ID) (*domainAnswerSheet.AnswerSheet, error) {
	return nil, nil
}

func (*answerSheetRepoStub) FindSummaryListByFiller(context.Context, uint64, int, int) ([]*domainAnswerSheet.AnswerSheetSummary, error) {
	return nil, nil
}

func (*answerSheetRepoStub) FindSummaryListByQuestionnaire(context.Context, string, int, int) ([]*domainAnswerSheet.AnswerSheetSummary, error) {
	return nil, nil
}

func (*answerSheetRepoStub) CountByFiller(context.Context, uint64) (int64, error) { return 0, nil }

func (s *answerSheetRepoStub) CountByQuestionnaire(_ context.Context, questionnaireCode string) (int64, error) {
	return s.countsByQuestionnaire[questionnaireCode], nil
}

func (*answerSheetRepoStub) CountWithConditions(context.Context, map[string]interface{}) (int64, error) {
	return 0, nil
}

func (*answerSheetRepoStub) Delete(context.Context, meta.ID) error { return nil }

func TestReadServiceGetOverviewNormalizesQueryFilterBeforeReadModelCalls(t *testing.T) {
	t.Parallel()

	stub := &statisticsReadModelStub{
		organizationOverview: domainStatistics.OrganizationOverview{TesteeCount: 7},
		accessFunnelWindow:   domainStatistics.AccessFunnelWindow{EntryOpenedCount: 3},
		accessTrendByMetric: map[AccessFunnelMetric][]domainStatistics.DailyCount{
			AccessFunnelMetricEntryOpened: {
				{Date: time.Date(2026, 4, 1, 0, 0, 0, 0, time.Local), Count: 2},
			},
		},
		assessmentTrendByMetric: map[AssessmentServiceMetric][]domainStatistics.DailyCount{},
		planTrendByMetric:       map[PlanTaskMetric][]domainStatistics.DailyCount{},
	}
	service := NewReadService(stub, nil)

	got, err := service.GetOverview(context.Background(), 9, QueryFilter{
		From: "2026-04-01",
		To:   "2026-04-02",
	})
	if err != nil {
		t.Fatalf("GetOverview returned error: %v", err)
	}

	wantFrom := time.Date(2026, 4, 1, 0, 0, 0, 0, time.Local)
	wantTo := time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local)
	if !stub.lastOverviewFrom.Equal(wantFrom) || !stub.lastOverviewTo.Equal(wantTo) {
		t.Fatalf("overview range = [%v,%v), want [%v,%v)", stub.lastOverviewFrom, stub.lastOverviewTo, wantFrom, wantTo)
	}
	if len(stub.lastTrendFrom) != 3 {
		t.Fatalf("trend calls = %d, want 3", len(stub.lastTrendFrom))
	}
	if got.OrganizationOverview.TesteeCount != 7 || got.AccessFunnel.Window.EntryOpenedCount != 3 {
		t.Fatalf("unexpected overview payload: %+v", got)
	}
	if len(got.AccessFunnel.Trend.EntryOpened) != 2 {
		t.Fatalf("filled access trend len = %d, want 2", len(got.AccessFunnel.Trend.EntryOpened))
	}
	if got.AccessFunnel.Trend.EntryOpened[1].Date.Format("2006-01-02") != "2026-04-02" || got.AccessFunnel.Trend.EntryOpened[1].Count != 0 {
		t.Fatalf("unexpected filled trend point: %+v", got.AccessFunnel.Trend.EntryOpened[1])
	}
}

func TestReadServiceGetOverviewUsesCacheAside(t *testing.T) {
	t.Parallel()

	cache := newStatisticsQueryCache(t)
	stub := &statisticsReadModelStub{
		organizationOverview: domainStatistics.OrganizationOverview{TesteeCount: 17},
		accessFunnelWindow: domainStatistics.AccessFunnelWindow{
			EntryOpenedCount:     5,
			IntakeConfirmedCount: 4,
			TesteeCreatedCount:   3,
		},
		assessmentServiceWindow: domainStatistics.AssessmentServiceWindow{AssessmentCreatedCount: 8},
		dimensionAnalysisSummary: domainStatistics.DimensionAnalysisSummary{
			ClinicianCount: 2,
			EntryCount:     1,
			ContentCount:   4,
		},
		planTaskWindow: domainStatistics.PlanTaskWindow{TaskCompletedCount: 6},
	}
	service := NewReadService(stub, nil, WithReadServiceCache(cache))
	filter := QueryFilter{
		From: "2026-04-01",
		To:   "2026-04-02",
	}

	first, err := service.GetOverview(context.Background(), 11, filter)
	if err != nil {
		t.Fatalf("first GetOverview returned error: %v", err)
	}
	if stub.overviewReadCalls != 1 {
		t.Fatalf("overview read calls after first request = %d, want 1", stub.overviewReadCalls)
	}

	stub.organizationOverview = domainStatistics.OrganizationOverview{TesteeCount: 999}
	second, err := service.GetOverview(context.Background(), 11, filter)
	if err != nil {
		t.Fatalf("second GetOverview returned error: %v", err)
	}
	if stub.overviewReadCalls != 1 {
		t.Fatalf("overview read calls after cache hit = %d, want 1", stub.overviewReadCalls)
	}
	if second.OrganizationOverview.TesteeCount != first.OrganizationOverview.TesteeCount || second.OrganizationOverview.TesteeCount != 17 {
		t.Fatalf("cached overview testee count = %d, want 17", second.OrganizationOverview.TesteeCount)
	}
	if second.AccessFunnel.Window.EntryOpenedCount != first.AccessFunnel.Window.EntryOpenedCount {
		t.Fatalf("cached access funnel changed: got %d want %d", second.AccessFunnel.Window.EntryOpenedCount, first.AccessFunnel.Window.EntryOpenedCount)
	}
}

func TestReadServiceListAssessmentEntryStatisticsNormalizesPaginationBeforeReadModelCall(t *testing.T) {
	t.Parallel()

	stub := &statisticsReadModelStub{
		countAssessmentEntriesResult: 3,
		listAssessmentEntryMetas:     []domainStatistics.AssessmentEntryStatisticsMeta{},
	}
	service := NewReadService(stub, nil)

	got, err := service.ListAssessmentEntryStatistics(context.Background(), 12, nil, nil, QueryFilter{}, 0, 500)
	if err != nil {
		t.Fatalf("ListAssessmentEntryStatistics returned error: %v", err)
	}

	if stub.lastListEntryPage != 1 || stub.lastListEntryPageSize != 100 {
		t.Fatalf("page = (%d,%d), want (1,100)", stub.lastListEntryPage, stub.lastListEntryPageSize)
	}
	if got.Page != 1 || got.PageSize != 100 || got.Total != 3 {
		t.Fatalf("unexpected list payload: %+v", got)
	}
}

func TestReadServiceGetQuestionnaireBatchStatisticsDeduplicatesKeepsOrderAndFallsBackToAnswerSheetRepo(t *testing.T) {
	t.Parallel()

	stub := &statisticsReadModelStub{
		questionnaireBatchTotals: []QuestionnaireBatchTotal{
			{Code: "PHQ9", TotalSubmissions: 10, TotalCompletions: 8},
		},
	}
	answerSheetRepo := &answerSheetRepoStub{
		countsByQuestionnaire: map[string]int64{
			"GAD7": 4,
		},
	}
	service := NewReadService(stub, answerSheetRepo)

	got, err := service.GetQuestionnaireBatchStatistics(context.Background(), 21, []string{" PHQ9 ", "GAD7", "", "PHQ9", "SCL90"})
	if err != nil {
		t.Fatalf("GetQuestionnaireBatchStatistics returned error: %v", err)
	}

	wantCodes := []string{"PHQ9", "GAD7", "SCL90"}
	if len(stub.lastBatchCodes) != len(wantCodes) {
		t.Fatalf("batch codes len = %d, want %d", len(stub.lastBatchCodes), len(wantCodes))
	}
	for i, want := range wantCodes {
		if stub.lastBatchCodes[i] != want {
			t.Fatalf("batch codes[%d] = %q, want %q", i, stub.lastBatchCodes[i], want)
		}
	}
	if len(got.Items) != 3 {
		t.Fatalf("items len = %d, want 3", len(got.Items))
	}
	if got.Items[0].Code != "PHQ9" || got.Items[0].TotalSubmissions != 10 || got.Items[0].TotalCompletions != 8 {
		t.Fatalf("unexpected PHQ9 stats: %+v", got.Items[0])
	}
	if got.Items[1].Code != "GAD7" || got.Items[1].TotalSubmissions != 4 || got.Items[1].TotalCompletions != 4 || got.Items[1].CompletionRate != 100 {
		t.Fatalf("unexpected GAD7 fallback stats: %+v", got.Items[1])
	}
	if got.Items[2].Code != "SCL90" || got.Items[2].TotalSubmissions != 0 || got.Items[2].TotalCompletions != 0 {
		t.Fatalf("unexpected SCL90 stats: %+v", got.Items[2])
	}
}

func newStatisticsQueryCache(t *testing.T) *statisticsCache.StatisticsCache {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewUniversalClient(&redis.UniversalOptions{Addrs: []string{mr.Addr()}})
	t.Cleanup(func() {
		_ = client.Close()
	})
	return statisticsCache.NewStatisticsCacheWithBuilderAndPolicy(
		client,
		keyspace.NewBuilderWithNamespace("stats-test"),
		cachepolicy.CachePolicy{},
	)
}

var _ StatisticsReadModel = (*statisticsReadModelStub)(nil)
var _ domainAnswerSheet.Repository = (*answerSheetRepoStub)(nil)
