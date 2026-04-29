package statistics

import statisticsreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/statisticsreadmodel"

type StatisticsReadModel = statisticsreadmodel.ReadModel

type OrgOverviewMetric = statisticsreadmodel.OrgOverviewMetric
type AccessFunnelMetric = statisticsreadmodel.AccessFunnelMetric
type AssessmentServiceMetric = statisticsreadmodel.AssessmentServiceMetric
type PlanTaskMetric = statisticsreadmodel.PlanTaskMetric

const (
	OrgOverviewMetricAssessmentCreated = statisticsreadmodel.OrgOverviewMetricAssessmentCreated
	OrgOverviewMetricIntakeConfirmed   = statisticsreadmodel.OrgOverviewMetricIntakeConfirmed
	OrgOverviewMetricRelationAssigned  = statisticsreadmodel.OrgOverviewMetricRelationAssigned

	AccessFunnelMetricEntryOpened                 = statisticsreadmodel.AccessFunnelMetricEntryOpened
	AccessFunnelMetricIntakeConfirmed             = statisticsreadmodel.AccessFunnelMetricIntakeConfirmed
	AccessFunnelMetricTesteeCreated               = statisticsreadmodel.AccessFunnelMetricTesteeCreated
	AccessFunnelMetricCareRelationshipEstablished = statisticsreadmodel.AccessFunnelMetricCareRelationshipEstablished

	AssessmentServiceMetricAnswerSheetSubmitted = statisticsreadmodel.AssessmentServiceMetricAnswerSheetSubmitted
	AssessmentServiceMetricAssessmentCreated    = statisticsreadmodel.AssessmentServiceMetricAssessmentCreated
	AssessmentServiceMetricReportGenerated      = statisticsreadmodel.AssessmentServiceMetricReportGenerated
	AssessmentServiceMetricAssessmentFailed     = statisticsreadmodel.AssessmentServiceMetricAssessmentFailed

	PlanTaskMetricCreated   = statisticsreadmodel.PlanTaskMetricCreated
	PlanTaskMetricOpened    = statisticsreadmodel.PlanTaskMetricOpened
	PlanTaskMetricCompleted = statisticsreadmodel.PlanTaskMetricCompleted
	PlanTaskMetricExpired   = statisticsreadmodel.PlanTaskMetricExpired
)

type QuestionnaireBatchTotal = statisticsreadmodel.QuestionnaireBatchTotal
