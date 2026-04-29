package statisticsreadmodel

import (
	"context"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

// OrgOverviewMetric identifies which overview trend metric to load.
type OrgOverviewMetric string

const (
	OrgOverviewMetricAssessmentCreated OrgOverviewMetric = "assessment_created"
	OrgOverviewMetricIntakeConfirmed   OrgOverviewMetric = "intake_confirmed"
	OrgOverviewMetricRelationAssigned  OrgOverviewMetric = "relation_assigned"
)

// AccessFunnelMetric identifies access-domain trend metrics.
type AccessFunnelMetric string

const (
	AccessFunnelMetricEntryOpened                 AccessFunnelMetric = "entry_opened"
	AccessFunnelMetricIntakeConfirmed             AccessFunnelMetric = "intake_confirmed"
	AccessFunnelMetricTesteeCreated               AccessFunnelMetric = "testee_created"
	AccessFunnelMetricCareRelationshipEstablished AccessFunnelMetric = "care_relationship_established"
)

// AssessmentServiceMetric identifies assessment-service-domain trend metrics.
type AssessmentServiceMetric string

const (
	AssessmentServiceMetricAnswerSheetSubmitted AssessmentServiceMetric = "answersheet_submitted"
	AssessmentServiceMetricAssessmentCreated    AssessmentServiceMetric = "assessment_created"
	AssessmentServiceMetricReportGenerated      AssessmentServiceMetric = "report_generated"
	AssessmentServiceMetricAssessmentFailed     AssessmentServiceMetric = "assessment_failed"
)

// PlanTaskMetric identifies plan task trend metrics.
type PlanTaskMetric string

const (
	PlanTaskMetricCreated   PlanTaskMetric = "task_created"
	PlanTaskMetricOpened    PlanTaskMetric = "task_opened"
	PlanTaskMetricCompleted PlanTaskMetric = "task_completed"
	PlanTaskMetricExpired   PlanTaskMetric = "task_expired"
)

// QuestionnaireBatchTotal carries questionnaire totals loaded from the read model.
type QuestionnaireBatchTotal struct {
	Code             string
	TotalSubmissions int64
	TotalCompletions int64
}

// ReadModel exposes statistics read-side queries needed by the application service.
type ReadModel interface {
	GetOrgOverviewSnapshot(ctx context.Context, orgID int64) (domainStatistics.OrgOverviewSnapshot, error)
	GetOrgOverviewWindow(ctx context.Context, orgID int64, from, to time.Time) (domainStatistics.OrgOverviewWindow, error)
	ListOrgOverviewTrend(ctx context.Context, orgID int64, metric OrgOverviewMetric, from, to time.Time) []domainStatistics.DailyCount
	GetOrganizationOverview(ctx context.Context, orgID int64) (domainStatistics.OrganizationOverview, error)
	GetAccessFunnel(ctx context.Context, orgID int64, from, to time.Time) (domainStatistics.AccessFunnelWindow, error)
	GetAccessFunnelTrend(ctx context.Context, orgID int64, from, to time.Time) (domainStatistics.AccessFunnelTrend, error)
	ListAccessFunnelTrend(ctx context.Context, orgID int64, metric AccessFunnelMetric, from, to time.Time) []domainStatistics.DailyCount
	GetAssessmentService(ctx context.Context, orgID int64, from, to time.Time) (domainStatistics.AssessmentServiceWindow, error)
	GetAssessmentServiceTrend(ctx context.Context, orgID int64, from, to time.Time) (domainStatistics.AssessmentServiceTrend, error)
	ListAssessmentServiceTrend(ctx context.Context, orgID int64, metric AssessmentServiceMetric, from, to time.Time) []domainStatistics.DailyCount
	GetDimensionAnalysisSummary(ctx context.Context, orgID int64) (domainStatistics.DimensionAnalysisSummary, error)
	GetPlanTaskOverview(ctx context.Context, orgID int64, from, to time.Time) (domainStatistics.PlanTaskWindow, error)
	GetPlanTaskOverviewByPlan(ctx context.Context, orgID int64, planID uint64, from, to time.Time) (domainStatistics.PlanTaskWindow, error)
	GetPlanTaskTrend(ctx context.Context, orgID int64, planID *uint64, from, to time.Time) (domainStatistics.PlanTaskTrend, error)
	ListPlanTaskTrend(ctx context.Context, orgID int64, planID *uint64, metric PlanTaskMetric, from, to time.Time) []domainStatistics.DailyCount

	CountClinicianSubjects(ctx context.Context, orgID int64) (int64, error)
	ListClinicianSubjects(ctx context.Context, orgID int64, page, pageSize int) ([]domainStatistics.ClinicianStatisticsSubject, error)
	GetClinicianSubject(ctx context.Context, orgID int64, clinicianID uint64) (*domainStatistics.ClinicianStatisticsSubject, error)
	GetCurrentClinicianSubject(ctx context.Context, orgID int64, operatorUserID int64) (*domainStatistics.ClinicianStatisticsSubject, error)
	GetClinicianSnapshot(ctx context.Context, orgID int64, clinicianID uint64) (domainStatistics.ClinicianStatisticsSnapshot, error)
	GetClinicianProjection(ctx context.Context, orgID int64, clinicianID uint64, from, to time.Time) (domainStatistics.ClinicianStatisticsWindow, domainStatistics.ClinicianStatisticsFunnel, error)
	GetClinicianTesteeSummaryCounts(ctx context.Context, orgID int64, clinicianID uint64, from, to time.Time) (int64, int64, error)

	CountAssessmentEntries(ctx context.Context, orgID int64, clinicianID *uint64, activeOnly *bool) (int64, error)
	ListAssessmentEntryMetas(ctx context.Context, orgID int64, clinicianID *uint64, activeOnly *bool, page, pageSize int) ([]domainStatistics.AssessmentEntryStatisticsMeta, error)
	GetAssessmentEntryMeta(ctx context.Context, orgID int64, entryID uint64) (*domainStatistics.AssessmentEntryStatisticsMeta, error)
	GetAssessmentEntryCounts(ctx context.Context, orgID int64, entryID uint64, from, to *time.Time) (domainStatistics.AssessmentEntryStatisticsCounts, error)
	GetAssessmentEntryLastEventTime(ctx context.Context, orgID int64, entryID uint64, eventName domainStatistics.BehaviorEventName) (*time.Time, error)

	GetQuestionnaireBatchTotals(ctx context.Context, orgID int64, codes []string) ([]QuestionnaireBatchTotal, error)
}
