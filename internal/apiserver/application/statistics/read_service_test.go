package statistics

import (
	"context"
	"fmt"
	"testing"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticscache "github.com/FangcunMount/qs-server/internal/apiserver/port/statisticscache"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type statisticsReadModelStub struct {
	lastOverviewOrgID int64
	lastOverviewFrom  time.Time
	lastOverviewTo    time.Time
	overviewReadCalls int

	lastTrendFrom []time.Time
	lastTrendTo   []time.Time

	lastListEntryPage     int
	lastListEntryPageSize int
	clinicianDetailCalls  int
	clinicianDetailIDs    []uint64
	entryDetailCalls      int
	entryDetailIDs        []uint64

	lastBatchRefs []ContentReference

	organizationOverview     domainStatistics.OrganizationOverview
	accessFunnelWindow       domainStatistics.AccessFunnelWindow
	accessTrend              domainStatistics.AccessFunnelTrend
	assessmentServiceWindow  domainStatistics.AssessmentServiceWindow
	assessmentTrend          domainStatistics.AssessmentServiceTrend
	dimensionAnalysisSummary domainStatistics.DimensionAnalysisSummary
	planTaskWindow           domainStatistics.PlanTaskActivityWindow
	planTrend                domainStatistics.PlanTaskActivityTrend
	planFulfillmentWindow    domainStatistics.PlanTaskFulfillmentWindow
	planFulfillmentTrend     domainStatistics.PlanTaskFulfillmentTrend

	countAssessmentEntriesResult int64
	listAssessmentEntryMetas     []domainStatistics.AssessmentEntryStatisticsMeta
	clinicianSubjects            []domainStatistics.ClinicianStatisticsSubject

	contentBatchTotals []ContentBatchTotal
}

func newReadServiceWithStub(stub *statisticsReadModelStub, opts ...ReadServiceOption) ReadService {
	return NewReadService(ReadServiceDeps{
		Overview:   stub,
		Clinicians: stub,
		Entries:    stub,
		Contents:   stub,
	}, opts...)
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

func (s *statisticsReadModelStub) GetAccessFunnelTrend(_ context.Context, _ int64, from, to time.Time) (domainStatistics.AccessFunnelTrend, error) {
	s.lastTrendFrom = append(s.lastTrendFrom, from)
	s.lastTrendTo = append(s.lastTrendTo, to)
	return s.accessTrend, nil
}

func (s *statisticsReadModelStub) GetAssessmentService(context.Context, int64, time.Time, time.Time) (domainStatistics.AssessmentServiceWindow, error) {
	return s.assessmentServiceWindow, nil
}

func (s *statisticsReadModelStub) GetAssessmentServiceTrend(_ context.Context, _ int64, from, to time.Time) (domainStatistics.AssessmentServiceTrend, error) {
	s.lastTrendFrom = append(s.lastTrendFrom, from)
	s.lastTrendTo = append(s.lastTrendTo, to)
	return s.assessmentTrend, nil
}

func (s *statisticsReadModelStub) GetDimensionAnalysisSummary(context.Context, int64) (domainStatistics.DimensionAnalysisSummary, error) {
	return s.dimensionAnalysisSummary, nil
}

func (s *statisticsReadModelStub) GetPlanTaskOverview(context.Context, int64, time.Time, time.Time) (domainStatistics.PlanTaskActivityWindow, error) {
	return s.planTaskWindow, nil
}

func (s *statisticsReadModelStub) GetPlanTaskTrend(_ context.Context, _ int64, _ *uint64, from, to time.Time) (domainStatistics.PlanTaskActivityTrend, error) {
	s.lastTrendFrom = append(s.lastTrendFrom, from)
	s.lastTrendTo = append(s.lastTrendTo, to)
	return s.planTrend, nil
}

func (s *statisticsReadModelStub) GetPlanTaskFulfillment(context.Context, int64, *uint64, time.Time, time.Time) (domainStatistics.PlanTaskFulfillmentWindow, error) {
	return s.planFulfillmentWindow, nil
}

func (s *statisticsReadModelStub) GetPlanTaskFulfillmentTrend(_ context.Context, _ int64, _ *uint64, from, to time.Time) (domainStatistics.PlanTaskFulfillmentTrend, error) {
	s.lastTrendFrom = append(s.lastTrendFrom, from)
	s.lastTrendTo = append(s.lastTrendTo, to)
	return s.planFulfillmentTrend, nil
}

func (*statisticsReadModelStub) CountClinicianSubjects(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *statisticsReadModelStub) ListClinicianSubjects(context.Context, int64, int, int) ([]domainStatistics.ClinicianStatisticsSubject, error) {
	return append([]domainStatistics.ClinicianStatisticsSubject(nil), s.clinicianSubjects...), nil
}

func (*statisticsReadModelStub) GetClinicianSubject(context.Context, int64, uint64) (*domainStatistics.ClinicianStatisticsSubject, error) {
	return nil, nil
}

func (*statisticsReadModelStub) GetCurrentClinicianSubject(context.Context, int64, int64) (*domainStatistics.ClinicianStatisticsSubject, error) {
	return nil, nil
}

func (s *statisticsReadModelStub) GetClinicianStatisticsDetails(_ context.Context, _ int64, ids []uint64, _, _ time.Time) (map[uint64]ClinicianStatisticsDetail, error) {
	s.clinicianDetailCalls++
	s.clinicianDetailIDs = append([]uint64(nil), ids...)
	return map[uint64]ClinicianStatisticsDetail{}, nil
}

func (*statisticsReadModelStub) GetClinicianSnapshot(context.Context, int64, uint64) (domainStatistics.ClinicianStatisticsSnapshot, error) {
	return domainStatistics.ClinicianStatisticsSnapshot{}, nil
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

func (s *statisticsReadModelStub) GetAssessmentEntryStatisticsDetails(_ context.Context, _ int64, ids []uint64, _, _ time.Time) (map[uint64]AssessmentEntryStatisticsDetail, error) {
	s.entryDetailCalls++
	s.entryDetailIDs = append([]uint64(nil), ids...)
	return map[uint64]AssessmentEntryStatisticsDetail{}, nil
}

func (s *statisticsReadModelStub) GetContentBatchTotals(_ context.Context, _ int64, refs []ContentReference) ([]ContentBatchTotal, error) {
	s.lastBatchRefs = append([]ContentReference(nil), refs...)
	return append([]ContentBatchTotal(nil), s.contentBatchTotals...), nil
}

func TestReadServiceGetOverviewNormalizesQueryFilterBeforeReadModelCalls(t *testing.T) {
	t.Parallel()

	stub := &statisticsReadModelStub{
		organizationOverview: domainStatistics.OrganizationOverview{TesteeCount: 7},
		accessFunnelWindow:   domainStatistics.AccessFunnelWindow{EntryOpenedCount: 3},
		accessTrend: domainStatistics.AccessFunnelTrend{
			EntryOpened: []domainStatistics.DailyCount{
				{Date: time.Date(2026, 4, 1, 0, 0, 0, 0, time.Local), Count: 2},
			},
		},
	}
	service := newReadServiceWithStub(stub)

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
	if len(stub.lastTrendFrom) != 4 {
		t.Fatalf("trend calls = %d, want 4", len(stub.lastTrendFrom))
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
		planTaskWindow: domainStatistics.PlanTaskActivityWindow{TaskCompletedCount: 6},
		planFulfillmentWindow: domainStatistics.PlanTaskFulfillmentWindow{
			DueTaskCount:       10,
			CompletedTaskCount: 7,
			CompletionRate:     70,
		},
	}
	service := newReadServiceWithStub(stub, WithReadServiceCache(cache))
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
	if second.Plan.Activity.Window.TaskCompletedCount != 6 {
		t.Fatalf("plan activity window not populated: %+v", second.Plan)
	}
	if second.Plan.Fulfillment.Window.DueTaskCount != 10 || second.Plan.Fulfillment.Window.CompletionRate != 70 {
		t.Fatalf("plan fulfillment window not populated: %+v", second.Plan.Fulfillment.Window)
	}
}

func TestReadServiceListAssessmentEntryStatisticsNormalizesPaginationBeforeReadModelCall(t *testing.T) {
	t.Parallel()

	stub := &statisticsReadModelStub{
		countAssessmentEntriesResult: 3,
		listAssessmentEntryMetas:     []domainStatistics.AssessmentEntryStatisticsMeta{},
	}
	service := newReadServiceWithStub(stub)

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

func TestReadServiceListsClinicianStatisticsWithOneBatchDetailRead(t *testing.T) {
	t.Parallel()
	stub := &statisticsReadModelStub{clinicianSubjects: []domainStatistics.ClinicianStatisticsSubject{
		{ID: meta.FromUint64(10)}, {ID: meta.FromUint64(20)}, {ID: meta.FromUint64(30)},
	}}
	service := newReadServiceWithStub(stub)
	if _, err := service.ListClinicianStatistics(context.Background(), 12, QueryFilter{}, 1, 20); err != nil {
		t.Fatal(err)
	}
	if stub.clinicianDetailCalls != 1 || fmt.Sprint(stub.clinicianDetailIDs) != "[10 20 30]" {
		t.Fatalf("batch detail calls=%d ids=%v", stub.clinicianDetailCalls, stub.clinicianDetailIDs)
	}
}

func TestReadServiceListsEntryStatisticsWithOneBatchDetailRead(t *testing.T) {
	t.Parallel()
	stub := &statisticsReadModelStub{listAssessmentEntryMetas: []domainStatistics.AssessmentEntryStatisticsMeta{
		{ID: meta.FromUint64(101)}, {ID: meta.FromUint64(202)},
	}}
	service := newReadServiceWithStub(stub)
	if _, err := service.ListAssessmentEntryStatistics(context.Background(), 12, nil, nil, QueryFilter{}, 1, 20); err != nil {
		t.Fatal(err)
	}
	if stub.entryDetailCalls != 1 || fmt.Sprint(stub.entryDetailIDs) != "[101 202]" {
		t.Fatalf("batch detail calls=%d ids=%v", stub.entryDetailCalls, stub.entryDetailIDs)
	}
}

func TestReadServiceGetContentBatchStatisticsKeepsTypedIdentityAndOrder(t *testing.T) {
	t.Parallel()

	stub := &statisticsReadModelStub{
		contentBatchTotals: []ContentBatchTotal{
			{Type: "questionnaire", Code: "COMMON", TotalSubmissions: 10, TotalCompletions: 8},
			{Type: "scale", Code: "COMMON", TotalSubmissions: 4, TotalCompletions: 1},
		},
	}
	service := newReadServiceWithStub(stub)

	got, err := service.GetContentBatchStatistics(context.Background(), 21, []domainStatistics.ContentReference{
		{Type: "QUESTIONNAIRE", Code: " COMMON "},
		{Type: "scale", Code: "COMMON"},
		{Type: "questionnaire", Code: "COMMON"},
	}, ContentStatisticsAccess{Questionnaire: true, Scale: true})
	if err != nil {
		t.Fatalf("GetContentBatchStatistics returned error: %v", err)
	}

	wantRefs := []ContentReference{{Type: "questionnaire", Code: "COMMON"}, {Type: "scale", Code: "COMMON"}}
	if len(stub.lastBatchRefs) != len(wantRefs) {
		t.Fatalf("batch refs len = %d, want %d", len(stub.lastBatchRefs), len(wantRefs))
	}
	for i, want := range wantRefs {
		if stub.lastBatchRefs[i] != want {
			t.Fatalf("batch refs[%d] = %+v, want %+v", i, stub.lastBatchRefs[i], want)
		}
	}
	if len(got.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(got.Items))
	}
	if got.Items[0].Type != domainStatistics.ContentTypeQuestionnaire || got.Items[0].TotalSubmissions != 10 || got.Items[0].TotalCompletions != 8 || got.Items[0].CompletionRate != 80 {
		t.Fatalf("unexpected questionnaire stats: %+v", got.Items[0])
	}
	if got.Items[1].Type != domainStatistics.ContentTypeScale || got.Items[1].TotalSubmissions != 4 || got.Items[1].TotalCompletions != 1 || got.Items[1].CompletionRate != 25 {
		t.Fatalf("unexpected scale stats: %+v", got.Items[1])
	}
}

func TestReadServiceGetContentBatchStatisticsRejectsInvalidReference(t *testing.T) {
	t.Parallel()

	service := newReadServiceWithStub(&statisticsReadModelStub{})
	if _, err := service.GetContentBatchStatistics(context.Background(), 21, []domainStatistics.ContentReference{{Type: "unknown", Code: "X"}}, ContentStatisticsAccess{}); err == nil {
		t.Fatal("GetContentBatchStatistics() error = nil, want invalid argument")
	}
}

func TestReadServiceGetContentBatchStatisticsRequiresAccessForEveryContentType(t *testing.T) {
	t.Parallel()
	service := newReadServiceWithStub(&statisticsReadModelStub{})
	refs := []domainStatistics.ContentReference{
		{Type: domainStatistics.ContentTypeQuestionnaire, Code: "Q-1"},
		{Type: domainStatistics.ContentTypeScale, Code: "S-1"},
	}
	if _, err := service.GetContentBatchStatistics(context.Background(), 21, refs, ContentStatisticsAccess{Questionnaire: true}); err == nil {
		t.Fatal("mixed content request without scale access should fail")
	}
	if _, err := service.GetContentBatchStatistics(context.Background(), 21, refs, ContentStatisticsAccess{Questionnaire: true, Scale: true}); err != nil {
		t.Fatalf("mixed content request with both capabilities failed: %v", err)
	}
}

func TestReadServiceGetContentBatchStatisticsAccessMatrix(t *testing.T) {
	t.Parallel()

	service := newReadServiceWithStub(&statisticsReadModelStub{})
	tests := []struct {
		name    string
		refs    []domainStatistics.ContentReference
		access  ContentStatisticsAccess
		wantErr bool
	}{
		{name: "questionnaire capability", refs: []domainStatistics.ContentReference{{Type: domainStatistics.ContentTypeQuestionnaire, Code: "Q-1"}}, access: ContentStatisticsAccess{Questionnaire: true}},
		{name: "questionnaire denied", refs: []domainStatistics.ContentReference{{Type: domainStatistics.ContentTypeQuestionnaire, Code: "Q-1"}}, access: ContentStatisticsAccess{Scale: true}, wantErr: true},
		{name: "scale capability", refs: []domainStatistics.ContentReference{{Type: domainStatistics.ContentTypeScale, Code: "S-1"}}, access: ContentStatisticsAccess{Scale: true}},
		{name: "scale denied", refs: []domainStatistics.ContentReference{{Type: domainStatistics.ContentTypeScale, Code: "S-1"}}, access: ContentStatisticsAccess{Questionnaire: true}, wantErr: true},
		{name: "empty with questionnaire entry", refs: nil, access: ContentStatisticsAccess{Questionnaire: true}},
		{name: "empty with scale entry", refs: nil, access: ContentStatisticsAccess{Scale: true}},
		{name: "empty denied", refs: nil, access: ContentStatisticsAccess{}, wantErr: true},
		{name: "invalid before permission", refs: []domainStatistics.ContentReference{{Type: "bad", Code: "X"}}, access: ContentStatisticsAccess{}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.GetContentBatchStatistics(context.Background(), 21, tt.refs, tt.access)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

type memoryStatisticsCache struct {
	overview map[string]*domainStatistics.StatisticsOverview
}

func newStatisticsQueryCache(t *testing.T) *memoryStatisticsCache {
	t.Helper()
	return &memoryStatisticsCache{overview: make(map[string]*domainStatistics.StatisticsOverview)}
}

func (c *memoryStatisticsCache) LoadOverview(_ context.Context, orgID int64, timeRange domainStatistics.StatisticsTimeRange) (*domainStatistics.StatisticsOverview, bool) {
	if c == nil {
		return nil, false
	}
	stats, ok := c.overview[statisticsCacheOverviewKey(orgID, timeRange)]
	return stats, ok
}

func (c *memoryStatisticsCache) StoreOverview(_ context.Context, orgID int64, timeRange domainStatistics.StatisticsTimeRange, stats *domainStatistics.StatisticsOverview) {
	if c == nil || stats == nil {
		return
	}
	c.overview[statisticsCacheOverviewKey(orgID, timeRange)] = stats
}

func statisticsCacheOverviewKey(orgID int64, timeRange domainStatistics.StatisticsTimeRange) string {
	return fmt.Sprintf("%d|%s|%s|%s",
		orgID,
		timeRange.Preset,
		timeRange.From.Format(time.RFC3339Nano),
		timeRange.To.Format(time.RFC3339Nano),
	)
}

var _ OverviewReader = (*statisticsReadModelStub)(nil)
var _ ClinicianStatisticsReader = (*statisticsReadModelStub)(nil)
var _ EntryStatisticsReader = (*statisticsReadModelStub)(nil)
var _ ContentStatisticsReader = (*statisticsReadModelStub)(nil)
var _ statisticscache.Cache = (*memoryStatisticsCache)(nil)
