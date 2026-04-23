package statistics

import statisticsreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/statisticsreadmodel"

type StatisticsReadModel = statisticsreadmodel.ReadModel

type OrgOverviewMetric = statisticsreadmodel.OrgOverviewMetric

const (
	OrgOverviewMetricAssessmentCreated = statisticsreadmodel.OrgOverviewMetricAssessmentCreated
	OrgOverviewMetricIntakeConfirmed   = statisticsreadmodel.OrgOverviewMetricIntakeConfirmed
	OrgOverviewMetricRelationAssigned  = statisticsreadmodel.OrgOverviewMetricRelationAssigned
)

type QuestionnaireBatchTotal = statisticsreadmodel.QuestionnaireBatchTotal
