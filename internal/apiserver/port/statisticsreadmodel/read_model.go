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
