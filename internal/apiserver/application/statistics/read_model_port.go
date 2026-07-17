package statistics

import (
	"context"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

// OverviewReader owns the queries used to assemble the organization overview.
type OverviewReader interface {
	GetOrganizationOverview(context.Context, int64) (domainStatistics.OrganizationOverview, error)
	GetAccessFunnel(context.Context, int64, time.Time, time.Time) (domainStatistics.AccessFunnelWindow, error)
	GetAccessFunnelTrend(context.Context, int64, time.Time, time.Time) (domainStatistics.AccessFunnelTrend, error)
	GetAssessmentService(context.Context, int64, time.Time, time.Time) (domainStatistics.AssessmentServiceWindow, error)
	GetAssessmentServiceTrend(context.Context, int64, time.Time, time.Time) (domainStatistics.AssessmentServiceTrend, error)
	GetDimensionAnalysisSummary(context.Context, int64) (domainStatistics.DimensionAnalysisSummary, error)
	GetPlanTaskOverview(context.Context, int64, time.Time, time.Time) (domainStatistics.PlanTaskActivityWindow, error)
	GetPlanTaskTrend(context.Context, int64, *uint64, time.Time, time.Time) (domainStatistics.PlanTaskActivityTrend, error)
	GetPlanTaskFulfillment(context.Context, int64, *uint64, time.Time, time.Time) (domainStatistics.PlanTaskFulfillmentWindow, error)
	GetPlanTaskFulfillmentTrend(context.Context, int64, *uint64, time.Time, time.Time) (domainStatistics.PlanTaskFulfillmentTrend, error)
}

// ClinicianStatisticsReader owns clinician list/detail statistics queries.
type ClinicianStatisticsReader interface {
	CountClinicianSubjects(context.Context, int64) (int64, error)
	ListClinicianSubjects(context.Context, int64, int, int) ([]domainStatistics.ClinicianStatisticsSubject, error)
	GetClinicianSubject(context.Context, int64, uint64) (*domainStatistics.ClinicianStatisticsSubject, error)
	GetCurrentClinicianSubject(context.Context, int64, int64) (*domainStatistics.ClinicianStatisticsSubject, error)
	GetClinicianStatisticsDetails(context.Context, int64, []uint64, time.Time, time.Time) (map[uint64]ClinicianStatisticsDetail, error)
	GetClinicianSnapshot(context.Context, int64, uint64) (domainStatistics.ClinicianStatisticsSnapshot, error)
	GetClinicianTesteeSummaryCounts(context.Context, int64, uint64, time.Time, time.Time) (int64, int64, error)
}

type ClinicianStatisticsDetail struct {
	Snapshot domainStatistics.ClinicianStatisticsSnapshot
	Window   domainStatistics.ClinicianStatisticsWindow
	Funnel   domainStatistics.ClinicianStatisticsFunnel
}

// EntryStatisticsReader owns assessment-entry list/detail statistics queries.
type EntryStatisticsReader interface {
	CountAssessmentEntries(context.Context, int64, *uint64, *bool) (int64, error)
	ListAssessmentEntryMetas(context.Context, int64, *uint64, *bool, int, int) ([]domainStatistics.AssessmentEntryStatisticsMeta, error)
	GetAssessmentEntryMeta(context.Context, int64, uint64) (*domainStatistics.AssessmentEntryStatisticsMeta, error)
	GetAssessmentEntryStatisticsDetails(context.Context, int64, []uint64, time.Time, time.Time) (map[uint64]AssessmentEntryStatisticsDetail, error)
	GetCurrentClinicianSubject(context.Context, int64, int64) (*domainStatistics.ClinicianStatisticsSubject, error)
}

type AssessmentEntryStatisticsDetail struct {
	Snapshot       domainStatistics.AssessmentEntryStatisticsCounts
	Window         domainStatistics.AssessmentEntryStatisticsCounts
	LastResolvedAt *time.Time
	LastIntakeAt   *time.Time
}

type ContentReference struct {
	Type string
	Code string
}

type ContentBatchTotal struct {
	Type             string
	Code             string
	TotalSubmissions int64
	TotalCompletions int64
}

// ContentStatisticsReader owns typed content statistics queries.
type ContentStatisticsReader interface {
	GetContentBatchTotals(context.Context, int64, []ContentReference) ([]ContentBatchTotal, error)
}

type StatisticsReadModel interface {
	OverviewReader
	ClinicianStatisticsReader
	EntryStatisticsReader
	ContentStatisticsReader
}
